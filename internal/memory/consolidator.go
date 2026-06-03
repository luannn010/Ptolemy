package memory

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

// ConsolidatorVersion is stamped into every synthesis row's metadata.
const ConsolidatorVersion = "consolidate_v1"

// synthImportance: summaries are high-value, start above atoms so they survive decay.
const synthImportance = 0.9

//go:embed prompts/consolidate_v1.txt
var consolidatePromptTemplate string

// Synthesis is the parsed LLM output for one (subject,project) summary.
type Synthesis struct {
	Content   string   `json:"content"`
	SourceIDs []string `json:"source_ids"`
}

// Consolidator turns a (subject,project)'s active atoms into one durable summary,
// reconciling against the prior summary and routing changes through Supersede.
// Batch/timed — never on the per-turn hot path.
type Consolidator struct {
	conn     *pgx.Conn
	embedder Embedder
	store    Store
	chat     ChatClient
	cfg      ConsolidateConfig
}

func NewConsolidator(conn *pgx.Conn, store Store, chat ChatClient, cfg ConsolidateConfig) *Consolidator {
	return &Consolidator{conn: conn, store: store, chat: chat, cfg: cfg}
}

// WithEmbedder sets the embedder (separate setter so parse-only unit tests can
// construct a Consolidator without one).
func (c *Consolidator) WithEmbedder(e Embedder) *Consolidator { c.embedder = e; return c }

// synthesize builds the prompt, calls the LLM, and parses the JSON summary.
func (c *Consolidator) synthesize(ctx context.Context, prevSummary string, atoms []Chunk) (Synthesis, error) {
	var b strings.Builder
	for _, a := range atoms {
		fmt.Fprintf(&b, "%s :: %s\n", a.ID, a.Content)
	}
	user := "PREVIOUS SUMMARY (may be empty):\n" + prevSummary + "\n\nATOMS (id :: content):\n" + b.String()
	raw, err := c.chat.Complete(ctx, consolidatePromptTemplate, user, CompleteOptions{})
	if err != nil {
		return Synthesis{}, fmt.Errorf("consolidate llm: %w", err)
	}
	return parseSynthesis(raw)
}

func parseSynthesis(raw string) (Synthesis, error) {
	s := strings.TrimSpace(raw)
	if m := jsonFence.FindStringSubmatch(s); m != nil {
		s = strings.TrimSpace(m[1])
	}
	if i := strings.Index(s, "{"); i > 0 {
		s = s[i:]
	}
	var out Synthesis
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return Synthesis{}, fmt.Errorf("parse synthesis: %w (raw=%q)", err, raw)
	}
	return out, nil
}

// ConsolidateSubjectProject loads the active atoms for (subject,project), synthesizes
// a summary, and stores it — superseding any prior summary (revision step) or upserting
// a fresh one. Skips when fewer than MinAtoms atoms exist or the LLM returns empty.
func (c *Consolidator) ConsolidateSubjectProject(ctx context.Context, subject, project string) error {
	atoms, err := c.activeAtoms(ctx, subject, project)
	if err != nil {
		return err
	}
	if len(atoms) < c.cfg.MinAtoms {
		return nil
	}
	prior, hasPrior, err := c.activeSynthesis(ctx, subject, project)
	if err != nil {
		return err
	}
	prevContent := ""
	if hasPrior {
		prevContent = prior.Content
	}
	syn, err := c.synthesize(ctx, prevContent, atoms)
	if err != nil {
		return err
	}
	if strings.TrimSpace(syn.Content) == "" {
		return nil
	}
	if hasPrior && normalizeContent(prior.Content) == normalizeContent(syn.Content) {
		return c.store.Reinforce(ctx, []string{prior.ID}) // unchanged → reinforce, don't supersede-to-self
	}
	vecs, err := c.embedder.Embed(ctx, []string{syn.Content})
	if err != nil || len(vecs) != 1 {
		if err == nil {
			err = fmt.Errorf("embedder returned %d vectors for synthesis", len(vecs))
		}
		return err
	}
	row := c.buildSynthChunk(subject, project, syn, vecs[0], time.Now().UTC())
	if hasPrior {
		return c.store.Supersede(ctx, []Chunk{row}, prior.ID)
	}
	return c.store.Upsert(ctx, []Chunk{row})
}

func (c *Consolidator) buildSynthChunk(subject, project string, syn Synthesis, vec []float32, now time.Time) Chunk {
	sub, prj, persp := subject, project, "factual"
	src := make([]any, len(syn.SourceIDs))
	for i, s := range syn.SourceIDs {
		src[i] = s
	}
	sum := sha256.Sum256([]byte(subject + "|" + project + "|" + syn.Content))
	return Chunk{
		ID:          "synth:" + hex.EncodeToString(sum[:])[:24],
		Content:     syn.Content,
		Embedding:   vec,
		PublishedAt: now,
		Scope:       "project",
		Status:      "active",
		Importance:  synthImportance,
		SubjectID:   &sub,
		ProjectID:   &prj,
		Perspective: &persp,
		Metadata: map[string]any{
			"kind":                 "synthesis",
			"consolidator_version": ConsolidatorVersion,
			"source_ids":           src,
		},
	}
}

// activeAtoms returns the subject's active atom rows for the project, oldest first.
func (c *Consolidator) activeAtoms(ctx context.Context, subject, project string) ([]Chunk, error) {
	rows, err := c.conn.Query(ctx, `
		SELECT id, content FROM chunks
		WHERE scope='project' AND status='active' AND subject_id=$1 AND project_id=$2
		  AND COALESCE(metadata->>'kind','atom')='atom'
		ORDER BY created_at ASC`, subject, project)
	if err != nil {
		return nil, fmt.Errorf("active atoms: %w", err)
	}
	defer rows.Close()
	var out []Chunk
	for rows.Next() {
		var ch Chunk
		if err := rows.Scan(&ch.ID, &ch.Content); err != nil {
			return nil, err
		}
		out = append(out, ch)
	}
	return out, rows.Err()
}

// activeSynthesis returns the current active summary for (subject,project), if any.
func (c *Consolidator) activeSynthesis(ctx context.Context, subject, project string) (Chunk, bool, error) {
	var ch Chunk
	err := c.conn.QueryRow(ctx, `
		SELECT id, content FROM chunks
		WHERE scope='project' AND status='active' AND subject_id=$1 AND project_id=$2
		  AND metadata->>'kind'='synthesis'
		ORDER BY created_at DESC LIMIT 1`, subject, project).Scan(&ch.ID, &ch.Content)
	if err == pgx.ErrNoRows {
		return Chunk{}, false, nil
	}
	if err != nil {
		return Chunk{}, false, fmt.Errorf("active synthesis: %w", err)
	}
	return ch, true, nil
}

// dueSubjectProjects returns (subject,project) pairs whose count of active atoms
// created since their last active synthesis is >= Buffer (or, with no prior
// synthesis, whose active atom count is >= Buffer). Drives the batch trigger.
func (c *Consolidator) dueSubjectProjects(ctx context.Context) ([][2]string, error) {
	rows, err := c.conn.Query(ctx, `
		SELECT a.subject_id, a.project_id
		FROM chunks a
		WHERE a.scope='project' AND a.status='active' AND a.subject_id IS NOT NULL AND a.project_id IS NOT NULL
		  AND COALESCE(a.metadata->>'kind','atom')='atom'
		  AND a.created_at > COALESCE((
		        SELECT max(s.created_at) FROM chunks s
		        WHERE s.metadata->>'kind'='synthesis' AND s.status='active'
		          AND s.subject_id=a.subject_id AND s.project_id=a.project_id), 'epoch'::timestamptz)
		GROUP BY a.subject_id, a.project_id
		HAVING count(*) >= $1`, c.cfg.Buffer)
	if err != nil {
		return nil, fmt.Errorf("due scan: %w", err)
	}
	defer rows.Close()
	var out [][2]string
	for rows.Next() {
		var s, p string
		if err := rows.Scan(&s, &p); err != nil {
			return nil, err
		}
		out = append(out, [2]string{s, p})
	}
	return out, rows.Err()
}

// consolidateOnce consolidates every due (subject,project). Directly callable in tests.
func (c *Consolidator) consolidateOnce(ctx context.Context) error {
	due, err := c.dueSubjectProjects(ctx)
	if err != nil {
		return err
	}
	for _, sp := range due {
		if err := c.ConsolidateSubjectProject(ctx, sp[0], sp[1]); err != nil {
			log.Error().Err(err).Str("subject", sp[0]).Str("project", sp[1]).Msg("consolidate failed; continuing")
		}
	}
	return nil
}

// Run ticks every cfg.Interval and consolidates due subject/projects. Closes done on return.
func (c *Consolidator) Run(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	log.Info().Dur("interval", c.cfg.Interval).Int("buffer", c.cfg.Buffer).Msg("consolidator loop started")
	runLoop(ctx, c.cfg.Interval, c.consolidateOnce)
	log.Info().Msg("consolidator loop stopped")
}
