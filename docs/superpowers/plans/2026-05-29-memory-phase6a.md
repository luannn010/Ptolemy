# Memory Phase 6a Implementation Plan — capture + subject-isolated recall

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add per-turn conversational capture (gate → extract → embed → store, async) and subject-isolated recall on top of the existing Memory GC, without regressing the all-global retrieval eval (must stay 1.000).

**Architecture:** Extend the existing `chunks` table with `subject_id`/`session_id`/`project_id`/`perspective` (migration 0006). A deterministic `Gate` skips trivial/secret-bearing turns. An LLM `Extractor` (BRAIN_* via a new `ChatClient`) turns an exchange into atomic, self-contained, grounded entries. `PerTurnCaptureHook` runs that pipeline on a bounded background channel and writes `scope='project'` rows, routing structured facts through the existing Phase-5 supersession ladder. Recall gains a `(subject_id IS NULL OR subject_id = $N)` isolation clause on both arms of the hybrid query plus a project-only recency tiebreak. `ContextBuilder` gains a real MMR selection pass (eval-neutral — eval bypasses it).

**Tech Stack:** Go 1.25, pgx v5, pgvector-go, ParadeDB (`@@@`/`paradedb.score`), zerolog, `go:embed`.

**Pre-reqs already in the tree (reuse, do not rebuild):** `OpenAIEmbedder.Embed`, `OpenAIGenerator` (HTTP `/v1/chat/completions`), `PgStore.{Upsert,Get,LookupFact,Reinforce,Supersede}`, `Orchestrator.Answer` (already calls `Reinforce`), `HybridRetriever`, the Phase-4/5 lifecycle columns + audit.

**Testing notes:**
- Pure/unit tests (gate, extractor with fake `ChatClient`, MMR, capture `processTurn` with fakes) run with no DB.
- DB integration tests use the existing `freshDB(t)` / `requirePG(t)` helpers in `internal/memory/*_test.go` and **skip** when `DATABASE_URL` is unset. This environment has a populated `.env`; run them with `set -a; . ./.env; set +a` exported.
- Run unit tests with `go test -p 1 ./internal/memory/...`. Run the eval with `make eval-memory`.

---

## File Structure

- `internal/memory/migrations/0006_chunks_subject.sql` — **create**. The four columns, two CHECKs, the recall index.
- `internal/memory/types.go` — **modify**. Add `SubjectID/SessionID/ProjectID/Perspective *string` to `Chunk`; add `SubjectID/ProjectID *string` to `Query`; add `Exchange` struct.
- `internal/memory/store.go` — **modify**. `Upsert` writes the four new columns + `importance`; `Get` scans them; add `importanceOrDefault`.
- `internal/memory/gate.go` — **create**. `Gate(Exchange) GateDecision` + secret/PII detection. Pure.
- `internal/memory/prompts/extract_v1.txt` — **create**. `go:embed`ed extraction prompt.
- `internal/memory/extractor.go` — **create**. `ChatClient` iface, `Extractor`, parsing, grounding + dangling-pronoun rejection.
- `internal/memory/generator.go` — **modify**. Add `OpenAIGenerator.Complete(ctx, system, user)` (raw completion, no citation parsing) to satisfy `ChatClient`.
- `internal/memory/capture.go` — **create**. `PerTurnCaptureHook`, `CaptureMetrics`, `Enqueue`, worker, `processTurn`, importance heuristic, capture chunk-id.
- `internal/memory/hybrid_retriever.go` — **modify**. Isolation clause on both arms + outer; project-only recency tiebreak.
- `internal/memory/context_builder.go` — **modify**. Add `MMRContextBuilder` (relevance = score, similarity = token-set cosine) + rune budget.
- `internal/memory/config.go` — **modify**. `CaptureBufferSize`, `MMRLambda` env knobs.
- `internal/memory/module.go` — **modify**. Swap `BudgetContextBuilder` → `MMRContextBuilder`; add `NewCaptureHook` helper.
- `internal/memory/eval/eval.go` + `internal/memory/capture_eval_test.go` — **create test**. 5–10 multi-session capture→recall→isolation cases.
- Test files: `gate_test.go`, `extractor_test.go`, `capture_test.go`, `context_builder_test.go` (extend), `hybrid_retriever_test.go` (extend), `store_test.go` (extend).

---

## Task 1: Migration 0006 + Chunk/Query struct + Upsert/Get columns

**Files:**
- Create: `internal/memory/migrations/0006_chunks_subject.sql`
- Modify: `internal/memory/types.go`
- Modify: `internal/memory/store.go`
- Test: `internal/memory/store_test.go` (extend), `internal/memory/migrations_test.go` (extend)

- [ ] **Step 1: Write the migration**

Create `internal/memory/migrations/0006_chunks_subject.sql`:

```sql
-- Phase 6a: conversational-memory scoping columns on chunks.
-- subject_id = user id (global across sessions); session_id + project_id tag the
-- capture context; perspective types the entry. Global KB rows leave all NULL.
ALTER TABLE chunks
  ADD COLUMN IF NOT EXISTS subject_id  TEXT,
  ADD COLUMN IF NOT EXISTS session_id  TEXT,
  ADD COLUMN IF NOT EXISTS project_id  TEXT,
  ADD COLUMN IF NOT EXISTS perspective TEXT;

DO $$ BEGIN
  ALTER TABLE chunks ADD CONSTRAINT chunks_perspective_chk
    CHECK (perspective IS NULL OR perspective IN ('factual','relational'));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- ISOLATION INVARIANT: a project row MUST be owned by a subject. Without this a
-- capture bug inserting subject_id=NULL produces a project row that matches the
-- `subject_id IS NULL` arm of recall and leaks to EVERY user.
DO $$ BEGIN
  ALTER TABLE chunks ADD CONSTRAINT chunks_project_owned_chk
    CHECK (scope = 'global' OR subject_id IS NOT NULL);
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- Recall filter index (project rows only; global KB unaffected).
CREATE INDEX IF NOT EXISTS chunks_subject ON chunks (subject_id, project_id)
  WHERE subject_id IS NOT NULL;
```

- [ ] **Step 2: Extend `Chunk` and `Query`, add `Exchange` in types.go**

In `internal/memory/types.go`, add to the `Chunk` struct (after `FactPredicate`):

```go
	// Phase 6a (conversational capture scoping). Nullable; global KB rows leave NULL.
	SubjectID   *string
	SessionID   *string
	ProjectID   *string
	Perspective *string // 'factual' | 'relational'
```

Extend `Query`:

```go
type Query struct {
	Text      string
	K         int
	AsOf      *time.Time
	Filters   map[string]any
	SubjectID *string // recall isolation; nil = global KB only
	ProjectID *string // reserved for 6b project filtering; populated, not filtered in 6a
}
```

Add the `Exchange` type at the end of the file:

```go
// Exchange is one full user+assistant turn, the unit of conversational capture.
// A turn is the exchange (user message + assistant reply), not the user message
// alone — the "how" of an implementation lives in the assistant's text.
type Exchange struct {
	UserText      string
	AssistantText string
	SubjectID     string
	SessionID     string
	ProjectID     string
}
```

- [ ] **Step 3: Update `Upsert` and `Get` in store.go**

Add the helper near `confidenceOrDefault`:

```go
// importanceOrDefault preserves the schema DEFAULT 1.0 for zero-value (document KB)
// chunks while letting capture set explicit project-row importance.
func importanceOrDefault(v float64) float64 {
	if v == 0 {
		return 1.0
	}
	return v
}
```

Replace the `Upsert` SQL + args to include the new columns and importance:

```go
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
```

Update `Get`'s SELECT and Scan to include the four columns:

```go
	rows, err := s.conn.Query(ctx, `
		SELECT id, content, embedding, metadata, COALESCE(source,''), COALESCE(tenant_id,''),
		       published_at, valid_from, valid_to, superseded_by, created_at,
		       scope, status, version, supersedes,
		       subject_id, session_id, project_id, perspective
		FROM chunks WHERE id = ANY($1)
	`, ids)
```

```go
		if err := rows.Scan(&c.ID, &c.Content, &emb, &metaJSON, &c.Source, &c.Tenant,
			&c.PublishedAt, &c.ValidFrom, &c.ValidTo, &c.SupersededBy, &c.CreatedAt,
			&c.Scope, &c.Status, &c.Version, &c.Supersedes,
			&c.SubjectID, &c.SessionID, &c.ProjectID, &c.Perspective); err != nil {
			return nil, err
		}
```

- [ ] **Step 4: Write failing integration test for the new columns + the CHECK**

Add to `internal/memory/store_test.go`:

```go
func TestPgStore_SubjectColumnsRoundTrip(t *testing.T) {
	conn := freshDB(t)
	s := NewPgStore(conn)
	subj, sess, proj, persp := "userA", "sess1", "ptolemy", "factual"
	in := Chunk{
		ID: "p1", Content: "the GC sweep archives stale rows",
		Embedding: []float32{1, 0, 0, 0}, PublishedAt: time.Now().UTC(),
		Scope: "project", Importance: 0.7,
		SubjectID: &subj, SessionID: &sess, ProjectID: &proj, Perspective: &persp,
	}
	if err := s.Upsert(context.Background(), []Chunk{in}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, err := s.Get(context.Background(), []string{"p1"})
	if err != nil || len(got) != 1 {
		t.Fatalf("Get: %v len=%d", err, len(got))
	}
	if got[0].SubjectID == nil || *got[0].SubjectID != "userA" || got[0].Perspective == nil || *got[0].Perspective != "factual" {
		t.Fatalf("scoping columns not round-tripped: %+v", got[0])
	}
}

func TestPgStore_ProjectRowRequiresSubject(t *testing.T) {
	conn := freshDB(t)
	_, err := conn.Exec(context.Background(),
		`INSERT INTO chunks (id, content, embedding, published_at, scope) VALUES ($1,$2,$3,$4,'project')`,
		"leak", "x", pgvector.NewVector([]float32{1, 0, 0, 0}), time.Now().UTC())
	if err == nil {
		t.Fatal("expected chunks_project_owned_chk to reject scope=project with subject_id=NULL")
	}
}
```

Add `"github.com/pgvector/pgvector-go"` to the test imports if not present.

- [ ] **Step 5: Run tests (fail if no DB → skip is acceptable; with DB they must pass)**

Run: `set -a; . ./.env; set +a; go test -p 1 -run 'SubjectColumns|ProjectRowRequires' ./internal/memory/`
Expected: PASS (or SKIP if `DATABASE_URL` unset).

- [ ] **Step 6: Commit**

```bash
git add internal/memory/migrations/0006_chunks_subject.sql internal/memory/types.go internal/memory/store.go internal/memory/store_test.go
git commit -m "feat(memory/6a): migration 0006 subject scoping + Upsert/Get columns"
```

---

## Task 2: MemoryGate (gate.go) — deterministic skip + secret guard

**Files:**
- Create: `internal/memory/gate.go`
- Test: `internal/memory/gate_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/memory/gate_test.go`:

```go
package memory

import "testing"

func TestGate_SkipsTrivial(t *testing.T) {
	cases := []Exchange{
		{UserText: "hi", AssistantText: "hello!"},
		{UserText: "thanks", AssistantText: "you're welcome"},
		{UserText: "ok", AssistantText: "👍"},
		{UserText: "", AssistantText: ""},
	}
	for _, ex := range cases {
		if d := Gate(ex); !d.Skip {
			t.Errorf("expected skip for %q/%q, got %+v", ex.UserText, ex.AssistantText, d)
		}
	}
}

func TestGate_KeepsSubstantive(t *testing.T) {
	ex := Exchange{
		UserText:      "how should the GC decide what to archive?",
		AssistantText: "the sweep archives project rows whose decay score falls below the threshold",
	}
	if d := Gate(ex); d.Skip {
		t.Errorf("expected keep, got skip: %+v", d)
	}
}

func TestGate_FlagsSecrets(t *testing.T) {
	secrets := []string{
		"my key is sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		"AWS key AKIAIOSFODNN7EXAMPLE here",
		"Authorization: Bearer xxxxxxxxxxxxxxxxxxxxxxxx",
		"-----BEGIN RSA PRIVATE KEY-----",
	}
	for _, s := range secrets {
		ex := Exchange{UserText: s, AssistantText: "noted, I stored the credential for you safely"}
		d := Gate(ex)
		if !d.ContainsSecret || !d.Skip {
			t.Errorf("expected secret skip for %q, got %+v", s, d)
		}
	}
}
```

- [ ] **Step 2: Run to verify fail**

Run: `go test -p 1 -run TestGate ./internal/memory/`
Expected: FAIL (undefined: Gate, GateDecision).

- [ ] **Step 3: Implement gate.go**

```go
package memory

import (
	"regexp"
	"strings"
)

// GateDecision is the deterministic, LLM-free verdict for whether a turn is
// worth capturing. ContainsSecret records that the turn matched an obvious-secret
// pattern (6a skips such turns; 6b may redact instead).
type GateDecision struct {
	Skip           bool
	Reason         string
	ContainsSecret bool
}

// minCaptureRunes: turns shorter than this carry no durable signal.
const minCaptureRunes = 16

var trivialPhrases = map[string]bool{
	"hi": true, "hey": true, "hello": true, "yo": true,
	"thanks": true, "thank you": true, "thx": true, "ty": true,
	"ok": true, "okay": true, "k": true, "cool": true, "nice": true,
	"yes": true, "no": true, "yep": true, "nope": true, "sure": true,
	"got it": true, "great": true, "perfect": true, "sounds good": true,
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`sk-[A-Za-z0-9]{20,}`),                       // OpenAI-style
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),                          // AWS access key id
	regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._\-]{20,}`),         // bearer token
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),        // PEM private key
	regexp.MustCompile(`(?i)(api[_-]?key|secret|token|password)\s*[:=]\s*\S{12,}`), // labelled secret
}

// highEntropyToken flags a single token that looks like a credential: long and
// drawing from at least three character classes (lower, upper, digit).
var highEntropyToken = regexp.MustCompile(`\b[A-Za-z0-9+/=_\-]{40,}\b`)

func containsSecret(s string) bool {
	for _, re := range secretPatterns {
		if re.MatchString(s) {
			return true
		}
	}
	for _, tok := range highEntropyToken.FindAllString(s, -1) {
		if charClasses(tok) >= 3 {
			return true
		}
	}
	return false
}

func charClasses(s string) int {
	var lower, upper, digit bool
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			lower = true
		case r >= 'A' && r <= 'Z':
			upper = true
		case r >= '0' && r <= '9':
			digit = true
		}
	}
	n := 0
	for _, b := range []bool{lower, upper, digit} {
		if b {
			n++
		}
	}
	return n
}

// Gate is a pure, fully testable skip for turns not worth capturing.
func Gate(ex Exchange) GateDecision {
	combined := strings.TrimSpace(ex.UserText + " " + ex.AssistantText)
	if containsSecret(ex.UserText) || containsSecret(ex.AssistantText) {
		return GateDecision{Skip: true, Reason: "secret", ContainsSecret: true}
	}
	if len([]rune(combined)) < minCaptureRunes {
		return GateDecision{Skip: true, Reason: "too_short"}
	}
	if trivialPhrases[strings.ToLower(strings.TrimSpace(ex.UserText))] &&
		len([]rune(strings.TrimSpace(ex.AssistantText))) < minCaptureRunes {
		return GateDecision{Skip: true, Reason: "greeting"}
	}
	return GateDecision{Skip: false}
}
```

- [ ] **Step 4: Run to verify pass**

Run: `go test -p 1 -run TestGate ./internal/memory/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/memory/gate.go internal/memory/gate_test.go
git commit -m "feat(memory/6a): deterministic capture gate + secret guard"
```

---

## Task 3: Extractor (prompt + extractor.go) + OpenAIGenerator.Complete

**Files:**
- Create: `internal/memory/prompts/extract_v1.txt`
- Create: `internal/memory/extractor.go`
- Modify: `internal/memory/generator.go`
- Test: `internal/memory/extractor_test.go`

- [ ] **Step 1: Write the embedded prompt**

Create `internal/memory/prompts/extract_v1.txt`:

```
You extract durable memory entries from one conversation turn (a user message and the assistant's reply).

Return ONLY a JSON array. Each element:
{"content": string, "perspective": "factual"|"relational", "fact_subject": string, "fact_predicate": string}

Rules:
- ATOMIC: exactly one fact per entry.
- SELF-CONTAINED: resolve every pronoun. No "it"/"this"/"that"/"they" without the noun. The entry will be read months later with zero surrounding context.
- DECLARATIVE: strip conversational filler ("I think", "maybe", "let's").
- ATTRIBUTE correctly: the user decided/asked; the assistant supplied the detail.
- TYPED: "factual" for durable facts/decisions/config; "relational" for preferences, working style, softer context.
- STRUCTURED when durable: for a fact about a stable subject, set fact_subject (the entity) and fact_predicate (the attribute). Otherwise set both to "".
- Capture nothing for trivial turns: return [].

Return [] if there is nothing durable. Output JSON only, no prose.

USER:
{{USER}}

ASSISTANT:
{{ASSISTANT}}
```

- [ ] **Step 2: Add `Complete` to OpenAIGenerator (generator.go)**

After the `Generate` method add:

```go
// Complete is a raw chat completion (no citation parsing). It satisfies the
// ChatClient interface the Extractor and Consolidator depend on.
func (g *OpenAIGenerator) Complete(ctx context.Context, system, user string) (string, error) {
	reqBody := chatRequest{
		Model: g.Model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if g.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+g.APIKey)
	}
	resp, err := g.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		msg, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("llm server %d: %s", resp.StatusCode, string(msg))
	}
	var parsed chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("llm returned no choices")
	}
	return parsed.Choices[0].Message.Content, nil
}
```

- [ ] **Step 3: Write failing extractor tests (fake ChatClient)**

Create `internal/memory/extractor_test.go`:

```go
package memory

import (
	"context"
	"testing"
)

type fakeChat struct{ resp string }

func (f fakeChat) Complete(ctx context.Context, system, user string) (string, error) {
	return f.resp, nil
}

func TestExtractor_ParsesAndKeepsGrounded(t *testing.T) {
	ex := Exchange{
		UserText:      "how does the GC decide what to archive?",
		AssistantText: "the sweep archives project rows whose decay score falls below the threshold",
	}
	json := `[{"content":"The GC sweep archives project rows below the decay threshold","perspective":"factual","fact_subject":"GC sweep","fact_predicate":"archives"}]`
	e := NewExtractor(fakeChat{resp: json})
	got, err := e.Extract(context.Background(), ex)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Perspective != "factual" {
		t.Fatalf("want 1 factual entry, got %+v", got)
	}
}

func TestExtractor_RejectsDanglingPronoun(t *testing.T) {
	ex := Exchange{UserText: "what does the sweep do?", AssistantText: "it archives stale rows"}
	json := `[{"content":"It archives stale rows","perspective":"factual","fact_subject":"","fact_predicate":""}]`
	e := NewExtractor(fakeChat{resp: json})
	got, _ := e.Extract(context.Background(), ex)
	if len(got) != 0 {
		t.Fatalf("dangling-pronoun entry should be rejected, got %+v", got)
	}
}

func TestExtractor_RejectsUngrounded(t *testing.T) {
	ex := Exchange{UserText: "how does the GC archive?", AssistantText: "the sweep archives stale rows"}
	json := `[{"content":"Quarterly revenue grew forty percent in Brazil","perspective":"factual","fact_subject":"","fact_predicate":""}]`
	e := NewExtractor(fakeChat{resp: json})
	got, _ := e.Extract(context.Background(), ex)
	if len(got) != 0 {
		t.Fatalf("ungrounded entry should be rejected, got %+v", got)
	}
}

func TestExtractor_GroundingAllowsCorefResolved(t *testing.T) {
	// Reconciliation case: exchange said "it"; entry resolved it to "the GC sweep".
	// Stemmed token overlap must NOT reject this.
	ex := Exchange{UserText: "what does it do?", AssistantText: "the sweep implemented archiving of stale project rows"}
	json := `[{"content":"The GC sweep implements archiving of stale project rows","perspective":"factual","fact_subject":"GC sweep","fact_predicate":"archiving"}]`
	e := NewExtractor(fakeChat{resp: json})
	got, _ := e.Extract(context.Background(), ex)
	if len(got) != 1 {
		t.Fatalf("coref-resolved grounded entry wrongly rejected, got %+v", got)
	}
}

func TestExtractor_HandlesEmptyAndFenced(t *testing.T) {
	e := NewExtractor(fakeChat{resp: "```json\n[]\n```"})
	got, err := e.Extract(context.Background(), Exchange{UserText: "hi", AssistantText: "hello"})
	if err != nil || len(got) != 0 {
		t.Fatalf("want empty, got %+v err=%v", got, err)
	}
}
```

- [ ] **Step 4: Run to verify fail**

Run: `go test -p 1 -run TestExtractor ./internal/memory/`
Expected: FAIL (undefined: NewExtractor, Extractor).

- [ ] **Step 5: Implement extractor.go**

```go
package memory

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// ExtractorVersion is stamped into every captured row's metadata so entries are
// auditable and selectively re-extractable when the prompt changes.
const ExtractorVersion = "extract_v1"

//go:embed prompts/extract_v1.txt
var extractPromptTemplate string

// ChatClient is the minimal LLM surface the Extractor (and 6b Consolidator) need.
// OpenAIGenerator.Complete satisfies it; tests use a fake.
type ChatClient interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

// ExtractedEntry is one atomic, self-contained memory candidate from a turn.
type ExtractedEntry struct {
	Content       string `json:"content"`
	Perspective   string `json:"perspective"`
	FactSubject   string `json:"fact_subject"`
	FactPredicate string `json:"fact_predicate"`
}

// Extractor turns an exchange into grounded, self-contained entries. The grounding
// and dangling-pronoun checks are deterministic and unit-tested with a fake client.
type Extractor struct {
	Client          ChatClient
	MinGroundOverlap float64 // fraction of entry content tokens that must appear in the exchange window
}

func NewExtractor(c ChatClient) *Extractor {
	return &Extractor{Client: c, MinGroundOverlap: 0.4}
}

func (e *Extractor) Extract(ctx context.Context, ex Exchange) ([]ExtractedEntry, error) {
	prompt := strings.NewReplacer("{{USER}}", ex.UserText, "{{ASSISTANT}}", ex.AssistantText).Replace(extractPromptTemplate)
	raw, err := e.Client.Complete(ctx, prompt, ex.UserText+"\n\n"+ex.AssistantText)
	if err != nil {
		return nil, fmt.Errorf("extractor llm: %w", err)
	}
	parsed, err := parseEntries(raw)
	if err != nil {
		return nil, err
	}
	// Ground against the WHOLE exchange window, not the user message alone.
	window := normalizeTokens(ex.UserText + " " + ex.AssistantText)
	windowSet := tokenSet(window)
	var out []ExtractedEntry
	for _, en := range parsed {
		if strings.TrimSpace(en.Content) == "" {
			continue
		}
		if en.Perspective != "factual" && en.Perspective != "relational" {
			en.Perspective = "relational"
		}
		if hasDanglingPronoun(en.Content) {
			continue
		}
		if !isGrounded(en.Content, windowSet, e.MinGroundOverlap) {
			continue
		}
		out = append(out, en)
	}
	return out, nil
}

var jsonFence = regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)\\s*```")

func parseEntries(raw string) ([]ExtractedEntry, error) {
	s := strings.TrimSpace(raw)
	if m := jsonFence.FindStringSubmatch(s); m != nil {
		s = strings.TrimSpace(m[1])
	}
	// Tolerate a leading object or array; require an array of entries.
	if i := strings.Index(s, "["); i > 0 {
		s = s[i:]
	}
	var entries []ExtractedEntry
	if err := json.Unmarshal([]byte(s), &entries); err != nil {
		return nil, fmt.Errorf("extractor parse: %w (raw=%q)", err, raw)
	}
	return entries, nil
}

// pronoun-at-clause-start detector: a dangling subject pronoun is one that opens
// the content or a sentence, with no antecedent available in isolation.
var clauseStartPronoun = regexp.MustCompile(`(?i)(^|[.;]\s+)(it|this|that|they|them|those|these|he|she)\b`)

func hasDanglingPronoun(content string) bool {
	return clauseStartPronoun.MatchString(strings.TrimSpace(content))
}

// isGrounded checks stemmed-token overlap of the entry against the exchange window.
func isGrounded(content string, windowSet map[string]bool, minOverlap float64) bool {
	toks := normalizeTokens(content)
	if len(toks) == 0 {
		return false
	}
	hit := 0
	for _, t := range toks {
		if windowSet[t] {
			hit++
		}
	}
	return float64(hit)/float64(len(toks)) >= minOverlap
}

var nonWord = regexp.MustCompile(`[^a-z0-9]+`)

var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "of": true, "to": true, "and": true,
	"or": true, "in": true, "on": true, "for": true, "is": true, "are": true,
	"was": true, "be": true, "by": true, "as": true, "at": true, "it": true,
	"this": true, "that": true, "with": true, "from": true,
}

// normalizeTokens lowercases, splits on non-word chars, drops stopwords, and
// applies a light stemmer so "implemented"/"implementation"/"implements" match.
func normalizeTokens(s string) []string {
	parts := nonWord.Split(strings.ToLower(s), -1)
	var out []string
	for _, p := range parts {
		if p == "" || stopWords[p] {
			continue
		}
		out = append(out, stem(p))
	}
	return out
}

func tokenSet(toks []string) map[string]bool {
	m := make(map[string]bool, len(toks))
	for _, t := range toks {
		m[t] = true
	}
	return m
}

// stem strips common English suffixes. Deliberately crude (no Porter dep):
// enough to bridge implement/implemented/implementation and archive/archiving.
func stem(w string) string {
	for _, suf := range []string{"ization", "ation", "ing", "edly", "ed", "ly", "es", "s"} {
		if len(w) > len(suf)+2 && strings.HasSuffix(w, suf) {
			return w[:len(w)-len(suf)]
		}
	}
	return w
}
```

- [ ] **Step 6: Run to verify pass**

Run: `go test -p 1 -run 'TestExtractor' ./internal/memory/`
Expected: PASS (all five).

- [ ] **Step 7: Commit**

```bash
git add internal/memory/prompts/extract_v1.txt internal/memory/extractor.go internal/memory/generator.go internal/memory/extractor_test.go
git commit -m "feat(memory/6a): grounded self-contained extractor + ChatClient.Complete"
```

---

## Task 4: PerTurnCaptureHook (capture.go) — async, bounded, metered

**Files:**
- Create: `internal/memory/capture.go`
- Test: `internal/memory/capture_test.go`

- [ ] **Step 1: Write failing tests (fakes; no DB)**

Create `internal/memory/capture_test.go`:

```go
package memory

import (
	"context"
	"sync"
	"testing"
)

type fakeEmbedder struct{}

func (fakeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{1, 0, 0, 0}
	}
	return out, nil
}

// captureFakeStore records Upsert calls; implements just enough of Store.
type captureFakeStore struct {
	mu       sync.Mutex
	upserted []Chunk
}

func (s *captureFakeStore) Upsert(ctx context.Context, c []Chunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.upserted = append(s.upserted, c...)
	return nil
}
func (s *captureFakeStore) Get(context.Context, []string) ([]Chunk, error)  { return nil, nil }
func (s *captureFakeStore) SupersedeOnUpsert(context.Context, []Chunk, string) error { return nil }
func (s *captureFakeStore) Supersede(context.Context, []Chunk, string) error { return nil }
func (s *captureFakeStore) History(context.Context, string) ([]Chunk, error) { return nil, nil }
func (s *captureFakeStore) LookupFact(context.Context, string, string) (Chunk, bool, error) {
	return Chunk{}, false, nil
}
func (s *captureFakeStore) Reinforce(context.Context, []string) error { return nil }
func (s *captureFakeStore) Stats(context.Context) ([]ScopeStatusCount, error) { return nil, nil }

func newTestHook(store Store) *PerTurnCaptureHook {
	return NewCaptureHook(NewExtractor(fakeChat{resp: `[{"content":"The GC sweep archives stale project rows below the threshold","perspective":"factual","fact_subject":"GC sweep","fact_predicate":"archives"}]`}),
		fakeEmbedder{}, store, 8)
}

func TestCapture_ProcessTurnWritesProjectRow(t *testing.T) {
	store := &captureFakeStore{}
	h := newTestHook(store)
	ex := Exchange{UserText: "how does the GC archive?", AssistantText: "the sweep archives stale project rows below the threshold",
		SubjectID: "userA", SessionID: "s1", ProjectID: "ptolemy"}
	if err := h.processTurn(context.Background(), ex); err != nil {
		t.Fatal(err)
	}
	if len(store.upserted) != 1 {
		t.Fatalf("want 1 row, got %d", len(store.upserted))
	}
	c := store.upserted[0]
	if c.Scope != "project" || c.SubjectID == nil || *c.SubjectID != "userA" || c.ProjectID == nil || *c.ProjectID != "ptolemy" {
		t.Fatalf("scoping wrong: %+v", c)
	}
	if c.Importance != factImportance {
		t.Fatalf("factual entry should get factImportance %.2f, got %.2f", factImportance, c.Importance)
	}
	if c.Metadata["extractor_version"] != ExtractorVersion {
		t.Fatalf("extractor_version not stamped: %+v", c.Metadata)
	}
}

func TestCapture_GateSkipsTrivial(t *testing.T) {
	store := &captureFakeStore{}
	h := newTestHook(store)
	_ = h.processTurn(context.Background(), Exchange{UserText: "thanks", AssistantText: "yw", SubjectID: "userA"})
	if len(store.upserted) != 0 {
		t.Fatalf("trivial turn should produce no rows, got %d", len(store.upserted))
	}
}

func TestCapture_EnqueueDoesNotBlockAndDrops(t *testing.T) {
	store := &captureFakeStore{}
	// buffer 1, no worker started → second+ enqueues must drop, never block.
	h := NewCaptureHook(NewExtractor(fakeChat{resp: "[]"}), fakeEmbedder{}, store, 1)
	for i := 0; i < 100; i++ {
		h.Enqueue(Exchange{UserText: "x", AssistantText: "y", SubjectID: "userA"})
	}
	if got := h.Metrics().Dropped(); got == 0 {
		t.Fatalf("expected drops on a full channel, got %d", got)
	}
}
```

- [ ] **Step 2: Run to verify fail**

Run: `go test -p 1 -run TestCapture ./internal/memory/`
Expected: FAIL (undefined: NewCaptureHook, PerTurnCaptureHook, factImportance).

- [ ] **Step 3: Implement capture.go**

```go
package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
)

// Initial importance (§7): durable facts start slightly above loose relational
// chatter so the GC's decay ordering is better than flat from day one.
const (
	factImportance = 0.7
	relImportance  = 0.5
)

// CaptureMetrics counts capture outcomes so silent forgetting is observable.
type CaptureMetrics struct {
	dropped    int64
	extractErr int64
	embedErr   int64
	captured   int64
}

func (m *CaptureMetrics) Dropped() int64    { return atomic.LoadInt64(&m.dropped) }
func (m *CaptureMetrics) ExtractErr() int64  { return atomic.LoadInt64(&m.extractErr) }
func (m *CaptureMetrics) EmbedErr() int64    { return atomic.LoadInt64(&m.embedErr) }
func (m *CaptureMetrics) Captured() int64    { return atomic.LoadInt64(&m.captured) }

// PerTurnCaptureHook runs gate→extract→embed→store on a bounded background
// channel. Enqueue never blocks; a full channel drops (and counts) the exchange.
// In-flight exchanges are lost on process restart (acceptable for 6a; a durable
// outbox is a 6b option).
type PerTurnCaptureHook struct {
	extractor *Extractor
	embedder  Embedder
	store     Store
	ch        chan Exchange
	metrics   *CaptureMetrics
}

func NewCaptureHook(ex *Extractor, emb Embedder, st Store, buf int) *PerTurnCaptureHook {
	if buf <= 0 {
		buf = 256
	}
	return &PerTurnCaptureHook{
		extractor: ex,
		embedder:  emb,
		store:     st,
		ch:        make(chan Exchange, buf),
		metrics:   &CaptureMetrics{},
	}
}

func (h *PerTurnCaptureHook) Metrics() *CaptureMetrics { return h.metrics }

// Start launches the background worker bound to ctx. Returns immediately.
func (h *PerTurnCaptureHook) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ex := <-h.ch:
				if err := h.processTurn(ctx, ex); err != nil {
					log.Warn().Err(err).Msg("capture processTurn failed; dropping exchange")
				}
			}
		}
	}()
}

// Enqueue hands an exchange to the worker without blocking. A full channel drops.
func (h *PerTurnCaptureHook) Enqueue(ex Exchange) {
	select {
	case h.ch <- ex:
	default:
		atomic.AddInt64(&h.metrics.dropped, 1)
		log.Warn().Msg("capture channel full; dropping exchange")
	}
}

// processTurn is the deterministic pipeline, directly callable in tests.
func (h *PerTurnCaptureHook) processTurn(ctx context.Context, ex Exchange) error {
	if d := Gate(ex); d.Skip {
		return nil
	}
	entries, err := h.extractor.Extract(ctx, ex)
	if err != nil {
		atomic.AddInt64(&h.metrics.extractErr, 1)
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	texts := make([]string, len(entries))
	for i, e := range entries {
		texts[i] = e.Content
	}
	vecs, err := h.embedder.Embed(ctx, texts)
	if err != nil || len(vecs) != len(entries) {
		atomic.AddInt64(&h.metrics.embedErr, 1)
		if err == nil {
			err = errors.New("embedder returned wrong vector count")
		}
		return err
	}
	now := time.Now().UTC()
	for i, e := range entries {
		c := h.buildChunk(ex, e, vecs[i], now)
		// Structured-fact ladder (reuses Phase 5; no new supersession code).
		if c.FactSubject != nil && c.FactPredicate != nil {
			existing, found, lerr := h.store.LookupFact(ctx, *c.FactSubject, *c.FactPredicate)
			if lerr != nil {
				return lerr
			}
			if found {
				if normalizeContent(existing.Content) == normalizeContent(c.Content) {
					if err := h.store.Reinforce(ctx, []string{existing.ID}); err != nil {
						return err
					}
				} else if err := h.store.Supersede(ctx, []Chunk{c}, existing.ID); err != nil {
					return err
				}
				atomic.AddInt64(&h.metrics.captured, 1)
				continue
			}
		}
		if err := h.store.Upsert(ctx, []Chunk{c}); err != nil {
			return err
		}
		atomic.AddInt64(&h.metrics.captured, 1)
	}
	return nil
}

func (h *PerTurnCaptureHook) buildChunk(ex Exchange, e ExtractedEntry, vec []float32, now time.Time) Chunk {
	subj, sess, proj, persp := ex.SubjectID, ex.SessionID, ex.ProjectID, e.Perspective
	imp := relImportance
	c := Chunk{
		ID:          captureChunkID(ex, e),
		Content:     e.Content,
		Embedding:   vec,
		PublishedAt: now,
		Scope:       "project",
		Perspective: &persp,
		SubjectID:   &subj,
		SessionID:   &sess,
		ProjectID:   &proj,
		Metadata:    map[string]any{"extractor_version": ExtractorVersion, "kind": "atom"},
	}
	if e.FactSubject != "" && e.FactPredicate != "" {
		fs, fp := e.FactSubject, e.FactPredicate
		c.FactSubject = &fs
		c.FactPredicate = &fp
		imp = factImportance
	}
	c.Importance = imp
	return c
}

// captureChunkID is content-addressed within (subject, project) so an identical
// re-extraction collapses onto the same row (the ladder/dedup then reinforces).
func captureChunkID(ex Exchange, e ExtractedEntry) string {
	sum := sha256.Sum256([]byte(ex.SubjectID + "|" + ex.ProjectID + "|" + e.Content))
	return "turn:" + hex.EncodeToString(sum[:])[:24]
}
```

- [ ] **Step 4: Run to verify pass**

Run: `go test -p 1 -run TestCapture ./internal/memory/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/memory/capture.go internal/memory/capture_test.go
git commit -m "feat(memory/6a): async bounded per-turn capture hook + fact ladder"
```

---

## Task 5: Recall — subject isolation + project-only recency tiebreak

**Files:**
- Modify: `internal/memory/hybrid_retriever.go`
- Test: `internal/memory/hybrid_retriever_test.go` (extend)

- [ ] **Step 1: Update the hybrid query and Retrieve**

Replace `hybridRrfQuery` with (isolation clause on both arms + outer, project-only recency secondary order):

```go
const hybridRrfQuery = `
WITH bm25 AS (
    SELECT id, ROW_NUMBER() OVER (ORDER BY paradedb.score(id) DESC) AS rank
    FROM chunks
    WHERE content @@@ $1
      AND status = 'active'
      AND published_at <= $5
      AND (subject_id IS NULL OR subject_id = $8)
    ORDER BY paradedb.score(id) DESC
    LIMIT $3
),
vec AS (
    SELECT id, ROW_NUMBER() OVER (ORDER BY embedding <=> $2) AS rank
    FROM chunks
    WHERE status = 'active'
      AND published_at <= $5
      AND (subject_id IS NULL OR subject_id = $8)
    ORDER BY embedding <=> $2
    LIMIT $3
)
SELECT c.id, c.content, c.metadata, COALESCE(c.source,''), c.published_at,
       COALESCE(1.0 / (60 + b.rank), 0)
     + COALESCE(1.0 / (60 + v.rank), 0)
     + $6 * exp(-extract(epoch FROM $5 - c.published_at) / $7) AS score
FROM chunks c
LEFT JOIN bm25 b ON b.id = c.id
LEFT JOIN vec  v ON v.id = c.id
WHERE (b.id IS NOT NULL OR v.id IS NOT NULL)
  AND c.status = 'active'
  AND c.published_at <= $5
  AND (c.subject_id IS NULL OR c.subject_id = $8)
ORDER BY score DESC,
         CASE WHEN c.scope = 'project' THEN c.last_accessed_at END DESC NULLS LAST
LIMIT $4
`
```

> Baseline safety: the all-global eval passes `SubjectID = nil` → `$8 = NULL`, so `subject_id = NULL` is never true and only `subject_id IS NULL` (every global row) matches — identical result set. The secondary `ORDER BY` key is `NULL` for every global row (`scope='global'`) → no reordering. Both arms' rank/score math are byte-identical to the prior query, so recall stays 1.000.

In `Retrieve`, bind `$8` from the query subject (nil → NULL):

```go
	var subj any
	if q.SubjectID != nil {
		subj = *q.SubjectID
	}
	rows, err := r.conn.Query(ctx, hybridRrfQuery,
		q.Text,
		pgvector.NewVector(vecs[0]),
		depth,
		finalK,
		asOf,
		r.recencyWeight,
		r.recencyHalfLife.Seconds(),
		subj,
	)
```

- [ ] **Step 2: Write failing isolation integration test**

Add to `internal/memory/hybrid_retriever_test.go` (follow the file's existing setup helper; if it has its own `freshDB`-style helper reuse it — otherwise use `freshDB(t)`):

```go
func TestHybrid_SubjectIsolation(t *testing.T) {
	conn := freshDB(t)
	s := NewPgStore(conn)
	emb := stubEmbedder{vec: []float32{1, 0, 0, 0}} // reuse the file's existing stub; adjust name to match
	r := NewHybridRetriever(conn, emb, 0.1, 30*24*time.Hour)

	a, b := "userA", "userB"
	now := time.Now().UTC()
	mk := func(id, content, subj string) Chunk {
		ss, pp := subj, "factual"
		sess, proj := "s", "ptolemy"
		return Chunk{ID: id, Content: content, Embedding: []float32{1, 0, 0, 0}, PublishedAt: now,
			Scope: "project", Importance: 0.7, SubjectID: &ss, SessionID: &sess, ProjectID: &proj, Perspective: &pp}
	}
	if err := s.Upsert(context.Background(), []Chunk{
		mk("pa", "userA secret pancake recipe", a),
		mk("pb", "userB secret pancake recipe", b),
	}); err != nil {
		t.Fatal(err)
	}
	got, err := r.Retrieve(context.Background(), Query{Text: "pancake recipe", K: 10, SubjectID: &a}, 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range got {
		if c.ID == "pb" {
			t.Fatalf("subject A must not see subject B's row pb; got %v", ids(got))
		}
	}
}
```

If `hybrid_retriever_test.go` lacks an embedder stub or `ids` helper, add minimal ones:

```go
type stubEmbedder struct{ vec []float32 }

func (e stubEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = e.vec
	}
	return out, nil
}

func ids(cs []RetrievedChunk) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.ID
	}
	return out
}
```

(Check for name collisions with existing helpers in the package test files before adding — reuse if present.)

- [ ] **Step 3: Run to verify (PASS with DB / SKIP without)**

Run: `set -a; . ./.env; set +a; go test -p 1 -run 'TestHybrid_SubjectIsolation' ./internal/memory/`
Expected: PASS (or SKIP without DB).

- [ ] **Step 4: Run the full eval to confirm no regression**

Run: `make eval-memory`
Expected: overall recall **1.000** (unchanged).

- [ ] **Step 5: Commit**

```bash
git add internal/memory/hybrid_retriever.go internal/memory/hybrid_retriever_test.go
git commit -m "feat(memory/6a): subject-isolation clause + project recency tiebreak in recall"
```

---

## Task 6: MMRContextBuilder (read-side relevance trim)

**Files:**
- Modify: `internal/memory/context_builder.go`
- Test: `internal/memory/context_builder_test.go` (extend)

> MMR is eval-neutral: the eval calls `Retriever.Retrieve` directly and never builds a prompt (`eval.RunRetrieval` deliberately skips the Generator). Similarity uses token-set cosine over normalized content (no embeddings/query-vector plumbing needed — `HybridRetriever` does not return embeddings). λ and final-k are config knobs.

- [ ] **Step 1: Write failing test**

Add to `internal/memory/context_builder_test.go`:

```go
func TestMMR_DropsNearDuplicates(t *testing.T) {
	chunks := []RetrievedChunk{
		{Chunk: Chunk{ID: "a", Content: "the GC sweep archives stale project rows below the decay threshold"}, Score: 1.0},
		{Chunk: Chunk{ID: "a2", Content: "the GC sweep archives stale project rows below the decay threshold"}, Score: 0.99},
		{Chunk: Chunk{ID: "b", Content: "recall isolates each subject so users never see another user memory"}, Score: 0.8},
	}
	b := MMRContextBuilder{Lambda: 0.7, K: 2, MaxRunes: 6000}
	pc := b.Build(Query{Text: "how does memory work"}, chunks)
	// With k=2, MMR should pick the near-duplicate's representative once and then
	// the distinct "b", not both near-identical "a"/"a2".
	if len(pc.SourceIDs) != 2 {
		t.Fatalf("want 2 selected, got %v", pc.SourceIDs)
	}
	got := map[string]bool{pc.SourceIDs[0]: true, pc.SourceIDs[1]: true}
	if !got["b"] {
		t.Fatalf("MMR should include the distinct chunk b; got %v", pc.SourceIDs)
	}
	if got["a"] && got["a2"] {
		t.Fatalf("MMR should not pick both near-duplicates; got %v", pc.SourceIDs)
	}
}
```

- [ ] **Step 2: Run to verify fail**

Run: `go test -p 1 -run TestMMR ./internal/memory/`
Expected: FAIL (undefined: MMRContextBuilder).

- [ ] **Step 3: Implement MMRContextBuilder in context_builder.go**

Append:

```go
// MMRContextBuilder selects a diverse, relevant subset via Maximal Marginal
// Relevance, then packs it under a rune budget. Relevance is the retrieval score;
// pairwise similarity is token-set cosine over normalized content. This is the
// read-side relevance trim (do NOT push relevance trimming to capture).
type MMRContextBuilder struct {
	Lambda   float64 // ~0.7
	K        int     // final selection cap (0 = all candidates)
	MaxRunes int
}

func (b MMRContextBuilder) Build(q Query, chunks []RetrievedChunk) PromptContext {
	selected := selectMMR(chunks, b.Lambda, b.K)
	var body strings.Builder
	var ids []string
	used := 0
	for i, c := range selected {
		piece := fmt.Sprintf("\n[source:%s]\n%s\n", c.ID, c.Content)
		if i > 0 && b.MaxRunes > 0 && used+len([]rune(piece)) > b.MaxRunes {
			break
		}
		body.WriteString(piece)
		ids = append(ids, c.ID)
		used += len([]rune(piece))
	}
	return PromptContext{
		System:    "You are a careful assistant. Answer using only the provided sources and cite them by id in square brackets like [source:id].",
		User:      fmt.Sprintf("Sources:\n%s\n\nQuestion: %s", body.String(), q.Text),
		SourceIDs: ids,
	}
}

func selectMMR(chunks []RetrievedChunk, lambda float64, k int) []RetrievedChunk {
	if k <= 0 || k > len(chunks) {
		k = len(chunks)
	}
	if len(chunks) == 0 {
		return nil
	}
	if lambda <= 0 {
		lambda = 0.7
	}
	sets := make([]map[string]int, len(chunks))
	for i, c := range chunks {
		sets[i] = tokenCounts(c.Content)
	}
	chosen := make([]bool, len(chunks))
	var out []RetrievedChunk
	for len(out) < k {
		best, bestScore := -1, 0.0
		first := len(out) == 0
		for i := range chunks {
			if chosen[i] {
				continue
			}
			maxSim := 0.0
			for j := range chunks {
				if !chosen[j] {
					continue
				}
				if s := cosineCounts(sets[i], sets[j]); s > maxSim {
					maxSim = s
				}
			}
			mmr := lambda*chunks[i].Score - (1-lambda)*maxSim
			if first || best == -1 || mmr > bestScore {
				best, bestScore = i, mmr
				first = false
			}
		}
		if best == -1 {
			break
		}
		chosen[best] = true
		out = append(out, chunks[best])
	}
	return out
}

func tokenCounts(s string) map[string]int {
	m := map[string]int{}
	for _, t := range normalizeTokens(s) {
		m[t]++
	}
	return m
}

func cosineCounts(a, b map[string]int) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	var dot, na, nb float64
	for t, av := range a {
		na += float64(av * av)
		if bv, ok := b[t]; ok {
			dot += float64(av * bv)
		}
	}
	for _, bv := range b {
		nb += float64(bv * bv)
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
```

Add `"math"` to the `context_builder.go` import block.

- [ ] **Step 4: Run to verify pass**

Run: `go test -p 1 -run TestMMR ./internal/memory/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/memory/context_builder.go internal/memory/context_builder_test.go
git commit -m "feat(memory/6a): MMR read-side selection in ContextBuilder"
```

---

## Task 7: Config knobs + Module wiring

**Files:**
- Modify: `internal/memory/config.go`
- Modify: `internal/memory/module.go`
- Test: `internal/memory/config_test.go` (extend)

- [ ] **Step 1: Add config fields + defaults**

In `MemoryConfig` add:

```go
	// Phase 6a capture/recall knobs.
	CaptureBufferSize int     // CAPTURE_BUFFER_SIZE (default 256)
	MMRLambda         float64 // RAG_MMR_LAMBDA (default 0.7)
```

In `LoadConfig`, after the GC block:

```go
	cfg.CaptureBufferSize = intEnv("CAPTURE_BUFFER_SIZE", 256)
	cfg.MMRLambda = floatEnv("RAG_MMR_LAMBDA", 0.7)
	if cfg.MMRLambda < 0 || cfg.MMRLambda > 1 {
		return MemoryConfig{}, fmt.Errorf("RAG_MMR_LAMBDA must be in [0,1], got %v", cfg.MMRLambda)
	}
```

- [ ] **Step 2: Write failing config test**

Add to `internal/memory/config_test.go` (match the file's existing env-set/reset pattern):

```go
func TestLoadConfig_CaptureAndMMRDefaults(t *testing.T) {
	setRequiredEnv(t) // reuse the helper the file already uses to set DATABASE_URL/EMBEDDING_*/BRAIN_*
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CaptureBufferSize != 256 {
		t.Errorf("CaptureBufferSize default = %d, want 256", cfg.CaptureBufferSize)
	}
	if cfg.MMRLambda != 0.7 {
		t.Errorf("MMRLambda default = %v, want 0.7", cfg.MMRLambda)
	}
}
```

> If `config_test.go` has no `setRequiredEnv` helper, inline `t.Setenv` calls for `DATABASE_URL`, `EMBEDDING_BASE_URL`, `EMBEDDING_MODEL`, `EMBEDDING_DIM`, `BRAIN_BASE_URL`, `BRAIN_MODEL` (mirror an existing passing test in that file).

- [ ] **Step 3: Run to verify fail then implement (already implemented in Step 1) → pass**

Run: `go test -p 1 -run TestLoadConfig_CaptureAndMMRDefaults ./internal/memory/`
Expected: PASS.

- [ ] **Step 4: Wire Module — swap ContextBuilder + add NewCaptureHook**

In `module.go`, change the `ContextBuilder` field in the returned `Orchestrator`:

```go
		ContextBuilder: MMRContextBuilder{Lambda: cfg.MMRLambda, K: cfg.TopK, MaxRunes: 6000},
```

Add a constructor below `NewModule`. The low-level `NewCaptureHook(ex, emb, st, buf)` from `capture.go` (Task 4) stays the single public primitive; this is a config-level convenience that builds the BRAIN extractor for the caller:

```go
// NewCaptureHookFromConfig builds (but does not start) the per-turn capture hook,
// constructing the BRAIN_* extractor from config. The caller starts it with
// hook.Start(ctx) and feeds it via hook.Enqueue. Wiring into the agent loop is
// out of 6a scope.
func NewCaptureHookFromConfig(cfg MemoryConfig, store Store, embedder Embedder) *PerTurnCaptureHook {
	chat := NewOpenAIGenerator(cfg.LLMBaseURL, cfg.LLMModel, "")
	return NewCaptureHook(NewExtractor(chat), embedder, store, cfg.CaptureBufferSize)
}
```

- [ ] **Step 5: Build + run package tests**

Run: `go build ./... && go test -p 1 ./internal/memory/...`
Expected: build OK; tests PASS/SKIP.

- [ ] **Step 6: Commit**

```bash
git add internal/memory/config.go internal/memory/config_test.go internal/memory/module.go
git commit -m "feat(memory/6a): config knobs + wire MMR builder and capture-hook constructor"
```

---

## Task 8: Minimal 6a capture→recall→isolation eval

**Files:**
- Create: `internal/memory/capture_eval_test.go`

> Integration test exercising the new behavior end-to-end against a real DB + the real BRAIN extractor is heavy; keep the committed minimal eval as a DB integration test using the **fake** extractor (deterministic) for capture, and the **real** hybrid retriever for recall. The real-LLM extractor is covered by a smoke target (Task 9), not committed CI.

- [ ] **Step 1: Write the multi-session eval test**

Create `internal/memory/capture_eval_test.go`:

```go
package memory

import (
	"context"
	"testing"
	"time"
)

// cannedExtractor returns one factual entry echoing the assistant text, so the
// capture→store→recall path is exercised deterministically without the BRAIN LLM.
type cannedExtractor struct{}

func (cannedExtractor) Complete(ctx context.Context, system, user string) (string, error) {
	// Echo a grounded, self-contained entry built from the user message.
	return `[{"content":"` + jsonEscape(user) + `","perspective":"factual","fact_subject":"","fact_predicate":""}]`, nil
}

func jsonEscape(s string) string {
	r := []rune(s)
	out := make([]rune, 0, len(r))
	for _, c := range r {
		if c == '"' || c == '\\' {
			out = append(out, '\\')
		}
		if c == '\n' {
			out = append(out, '\\', 'n')
			continue
		}
		out = append(out, c)
	}
	return string(out)
}

func TestCaptureRecall_MultiSession(t *testing.T) {
	conn := freshDB(t)
	store := NewPgStore(conn)
	emb := stubEmbedder{vec: []float32{1, 0, 0, 0}}
	hook := NewCaptureHook(&Extractor{Client: cannedExtractor{}, MinGroundOverlap: 0.0}, emb, store, 8)

	// Session 1, subject A captures a decision.
	exA := Exchange{
		UserText:      "we will store synthesis summaries as chunks rows tagged kind synthesis",
		AssistantText: "synthesis summaries are chunks rows distinguished by a metadata marker",
		SubjectID:     "userA", SessionID: "s1", ProjectID: "ptolemy",
	}
	if err := hook.processTurn(context.Background(), exA); err != nil {
		t.Fatal(err)
	}
	// Subject B captures something unrelated.
	exB := Exchange{
		UserText: "the billing service uses stripe webhooks for invoices", AssistantText: "stripe webhooks drive invoice state",
		SubjectID: "userB", SessionID: "s2", ProjectID: "other",
	}
	if err := hook.processTurn(context.Background(), exB); err != nil {
		t.Fatal(err)
	}

	r := NewHybridRetriever(conn, emb, 0.1, 30*24*time.Hour)
	a := "userA"
	got, err := r.Retrieve(context.Background(), Query{Text: "synthesis summaries chunks", K: 10, SubjectID: &a}, 20)
	if err != nil {
		t.Fatal(err)
	}
	var recalledA, leakedB bool
	for _, c := range got {
		if c.SubjectID != nil && *c.SubjectID == "userA" {
			recalledA = true
		}
		if c.SubjectID != nil && *c.SubjectID == "userB" {
			leakedB = true
		}
	}
	if !recalledA {
		t.Fatalf("subject A's captured entry was not recalled; got %v", ids(got))
	}
	if leakedB {
		t.Fatalf("subject B's entry leaked into subject A's recall; got %v", ids(got))
	}
}
```

> Uses `stubEmbedder`/`ids` from Task 5. If the hybrid `@@@` BM25 arm returns nothing for the stub (identical vectors make the vec arm dominate), recall still surfaces A's row via the vector arm; the assertion only requires A present and B absent.

- [ ] **Step 2: Run (PASS with DB / SKIP without)**

Run: `set -a; . ./.env; set +a; go test -p 1 -run 'TestCaptureRecall_MultiSession' ./internal/memory/`
Expected: PASS (or SKIP).

- [ ] **Step 3: Commit**

```bash
git add internal/memory/capture_eval_test.go
git commit -m "test(memory/6a): multi-session capture→recall→isolation eval"
```

---

## Task 9: Full verification + smoke target + decay-eligibility check

**Files:**
- Modify: `Makefile` (add `smoke-capture` target)
- Verify only: existing sweep test covers decay; add one assertion if missing.

- [ ] **Step 1: Confirm project rows are decay-eligible (Phase-4 sweep archives an old unaccessed project row, pinned/high survives)**

Inspect `internal/memory/sweep_test.go`. If a project-row archive case exists, no change. If not, add:

```go
func TestSweep_ArchivesOldProjectRow_KeepsPinned(t *testing.T) {
	conn := freshDB(t)
	s := NewPgStore(conn)
	subj := "userA"
	old := time.Now().Add(-90 * 24 * time.Hour).UTC()
	mk := func(id string, pinned bool, imp float64) Chunk {
		sess, proj, persp := "s", "ptolemy", "factual"
		return Chunk{ID: id, Content: id + " content", Embedding: []float32{1, 0, 0, 0}, PublishedAt: old,
			Scope: "project", Importance: imp, Pinned: pinned, SubjectID: &subj, SessionID: &sess, ProjectID: &proj, Perspective: &persp}
	}
	_ = s.Upsert(context.Background(), []Chunk{mk("decayme", false, 0.2), mk("keepme", true, 0.2)})
	// Backdate last_accessed_at so the decay score falls below threshold.
	_, _ = conn.Exec(context.Background(), `UPDATE chunks SET last_accessed_at=$1 WHERE id IN ('decayme','keepme')`, old)
	sw := NewSweeper(conn, GCConfig{DecayLambda: 0.05, ArchiveThreshold: 0.1})
	if err := sw.archiveDecayed(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get(context.Background(), []string{"decayme", "keepme"})
	for _, c := range got {
		if c.ID == "decayme" && c.Status != "archived" {
			t.Errorf("decayme should be archived, got %s", c.Status)
		}
		if c.ID == "keepme" && c.Status != "active" {
			t.Errorf("pinned keepme should stay active, got %s", c.Status)
		}
	}
}
```

(`Chunk.Pinned` already exists; `Get` returns `Status`.)

- [ ] **Step 2: Add a BRAIN-extractor smoke target to the Makefile**

```make
# Phase 6a capture smoke: runs the REAL BRAIN_* extractor against a sample
# exchange and prints the extracted entries. Requires .env (BRAIN_*).
smoke-capture: build
	@set -a; . ./.env; set +a; \
	  go test -p 1 -tags=smoke -run TestExtractorSmoke ./internal/memory/ -v
```

(If a `smoke` build tag pattern is not yet used in the repo, instead document running the extractor via a tiny `cmd/memory-demo` subcommand; keep the committed change to just the Makefile comment + target, and gate the smoke test file with `//go:build smoke`.)

- [ ] **Step 3: Full suite + eval**

Run:
```bash
go build ./...
set -a; . ./.env; set +a
go test -p 1 ./internal/memory/...
make eval-memory
```
Expected: build OK; all tests PASS; eval overall recall **1.000**.

- [ ] **Step 4: Commit**

```bash
git add internal/memory/sweep_test.go Makefile
git commit -m "test(memory/6a): project-row decay eligibility + capture smoke target"
```

---

## Acceptance criteria mapping (verify each before declaring 6a done)

- Earlier-turn fact recalled in a later session → **Task 8**.
- Trivial turns produce no entries → **Task 2 + Task 4** (`TestCapture_GateSkipsTrivial`).
- Obvious secrets not persisted → **Task 2** (`TestGate_FlagsSecrets`) + gate runs first in `processTurn`.
- Capture async / `Enqueue` non-blocking → **Task 4** (`TestCapture_EnqueueDoesNotBlockAndDrops`).
- Self-contained entries; dangling rejected; coref-resolved NOT wrongly rejected → **Task 3** (`TestExtractor_RejectsDanglingPronoun`, `TestExtractor_GroundingAllowsCorefResolved`).
- Changed structured fact routes through `Supersede()` → **Task 4** `processTurn` ladder (reuses Phase 5; add a `processTurn` supersession test against DB if desired).
- Subject isolation + schema CHECK → **Task 1** (`TestPgStore_ProjectRowRequiresSubject`) + **Task 5** (`TestHybrid_SubjectIsolation`).
- Reinforce on recall (`access_count` bumped) → unchanged `Orchestrator.Answer` (already wired); no new code.
- Project rows decay-eligible → **Task 9** (`TestSweep_ArchivesOldProjectRow_KeepsPinned`).
- All-global eval stays 1.000 → **Task 5 Step 4 / Task 9 Step 3** (`make eval-memory`).

## Deferred to 6b (do NOT build here)

Consolidator/synthesis, dual-circuit recall, `project_id` *filtering*, full decay-rank blend for project rows, synonym/alias expansion, durable capture outbox, inline extract retry, richer importance scoring. (`project_id` is *populated* in 6a — Task 4 — but not filtered.)
