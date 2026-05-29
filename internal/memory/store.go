package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
)

type Store interface {
	Upsert(ctx context.Context, chunks []Chunk) error
	Get(ctx context.Context, ids []string) ([]Chunk, error)

	// SupersedeOnUpsert atomically upserts the new chunks AND retires every chunk
	// whose id starts with "<oldDocID>#" via the unified version-chain model.
	// Doc-level: it sets the back-link (old rows → status='superseded', superseded_by)
	// but NOT a forward supersedes/version on the new chunks (a multi-chunk doc has no
	// 1:1 chunk mapping), so History() walks chains created by row-level Supersede, not
	// by doc-level replacement.
	SupersedeOnUpsert(ctx context.Context, chunks []Chunk, supersedesOldDocID string) error

	// Supersede inserts newChunks (active) and retires oldID, linking the chain:
	// old → status='superseded', superseded_by=newChunks[0].ID; newChunks[0] →
	// supersedes=oldID, version=old.version+1. Transactional + audited.
	// Non-representative chunks in newChunks are inserted as fresh rows
	// (version=1, no supersedes pointer); callers must pass new ids for them.
	Supersede(ctx context.Context, newChunks []Chunk, oldID string) error

	// History returns the full version chain for id, oldest → newest.
	History(ctx context.Context, id string) ([]Chunk, error)

	// LookupFact returns the most-recent active chunk for (factSubject, factPredicate),
	// or found=false when none exists. Drives the structured-fact ingest ladder.
	LookupFact(ctx context.Context, factSubject, factPredicate string) (chunk Chunk, found bool, err error)

	Reinforce(ctx context.Context, ids []string) error
	Stats(ctx context.Context) ([]ScopeStatusCount, error)
}

// ScopeStatusCount holds a single (scope, status) bucket with its row count.
type ScopeStatusCount struct {
	Scope  string
	Status string
	Count  int
}

type PgStore struct {
	conn *pgx.Conn
}

func NewPgStore(conn *pgx.Conn) *PgStore { return &PgStore{conn: conn} }

func (s *PgStore) Upsert(ctx context.Context, chunks []Chunk) error {
	for _, c := range chunks {
		meta, err := json.Marshal(c.Metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata for %s: %w", c.ID, err)
		}
		_, err = s.conn.Exec(ctx, `
			INSERT INTO chunks (id, content, embedding, metadata, source, tenant_id, published_at, scope,
			                    confidence, fact_subject, fact_predicate, importance,
			                    subject_id, session_id, project_id, perspective)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
			ON CONFLICT (id) DO UPDATE SET
				content = EXCLUDED.content,
				embedding = EXCLUDED.embedding,
				metadata = EXCLUDED.metadata,
				source = EXCLUDED.source,
				tenant_id = EXCLUDED.tenant_id,
				published_at = EXCLUDED.published_at,
				scope = EXCLUDED.scope,
				confidence = EXCLUDED.confidence,
				fact_subject = EXCLUDED.fact_subject,
				fact_predicate = EXCLUDED.fact_predicate,
				-- importance is re-asserted (not preserved) on re-upsert. Callers re-upsert
				-- whole chunks, so EXCLUDED carries the intended importance; the GC mutates
				-- decay via access_count/last_accessed_at, never via Upsert.
				importance = EXCLUDED.importance,
				subject_id = EXCLUDED.subject_id,
				session_id = EXCLUDED.session_id,
				project_id = EXCLUDED.project_id,
				perspective = EXCLUDED.perspective
		`,
			c.ID, c.Content, pgvector.NewVector(c.Embedding), meta,
			nullableStr(c.Source), nullableStr(c.Tenant), c.PublishedAt, scopeOrDefault(c.Scope),
			confidenceOrDefault(c.Confidence), c.FactSubject, c.FactPredicate, importanceOrDefault(c.Importance),
			c.SubjectID, c.SessionID, c.ProjectID, c.Perspective,
		)
		if err != nil {
			return fmt.Errorf("upsert %s: %w", c.ID, err)
		}
	}
	return nil
}

func (s *PgStore) Get(ctx context.Context, ids []string) ([]Chunk, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT id, content, embedding, metadata, COALESCE(source,''), COALESCE(tenant_id,''),
		       published_at, valid_from, valid_to, superseded_by, created_at,
		       scope, status, version, supersedes,
		       subject_id, session_id, project_id, perspective
		FROM chunks WHERE id = ANY($1)
	`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Chunk
	for rows.Next() {
		var c Chunk
		var emb pgvector.Vector
		var metaJSON []byte
		if err := rows.Scan(&c.ID, &c.Content, &emb, &metaJSON, &c.Source, &c.Tenant,
			&c.PublishedAt, &c.ValidFrom, &c.ValidTo, &c.SupersededBy, &c.CreatedAt,
			&c.Scope, &c.Status, &c.Version, &c.Supersedes,
			&c.SubjectID, &c.SessionID, &c.ProjectID, &c.Perspective); err != nil {
			return nil, err
		}
		c.Embedding = emb.Slice()
		if len(metaJSON) > 0 {
			_ = json.Unmarshal(metaJSON, &c.Metadata)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func nullableStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func scopeOrDefault(scope string) string {
	if scope == "" {
		return "global"
	}
	return scope
}

func confidenceOrDefault(c string) string {
	if c == "" {
		return "normal"
	}
	return c
}

// importanceOrDefault preserves the schema DEFAULT 1.0 for zero-value (document KB)
// chunks while letting capture set explicit project-row importance.
func importanceOrDefault(v float64) float64 {
	if v == 0 {
		return 1.0
	}
	return v
}

// normalizeContent trims and collapses internal whitespace, case-sensitive
// (content can be code/identifiers where case is meaningful). Used by the
// structured-fact ladder and dedup to decide content equality.
func normalizeContent(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// markSupersededTx retires each old id in favour of newID inside tx: status='superseded',
// superseded_by=newID, audited (active → superseded, 'supersession'). Matching zero rows is
// not an error. The caller must have inserted the new row(s) and set their supersedes/version.
func markSupersededTx(ctx context.Context, tx pgx.Tx, oldIDs []string, newID string) error {
	if len(oldIDs) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		WITH moved AS (
		  UPDATE chunks SET status='superseded', superseded_by=$1
		   WHERE id = ANY($2) AND status='active'
		  RETURNING id
		)
		INSERT INTO chunk_audit(chunk_id, old_status, new_status, reason)
		SELECT id, 'active', 'superseded', 'supersession' FROM moved
	`, newID, oldIDs); err != nil {
		return fmt.Errorf("mark superseded: %w", err)
	}
	return nil
}

func (s *PgStore) Supersede(ctx context.Context, newChunks []Chunk, oldID string) error {
	if len(newChunks) == 0 {
		return fmt.Errorf("Supersede: empty chunk slice")
	}
	tx, err := s.conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rep := newChunks[0].ID
	if oldID == rep {
		return fmt.Errorf("Supersede: new representative id %q must differ from oldID", rep)
	}
	var oldVersion int
	if err := tx.QueryRow(ctx, `SELECT version FROM chunks WHERE id=$1`, oldID).Scan(&oldVersion); err != nil {
		return fmt.Errorf("supersede: load old %s: %w", oldID, err)
	}
	for _, c := range newChunks {
		meta, err := json.Marshal(c.Metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata for %s: %w", c.ID, err)
		}
		var supersedes any
		version := 1
		if c.ID == rep {
			supersedes = oldID
			version = oldVersion + 1
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO chunks (id, content, embedding, metadata, source, tenant_id, published_at,
			                    scope, confidence, fact_subject, fact_predicate, supersedes, version,
			                    importance, subject_id, session_id, project_id, perspective)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
			ON CONFLICT (id) DO UPDATE SET
				content=EXCLUDED.content, embedding=EXCLUDED.embedding, metadata=EXCLUDED.metadata,
				source=EXCLUDED.source, tenant_id=EXCLUDED.tenant_id, published_at=EXCLUDED.published_at,
				scope=EXCLUDED.scope, confidence=EXCLUDED.confidence,
				fact_subject=EXCLUDED.fact_subject, fact_predicate=EXCLUDED.fact_predicate,
				supersedes=EXCLUDED.supersedes, version=EXCLUDED.version,
				importance=EXCLUDED.importance, subject_id=EXCLUDED.subject_id,
				session_id=EXCLUDED.session_id, project_id=EXCLUDED.project_id,
				perspective=EXCLUDED.perspective
		`, c.ID, c.Content, pgvector.NewVector(c.Embedding), meta, nullableStr(c.Source), nullableStr(c.Tenant),
			c.PublishedAt, scopeOrDefault(c.Scope), confidenceOrDefault(c.Confidence),
			c.FactSubject, c.FactPredicate, supersedes, version,
			importanceOrDefault(c.Importance), c.SubjectID, c.SessionID, c.ProjectID, c.Perspective); err != nil {
			return fmt.Errorf("supersede insert %s: %w", c.ID, err)
		}
	}
	if err := markSupersededTx(ctx, tx, []string{oldID}, rep); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PgStore) History(ctx context.Context, id string) ([]Chunk, error) {
	rows, err := s.conn.Query(ctx, `
		WITH RECURSIVE history AS (
		  SELECT id, content, scope, status, version, supersedes, superseded_by, ARRAY[id] AS path
		    FROM chunks WHERE id=$1
		  UNION ALL
		  SELECT c.id, c.content, c.scope, c.status, c.version, c.supersedes, c.superseded_by, h.path || c.id
		    FROM chunks c JOIN history h ON c.id = h.supersedes
		   WHERE NOT c.id = ANY(h.path)
		)
		SELECT id, content, scope, status, version, supersedes, superseded_by FROM history ORDER BY version
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Chunk
	for rows.Next() {
		var c Chunk
		if err := rows.Scan(&c.ID, &c.Content, &c.Scope, &c.Status, &c.Version, &c.Supersedes, &c.SupersededBy); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *PgStore) LookupFact(ctx context.Context, factSubject, factPredicate string) (Chunk, bool, error) {
	var c Chunk
	err := s.conn.QueryRow(ctx, `
		SELECT id, content, scope, status, version
		FROM chunks
		WHERE fact_subject=$1 AND fact_predicate=$2 AND status='active'
		ORDER BY version DESC, created_at DESC
		LIMIT 1
	`, factSubject, factPredicate).Scan(&c.ID, &c.Content, &c.Scope, &c.Status, &c.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return Chunk{}, false, nil
	}
	if err != nil {
		return Chunk{}, false, fmt.Errorf("lookup fact: %w", err)
	}
	return c, true, nil
}

func (s *PgStore) SupersedeOnUpsert(ctx context.Context, chunks []Chunk, supersedesOldDocID string) error {
	if len(chunks) == 0 {
		return fmt.Errorf("SupersedeOnUpsert: empty chunk slice")
	}
	tx, err := s.conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	// Defer Rollback; pgx makes a successful Commit a no-op for Rollback.
	defer func() { _ = tx.Rollback(ctx) }()

	for _, c := range chunks {
		meta, err := json.Marshal(c.Metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata for %s: %w", c.ID, err)
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO chunks (id, content, embedding, metadata, source, tenant_id, published_at, scope)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (id) DO UPDATE SET
				content = EXCLUDED.content,
				embedding = EXCLUDED.embedding,
				metadata = EXCLUDED.metadata,
				source = EXCLUDED.source,
				tenant_id = EXCLUDED.tenant_id,
				published_at = EXCLUDED.published_at,
				scope = EXCLUDED.scope
		`,
			c.ID, c.Content, pgvector.NewVector(c.Embedding), meta,
			nullableStr(c.Source), nullableStr(c.Tenant), c.PublishedAt, scopeOrDefault(c.Scope),
		)
		if err != nil {
			return fmt.Errorf("upsert %s: %w", c.ID, err)
		}
	}

	// Representative new chunk; one pointer per supersession is enough.
	rep := chunks[0].ID
	var oldIDs []string
	rows, err := tx.Query(ctx, `SELECT id FROM chunks WHERE id LIKE $1 || '#%' AND status='active'`, supersedesOldDocID)
	if err != nil {
		return fmt.Errorf("supersede %s: %w", supersedesOldDocID, err)
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		oldIDs = append(oldIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if err := markSupersededTx(ctx, tx, oldIDs, rep); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (s *PgStore) Reinforce(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := s.conn.Exec(ctx, `
		UPDATE chunks
		SET access_count = access_count + 1, last_accessed_at = now()
		WHERE id = ANY($1)
	`, ids)
	if err != nil {
		return fmt.Errorf("reinforce: %w", err)
	}
	return nil
}

func (s *PgStore) Stats(ctx context.Context) ([]ScopeStatusCount, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT scope, status, count(*)
		FROM chunks
		GROUP BY scope, status
		ORDER BY scope, status
	`)
	if err != nil {
		return nil, fmt.Errorf("stats: %w", err)
	}
	defer rows.Close()
	var out []ScopeStatusCount
	for rows.Next() {
		var c ScopeStatusCount
		if err := rows.Scan(&c.Scope, &c.Status, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
