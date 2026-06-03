# Memory Phase 6b Implementation Plan — consolidation, dual-circuit recall, decay blend, synthesis eval

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the synthesis layer on top of 6a — periodic LLM consolidation of a subject's project atoms into a durable summary (with a mandatory revision/conflict-resolution step), dual-circuit recall that prefers a matching summary, `project_id` filtering, the full decay-rank blend for project rows, and config-gated alias expansion — gated by a multi-session synthesis eval set, without regressing the all-global eval (must stay 1.000).

**Architecture:** Synthesis summaries are `chunks` rows tagged `metadata.kind="synthesis"` (6a already tags atoms `kind="atom"`), carrying `subject_id`/`project_id`/`source_ids`, embedded + BM25-indexed so recall finds them natively. A `Consolidator` (BRAIN LLM, versioned `go:embed` prompt) runs as a per-subject buffered/timed batch job and reconciles each new summary against the prior summary + `superseded_by` on its sources, routing contradictions through the existing `Supersede()`. Recall gains `project_id` filtering and a decay-rank blend — both **eval-neutral by construction** (gated on nil project / multiply only project rows by `1.0`). Alias expansion is the one eval-affecting change: config-gated (default off), measured. A dedicated eval DB (`MEMORY_EVAL_DATABASE_URL`) ends the unit-test (vector(4)) vs eval (vector(768)) dim conflict.

**Tech Stack:** Go 1.25, pgx v5, pgvector-go, ParadeDB (`@@@`/`paradedb.score`), zerolog, `go:embed`, BRAIN_* `/v1/chat/completions`.

**Pre-reqs from 6a (reuse, do not rebuild):** `Chunk` scoping fields + `Store.{Upsert,Get,LookupFact,Reinforce,Supersede,History}`; `HybridRetriever` (subject isolation `$8`, project recency tiebreak, projects scoping columns onto `RetrievedChunk`); `MMRContextBuilder`; `Extractor`/`ChatClient`/`OpenAIGenerator.Complete`; `PerTurnCaptureHook` (stamps `metadata.kind="atom"`, `extractor_version`); `normalizeContent`, `normalizeTokens`; `runLoop` + `Sweeper`/`MaybeStartSweep` pattern; the decay formula in `sweep.go` `archiveDecayed` (`importance * exp(-lambda*days_since_access/(1+access_count))`).

**Confirmed decisions (see spec addendum):** (1) synthesis eval = full runner + ~12 seed scenarios, growable; (2) dedicated eval DB; (3) honor explicit `Query.ProjectID`, defer current-project resolution.

**Testing notes:**
- Unit tests (alias map, MMR dual-circuit partition, consolidator with fake LLM, config) need no DB.
- DB integration tests use the existing `freshDB(t)`/`requirePG(t)` (skip without `DATABASE_URL`; dim=4).
- The eval / synthesis-eval binaries use `MEMORY_EVAL_DATABASE_URL` (fallback `DATABASE_URL`) at the real `EMBEDDING_DIM` (768). Run with `set -a; . ./.env; set +a`.
- Eval-neutrality: the all-global eval builds `Query{Text,K}` (nil subject/project). Tasks 2 & 3 MUST keep `make eval-memory` at **1.000**; Task 4 (alias) is measured and ships default-off.

---

## File Structure

- `internal/memory/config.go` — **modify**. Add `AliasExpansion bool` + `ConsolidateConfig` (Enabled/Buffer/Interval/MinAtoms); load + validate.
- `cmd/memory-eval/main.go` — **modify**. Override `cfg.DatabaseURL` from `MEMORY_EVAL_DATABASE_URL` when set (eval binary only).
- `internal/memory/hybrid_retriever.go` — **modify** (Tasks 2,3,4 in order). `project_id` filter (`$9`), decay blend (`$10`), alias-expanded bm25 query.
- `internal/memory/aliases.go` + `_test.go` — **create**. In-repo alias map + `expandAliases`.
- `internal/memory/context_builder.go` — **modify**. `MMRContextBuilder` dual-circuit (prefer synthesis, fill atoms).
- `internal/memory/prompts/consolidate_v1.txt` — **create**. `go:embed`ed consolidation prompt.
- `internal/memory/consolidator.go` + `_test.go` — **create**. `Consolidator`, `ConsolidateSubjectProject`, `Run` trigger.
- `internal/memory/module.go` — **modify**. `MaybeStartConsolidator` (mirrors `MaybeStartSweep`).
- `internal/memory/consolidator_smoke_test.go` — **create** (`//go:build smoke`). Real BRAIN consolidation smoke.
- `internal/memory/eval/synth.go` + `internal/memory/eval/synth_test.go` — **create**. Synthesis scenario types, runner (recall@k + LLM-judge), pure-logic tests.
- `internal/memory/eval/testdata/synth_scenarios.json` — **create**. ~12 multi-session scenarios.
- `cmd/memory-synth-eval/main.go` — **create**. Thin CLI for the synthesis eval (uses `MEMORY_EVAL_DATABASE_URL`).
- `Makefile` — **modify**. `eval-synth` target + `MEMORY_EVAL_DATABASE_URL` plumbing; `smoke-consolidate`.

**Shared identifiers (consistent across tasks):**
- `metadata` keys: `kind` (`"atom"`|`"synthesis"`), `source_ids` (`[]string`), `consolidator_version` (`"consolidate_v1"`).
- `ConsolidatorVersion = "consolidate_v1"`; `synthImportance = 0.9`.
- Synthesis chunk id: `"synth:" + hex(sha256(subject|project|content))[:24]`.
- New hybrid query params: `$9` = projectID (nil→NULL), `$10` = decay lambda (float seconds-free, days-based).

---

## Task 1: Dedicated eval DB + config knobs

**Files:**
- Modify: `internal/memory/config.go`
- Modify: `cmd/memory-eval/main.go`
- Test: `internal/memory/config_test.go`

- [ ] **Step 1: Add config fields**

In `MemoryConfig` (after the 6a knobs at line ~43):

```go
	// Phase 6b knobs.
	AliasExpansion bool // RAG_ALIAS_EXPANSION (default false; eval-affecting, measured)
	Consolidate    ConsolidateConfig
```

Add the struct near `GCConfig`:

```go
// ConsolidateConfig holds the Phase-6b consolidation knobs.
type ConsolidateConfig struct {
	Enabled  bool          // CONSOLIDATE_ENABLED (default false)
	Buffer   int           // CONSOLIDATE_BUFFER new atoms before a (subject,project) is consolidated (default 20)
	Interval time.Duration // CONSOLIDATE_INTERVAL timer fallback (default 1h)
	MinAtoms int           // CONSOLIDATE_MIN_ATOMS minimum atoms to bother summarizing (default 3)
}
```

In `LoadConfig`, after the 6a block:

```go
	cfg.AliasExpansion = boolEnv("RAG_ALIAS_EXPANSION", false)
	cfg.Consolidate = ConsolidateConfig{
		Enabled:  boolEnv("CONSOLIDATE_ENABLED", false),
		Buffer:   intEnv("CONSOLIDATE_BUFFER", 20),
		Interval: durationEnv("CONSOLIDATE_INTERVAL", time.Hour),
		MinAtoms: intEnv("CONSOLIDATE_MIN_ATOMS", 3),
	}
	if cfg.Consolidate.Interval < time.Second {
		return MemoryConfig{}, fmt.Errorf("CONSOLIDATE_INTERVAL must be >= 1s, got %v", cfg.Consolidate.Interval)
	}
```

(`intEnv` already floors at >0→fallback, so Buffer/MinAtoms default cleanly.)

- [ ] **Step 2: Failing config test**

Append to `config_test.go` (mirror the inline `t.Setenv` of the six required vars used by `TestLoadConfig_CaptureAndMMRDefaults`):

```go
func TestLoadConfig_Phase6bDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("EMBEDDING_BASE_URL", "http://e")
	t.Setenv("EMBEDDING_MODEL", "m")
	t.Setenv("EMBEDDING_DIM", "768")
	t.Setenv("BRAIN_BASE_URL", "http://l")
	t.Setenv("BRAIN_MODEL", "lm")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AliasExpansion {
		t.Error("AliasExpansion must default false (eval-affecting)")
	}
	if cfg.Consolidate.Enabled {
		t.Error("Consolidate.Enabled must default false")
	}
	if cfg.Consolidate.Buffer != 20 || cfg.Consolidate.MinAtoms != 3 || cfg.Consolidate.Interval != time.Hour {
		t.Errorf("consolidate defaults wrong: %+v", cfg.Consolidate)
	}
}
```

- [ ] **Step 3: Run → expect FAIL → implement Step 1 → PASS**

Run: `go test -p 1 -run TestLoadConfig_Phase6bDefaults ./internal/memory/` → PASS.

- [ ] **Step 4: Wire dedicated eval DB in `cmd/memory-eval/main.go`**

Right after `cfg, err := memory.LoadConfig()` (line ~63), before `NewModule`:

```go
	if evalURL := strings.TrimSpace(os.Getenv("MEMORY_EVAL_DATABASE_URL")); evalURL != "" {
		cfg.DatabaseURL = evalURL // eval runs against a dedicated DB so unit-test freshDB (dim=4) and eval (dim=768) stop clobbering each other
	}
```

(`os` and `strings` are already imported in this file.)

- [ ] **Step 5: Build + verify**

Run: `go build ./... && go test -p 1 -run TestLoadConfig ./internal/memory/`
Expected: build OK, tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/memory/config.go internal/memory/config_test.go cmd/memory-eval/main.go
git commit -m "feat(memory/6b): config knobs (alias, consolidate) + dedicated eval DB override

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: `project_id` filtering in recall (eval-neutral)

**Files:**
- Modify: `internal/memory/hybrid_retriever.go`
- Test: `internal/memory/recall_isolation_test.go` (extend)

- [ ] **Step 1: Add `$9` project clause to `hybridRrfQuery`**

In both CTEs add (after the `$8` subject clause):
```sql
      AND ($9::text IS NULL OR project_id IS NULL OR project_id = $9)
```
In the outer `WHERE` add (after the `c.subject_id` clause):
```sql
  AND ($9::text IS NULL OR c.project_id IS NULL OR c.project_id = $9)
```

> Why eval-neutral: the eval passes `ProjectID=nil` → `$9 = NULL` → `$9::text IS NULL` is TRUE → the whole clause is TRUE for every row → no filtering. When set to `'X'`, it admits global rows (`project_id IS NULL`) + project-X rows only. Combined with the `$8` subject clause this yields "global KB + this subject's rows in this project."

- [ ] **Step 2: Bind `$9` in `Retrieve`**

After the `var subj any ...` block add:
```go
	var proj any
	if q.ProjectID != nil {
		proj = *q.ProjectID
	}
```
Append `proj` as the 9th argument to the `r.conn.Query(ctx, hybridRrfQuery, ... , subj, proj)` call.

- [ ] **Step 3: Failing integration test**

Append to `recall_isolation_test.go`:

```go
func TestHybrid_ProjectScope(t *testing.T) {
	conn := freshDB(t)
	s := NewPgStore(conn)
	emb := fakeEmbedder{vecs: [][]float32{{1, 0, 0, 0}}}
	r := NewHybridRetriever(conn, emb, 0.1, 30*24*time.Hour)
	subj := "userA"
	now := time.Now().UTC()
	mk := func(id, content, project string) Chunk {
		ss, pp, sess, pr := subj, "factual", "s", project
		return Chunk{ID: id, Content: content, Embedding: []float32{1, 0, 0, 0}, PublishedAt: now,
			Scope: "project", Importance: 0.7, SubjectID: &ss, SessionID: &sess, ProjectID: &pr, Perspective: &pp}
	}
	if err := s.Upsert(context.Background(), []Chunk{
		mk("pa", "alpha config detail one two three", "projA"),
		mk("pb", "alpha config detail one two three", "projB"),
	}); err != nil {
		t.Fatal(err)
	}
	projA := "projA"
	got, err := r.Retrieve(context.Background(), Query{Text: "alpha config", K: 10, SubjectID: &subj, ProjectID: &projA}, 20)
	if err != nil {
		t.Fatal(err)
	}
	var sawA, sawB bool
	for _, c := range got {
		if c.ID == "pa" {
			sawA = true
		}
		if c.ID == "pb" {
			sawB = true
		}
	}
	if !sawA {
		t.Fatalf("project-A query must return projA row; got %v", ids(got))
	}
	if sawB {
		t.Fatalf("project-A query must NOT return projB row; got %v", ids(got))
	}
}
```

- [ ] **Step 4: Run + eval-neutrality check**

Run: `set -a; . ./.env; set +a; go test -p 1 -run 'TestHybrid_ProjectScope|TestHybrid_SubjectIsolation|TestCaptureRecall' ./internal/memory/` → PASS.
Then full suite: `go test -p 1 ./internal/memory/` → PASS (the 6a `TestHybrid_SubjectIsolation`/`TestCaptureRecall_MultiSession` still pass — `$9 NULL` is a no-op there).

- [ ] **Step 5: Commit**

```bash
git add internal/memory/hybrid_retriever.go internal/memory/recall_isolation_test.go
git commit -m "feat(memory/6b): honor Query.ProjectID in recall (eval-neutral when unset)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Decay-rank blend for project rows (eval-neutral)

**Files:**
- Modify: `internal/memory/hybrid_retriever.go`
- Test: `internal/memory/hybrid_retriever_test.go` (extend) — note check the file's existing test helpers/names first.

- [ ] **Step 1: Multiply project-row score by decay; add `$10` lambda**

Wrap the existing score expression in parens and multiply by a project-only decay factor. Replace the SELECT score expression:
```sql
SELECT c.id, c.content, c.metadata, COALESCE(c.source,''), c.published_at,
       c.subject_id, c.session_id, c.project_id,
       ( COALESCE(1.0 / (60 + b.rank), 0)
       + COALESCE(1.0 / (60 + v.rank), 0)
       + $6 * exp(-extract(epoch FROM $5 - c.published_at) / $7) )
     * CASE WHEN c.scope = 'project' AND NOT c.pinned
            THEN c.importance * exp(-$10::float8 * extract(epoch FROM now() - c.last_accessed_at) / 86400
                                    / (1 + c.access_count))
            ELSE 1.0 END AS score
```

> Why eval-neutral: global rows (and pinned) take the `ELSE 1.0` branch → `(oldscore) * 1.0`, which is byte-identical to the prior score in IEEE float. The all-global eval has no project rows, so its ranking is unchanged → 1.000 holds. The decay factor is the **same formula** as `sweep.go archiveDecayed`, so a row's rank sinks consistently *before* the sweep archives it.

- [ ] **Step 2: Bind `$10` from the configured lambda**

The retriever needs the decay lambda. Add a field + constructor param:
```go
type HybridRetriever struct {
	conn            *pgx.Conn
	embedder        Embedder
	recencyWeight   float64
	recencyHalfLife time.Duration
	decayLambda     float64
}

func NewHybridRetriever(conn *pgx.Conn, e Embedder, recencyWeight float64, recencyHalfLife time.Duration) *HybridRetriever {
	return &HybridRetriever{conn: conn, embedder: e, recencyWeight: recencyWeight, recencyHalfLife: recencyHalfLife, decayLambda: 0.05}
}

// WithDecayLambda overrides the project-row decay dial (SPEC-GC §4 default 0.05).
func (r *HybridRetriever) WithDecayLambda(l float64) *HybridRetriever { r.decayLambda = l; return r }
```
Keep the existing 4-arg constructor signature (callers in `cmd/memory-eval` and tests pass 4 args). `NewModule` sets the lambda from config (Step 4). Append `r.decayLambda` as the 10th `Query(...)` arg.

- [ ] **Step 3: Failing test — stale project row sinks below a fresh one; global unaffected**

Append to `hybrid_retriever_test.go` (reuse its existing embedder stub / `freshDB`; if it uses `fakeEmbedder{vecs}`, match that):

```go
func TestHybrid_DecayBlend_StaleProjectRowSinks(t *testing.T) {
	conn := freshDB(t)
	s := NewPgStore(conn)
	emb := fakeEmbedder{vecs: [][]float32{{1, 0, 0, 0}}}
	r := NewHybridRetriever(conn, emb, 0.0, 30*24*time.Hour).WithDecayLambda(0.05)
	subj := "userA"
	now := time.Now().UTC()
	mk := func(id string, accessedDaysAgo float64) Chunk {
		ss, pp, sess, pr := subj, "factual", "s", "ptolemy"
		c := Chunk{ID: id, Content: "kafka retention policy detail", Embedding: []float32{1, 0, 0, 0}, PublishedAt: now,
			Scope: "project", Importance: 0.7, SubjectID: &ss, SessionID: &sess, ProjectID: &pr, Perspective: &pp}
		return c
	}
	if err := s.Upsert(context.Background(), []Chunk{mk("fresh", 0), mk("stale", 0)}); err != nil {
		t.Fatal(err)
	}
	// Backdate "stale" so its decay factor is much smaller.
	if _, err := conn.Exec(context.Background(),
		`UPDATE chunks SET last_accessed_at = now() - interval '120 days' WHERE id='stale'`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(context.Background(),
		`UPDATE chunks SET last_accessed_at = now() WHERE id='fresh'`); err != nil {
		t.Fatal(err)
	}
	got, err := r.Retrieve(context.Background(), Query{Text: "kafka retention", K: 10, SubjectID: &subj}, 20)
	if err != nil {
		t.Fatal(err)
	}
	// Both match identically on BM25/vector; decay must order fresh before stale.
	var iFresh, iStale = -1, -1
	for i, c := range got {
		if c.ID == "fresh" {
			iFresh = i
		}
		if c.ID == "stale" {
			iStale = i
		}
	}
	if iFresh == -1 || iStale == -1 {
		t.Fatalf("both rows should be retrieved; got %v", ids(got))
	}
	if iFresh > iStale {
		t.Fatalf("fresh row should rank before stale after decay blend; fresh@%d stale@%d", iFresh, iStale)
	}
}
```

- [ ] **Step 4: Wire lambda in `module.go`**

In `NewModule`, set the retriever's decay lambda from GC config:
```go
		Retriever:      NewHybridRetriever(conn, embedder, cfg.RecencyWeight, cfg.RecencyHalfLife).WithDecayLambda(cfg.GC.DecayLambda),
```

- [ ] **Step 5: Run + eval-neutrality**

Run: `set -a; . ./.env; set +a; go test -p 1 -run 'TestHybrid' ./internal/memory/` → PASS.
Full suite: `go test -p 1 ./internal/memory/` → PASS.
(Eval baseline is measured in Task 9 against the dedicated DB; this step proves no Go-test regressions.)

- [ ] **Step 6: Commit**

```bash
git add internal/memory/hybrid_retriever.go internal/memory/hybrid_retriever_test.go internal/memory/module.go
git commit -m "feat(memory/6b): blend SPEC-GC decay into project-row rank (global byte-identical)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: Alias/synonym expansion (config-gated, default off)

**Files:**
- Create: `internal/memory/aliases.go`, `internal/memory/aliases_test.go`
- Modify: `internal/memory/hybrid_retriever.go`

- [ ] **Step 1: Failing test for `expandAliases`**

Create `internal/memory/aliases_test.go`:
```go
package memory

import "testing"

func TestExpandAliases_AddsKnownSynonyms(t *testing.T) {
	out := expandAliases("how does the GC work")
	// "gc" → appends "garbage collector"; original terms preserved.
	if out == "how does the GC work" {
		t.Fatalf("expected expansion, got unchanged %q", out)
	}
	if !containsFold(out, "garbage collector") || !containsFold(out, "GC") {
		t.Fatalf("expansion must keep original + add alias, got %q", out)
	}
}

func TestExpandAliases_NoMatchUnchanged(t *testing.T) {
	in := "completely unrelated query text"
	if out := expandAliases(in); out != in {
		t.Fatalf("no-alias query must be unchanged, got %q", out)
	}
}
```

- [ ] **Step 2: Run → FAIL → implement `aliases.go`**

```go
package memory

import "strings"

// aliasMap expands common terms whose synonyms BM25 treats as unrelated tokens.
// Keys are matched case-insensitively as whole words; values are appended to the
// lexical query. Keep small and in-repo (SPEC-GC §6); measure recall delta before
// trusting (ships behind RAG_ALIAS_EXPANSION, default off).
var aliasMap = map[string][]string{
	"gc":        {"garbage collector"},
	"rag":       {"retrieval augmented generation"},
	"bm25":      {"lexical search"},
	"pgvector":  {"vector search"},
	"oob":       {"out of band"},
}

func containsFold(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

// expandAliases appends known synonyms for any alias term present in the query,
// preserving the original text. Whole-word, case-insensitive match on the term.
func expandAliases(text string) string {
	lowerTokens := map[string]bool{}
	for _, t := range strings.Fields(strings.ToLower(text)) {
		lowerTokens[strings.Trim(t, ".,!?;:()")] = true
	}
	var add []string
	seen := map[string]bool{}
	for term, syns := range aliasMap {
		if lowerTokens[term] {
			for _, syn := range syns {
				if !seen[syn] && !containsFold(text, syn) {
					add = append(add, syn)
					seen[syn] = true
				}
			}
		}
	}
	if len(add) == 0 {
		return text
	}
	return text + " " + strings.Join(add, " ")
}
```

> `aliasMap` iteration order is nondeterministic; the test asserts substring presence, not order, so it's stable. If a future test needs deterministic output, sort `add` before joining.

- [ ] **Step 3: Gate the bm25 arm on the alias flag**

Add an `aliasExpansion bool` field + setter to `HybridRetriever`:
```go
// WithAliasExpansion toggles lexical-arm synonym expansion (default off).
func (r *HybridRetriever) WithAliasExpansion(on bool) *HybridRetriever { r.aliasExpansion = on; return r }
```
In `Retrieve`, compute the bm25 query text and pass it as `$1` (the vector arm keeps using the original `q.Text` for embedding):
```go
	bm25Text := q.Text
	if r.aliasExpansion {
		bm25Text = expandAliases(q.Text)
	}
```
Change the first `Query(...)` arg from `q.Text` to `bm25Text`. (The embedding is still computed from `q.Text` above — leave that call unchanged.)

In `NewModule`, chain `.WithAliasExpansion(cfg.AliasExpansion)` onto the retriever constructor.

- [ ] **Step 4: Run**

Run: `go test -p 1 -run 'TestExpandAliases|TestHybrid' ./internal/memory/` → PASS. `go build ./...` OK.
Full suite: `go test -p 1 ./internal/memory/` → PASS. (Default off → bm25Text == q.Text → eval unchanged. The recall delta with it ON is measured in Task 9.)

- [ ] **Step 5: Commit**

```bash
git add internal/memory/aliases.go internal/memory/aliases_test.go internal/memory/hybrid_retriever.go internal/memory/module.go
git commit -m "feat(memory/6b): config-gated alias expansion on the lexical arm (default off)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: Consolidator core — synthesis with mandatory revision

**Files:**
- Create: `internal/memory/prompts/consolidate_v1.txt`
- Create: `internal/memory/consolidator.go`, `internal/memory/consolidator_test.go`

- [ ] **Step 1: Create `internal/memory/prompts/consolidate_v1.txt`**

```
You maintain ONE durable topic summary for a user's work on a project, by consolidating their atomic memory entries into a single coherent answer to "how did I do X here".

You are given the current ACTIVE atomic entries and (optionally) the PREVIOUS summary. Produce the new summary.

Rules:
- RECONCILE, don't accumulate: if an atom contradicts the previous summary, the atom is newer truth — state the current fact and DROP the stale claim. Never keep both sides of a contradiction.
- SELF-CONTAINED and declarative; no pronouns without their noun; readable months later in isolation.
- Cite the atoms you used by their id in "source_ids".
- If nothing durable, return an empty content string.

Return ONLY JSON: {"content": string, "source_ids": [string]}

The next message contains the entries, formatted as:
PREVIOUS SUMMARY (may be empty):
<text>

ATOMS (id :: content):
<id> :: <content>
...
```

- [ ] **Step 2: Failing test (fake LLM)**

Create `internal/memory/consolidator_test.go`:
```go
package memory

import (
	"context"
	"testing"
)

func TestConsolidator_BuildPromptAndParse(t *testing.T) {
	atoms := []Chunk{
		{ID: "a1", Content: "the archive threshold is 0.1"},
		{ID: "a2", Content: "the sweep runs hourly"},
	}
	resp := `{"content":"On this project the GC archive threshold is 0.1 and the sweep runs hourly.","source_ids":["a1","a2"]}`
	c := NewConsolidator(nil, nil, fakeChat{resp: resp}, ConsolidateConfig{MinAtoms: 1})
	syn, err := c.synthesize(context.Background(), "", atoms)
	if err != nil {
		t.Fatal(err)
	}
	if syn.Content == "" || len(syn.SourceIDs) != 2 {
		t.Fatalf("bad synthesis: %+v", syn)
	}
}

func TestConsolidator_ParseHandlesFence(t *testing.T) {
	c := NewConsolidator(nil, nil, fakeChat{resp: "```json\n{\"content\":\"x summary text\",\"source_ids\":[\"a1\"]}\n```"}, ConsolidateConfig{})
	syn, err := c.synthesize(context.Background(), "prev", []Chunk{{ID: "a1", Content: "x"}})
	if err != nil || syn.Content != "x summary text" {
		t.Fatalf("fence parse failed: %+v err=%v", syn, err)
	}
}
```

(`fakeChat` is defined in `extractor_test.go` — reuse it.)

- [ ] **Step 3: Run → FAIL → implement `consolidator.go`**

```go
package memory

import (
	"context"
	"crypto/sha256"
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

// WithEmbedder sets the embedder (separate setter so the parse-only unit tests can
// construct a Consolidator without one).
func (c *Consolidator) WithEmbedder(e Embedder) *Consolidator { c.embedder = e; return c }

// synthesize builds the prompt, calls the LLM, and parses the JSON summary.
func (c *Consolidator) synthesize(ctx context.Context, prevSummary string, atoms []Chunk) (Synthesis, error) {
	var b strings.Builder
	for _, a := range atoms {
		fmt.Fprintf(&b, "%s :: %s\n", a.ID, a.Content)
	}
	user := "PREVIOUS SUMMARY (may be empty):\n" + prevSummary + "\n\nATOMS (id :: content):\n" + b.String()
	raw, err := c.chat.Complete(ctx, consolidatePromptTemplate, user)
	if err != nil {
		return Synthesis{}, fmt.Errorf("consolidate llm: %w", err)
	}
	return parseSynthesis(raw)
}

var synthFence = jsonFence // reuse the fenced-json regexp from extractor.go

func parseSynthesis(raw string) (Synthesis, error) {
	s := strings.TrimSpace(raw)
	if m := synthFence.FindStringSubmatch(s); m != nil {
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
	// No-op if the summary is unchanged (reinforce, don't duplicate/supersede-to-self).
	if hasPrior && normalizeContent(prior.Content) == normalizeContent(syn.Content) {
		return c.store.Reinforce(ctx, []string{prior.ID})
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
		return c.store.Supersede(ctx, []Chunk{row}, prior.ID) // revision: retire stale summary, link chain
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
```

Add `_ "embed"` is not needed (the `//go:embed` directive requires `import "embed"` only when using `embed.FS`; for `string` targets the blank import is needed). Match `extractor.go`: it uses `_ "embed"`. Add `_ "embed"` to the import block.

- [ ] **Step 4: Run unit tests → PASS**

Run: `go test -p 1 -run TestConsolidator ./internal/memory/` → PASS. `go build ./...` OK; `go vet ./internal/memory/` clean.

- [ ] **Step 5: DB integration test — revision supersedes the prior summary**

Append to `consolidator_test.go`:
```go
func TestConsolidator_RevisionSupersedesPriorSummary(t *testing.T) {
	conn := freshDB(t)
	store := NewPgStore(conn)
	emb := fakeEmbedder{vecs: [][]float32{{1, 0, 0, 0}}}
	subj, proj := "userA", "ptolemy"
	// Seed two atoms (one will be contradicted).
	mkAtom := func(id, content string) Chunk {
		ss, pr, pe := subj, proj, "factual"
		return Chunk{ID: id, Content: content, Embedding: []float32{1, 0, 0, 0}, PublishedAt: time.Now().UTC(),
			Scope: "project", Importance: 0.5, SubjectID: &ss, ProjectID: &pr, Perspective: &pe,
			Metadata: map[string]any{"kind": "atom"}}
	}
	if err := store.Upsert(context.Background(), []Chunk{mkAtom("a1", "archive threshold is 0.2")}); err != nil {
		t.Fatal(err)
	}
	// First consolidation → creates summary mentioning 0.2.
	c1 := NewConsolidator(conn, store, fakeChat{resp: `{"content":"archive threshold is 0.2 on ptolemy","source_ids":["a1"]}`}, ConsolidateConfig{MinAtoms: 1}).WithEmbedder(emb)
	if err := c1.ConsolidateSubjectProject(context.Background(), subj, proj); err != nil {
		t.Fatal(err)
	}
	// Correction atom + second consolidation → new summary mentioning 0.1.
	if err := store.Upsert(context.Background(), []Chunk{mkAtom("a2", "archive threshold is now 0.1")}); err != nil {
		t.Fatal(err)
	}
	c2 := NewConsolidator(conn, store, fakeChat{resp: `{"content":"archive threshold is 0.1 on ptolemy","source_ids":["a1","a2"]}`}, ConsolidateConfig{MinAtoms: 1}).WithEmbedder(emb)
	if err := c2.ConsolidateSubjectProject(context.Background(), subj, proj); err != nil {
		t.Fatal(err)
	}
	// Exactly one ACTIVE synthesis, and it states 0.1 (stale 0.2 summary superseded, not duplicated).
	var nActive int
	var content string
	if err := conn.QueryRow(context.Background(), `
		SELECT count(*), max(content) FROM chunks
		WHERE metadata->>'kind'='synthesis' AND status='active' AND subject_id=$1 AND project_id=$2`,
		subj, proj).Scan(&nActive, &content); err != nil {
		t.Fatal(err)
	}
	if nActive != 1 {
		t.Fatalf("want exactly 1 active synthesis, got %d", nActive)
	}
	if !containsFold(content, "0.1") || containsFold(content, "0.2") {
		t.Fatalf("active summary must reflect current truth 0.1, not stale 0.2: %q", content)
	}
}
```

Run: `set -a; . ./.env; set +a; go test -p 1 -run TestConsolidator ./internal/memory/` → PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/memory/prompts/consolidate_v1.txt internal/memory/consolidator.go internal/memory/consolidator_test.go
git commit -m "feat(memory/6b): consolidator core — synthesis with mandatory revision via Supersede

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: Consolidator trigger (buffer + timer) + module wiring

**Files:**
- Modify: `internal/memory/consolidator.go`
- Modify: `internal/memory/module.go`
- Test: `internal/memory/consolidator_test.go` (extend)

- [ ] **Step 1: Add the scan + `Run` loop to `consolidator.go`**

```go
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
```

- [ ] **Step 2: Failing test — buffer gating**

Append to `consolidator_test.go`:
```go
func TestConsolidator_DueRespectsBuffer(t *testing.T) {
	conn := freshDB(t)
	store := NewPgStore(conn)
	subj, proj := "userA", "ptolemy"
	mkAtom := func(id string) Chunk {
		ss, pr, pe := subj, proj, "factual"
		return Chunk{ID: id, Content: id + " content detail", Embedding: []float32{1, 0, 0, 0}, PublishedAt: time.Now().UTC(),
			Scope: "project", Importance: 0.5, SubjectID: &ss, ProjectID: &pr, Perspective: &pe, Metadata: map[string]any{"kind": "atom"}}
	}
	if err := store.Upsert(context.Background(), []Chunk{mkAtom("a1"), mkAtom("a2")}); err != nil {
		t.Fatal(err)
	}
	c := NewConsolidator(conn, store, fakeChat{resp: "{}"}, ConsolidateConfig{Buffer: 3, MinAtoms: 1})
	due, err := c.dueSubjectProjects(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Fatalf("2 atoms < buffer 3 → not due; got %v", due)
	}
	if err := store.Upsert(context.Background(), []Chunk{mkAtom("a3")}); err != nil {
		t.Fatal(err)
	}
	due, _ = c.dueSubjectProjects(context.Background())
	if len(due) != 1 {
		t.Fatalf("3 atoms >= buffer 3 → due; got %v", due)
	}
}
```

Run: `set -a; . ./.env; set +a; go test -p 1 -run TestConsolidator_DueRespectsBuffer ./internal/memory/` → PASS.

- [ ] **Step 3: `MaybeStartConsolidator` in `module.go`**

Mirror `MaybeStartSweep` exactly (gate on `CONSOLIDATE_ENABLED`; require `DATABASE_URL`; build conn + migrate; construct `NewConsolidator(conn, NewPgStore(conn), NewOpenAIGenerator(cfg.LLMBaseURL,cfg.LLMModel,""), cfg.Consolidate).WithEmbedder(NewOpenAIEmbedder(...))`; start `Run` in a goroutine; return cleanup that cancels + waits on `done` with a 5s timeout then closes conn):

```go
// MaybeStartConsolidator mirrors MaybeStartSweep: if CONSOLIDATE_ENABLED and a
// DATABASE_URL is set, it starts the batch consolidation loop bound to ctx.
func MaybeStartConsolidator(ctx context.Context) (cleanup func(), enabled bool, err error) {
	if !boolEnv("CONSOLIDATE_ENABLED", false) {
		return nil, false, nil
	}
	if strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		return nil, true, fmt.Errorf("CONSOLIDATE_ENABLED=true but DATABASE_URL is not set")
	}
	cfg, err := LoadConfig()
	if err != nil {
		return nil, true, fmt.Errorf("memory config: %w", err)
	}
	conn, err := pgx.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, true, fmt.Errorf("connect postgres: %w", err)
	}
	if err := ApplyMigrations(ctx, conn, cfg.EmbeddingDim); err != nil {
		_ = conn.Close(ctx)
		return nil, true, fmt.Errorf("migrate: %w", err)
	}
	cons := NewConsolidator(conn, NewPgStore(conn),
		NewOpenAIGenerator(cfg.LLMBaseURL, cfg.LLMModel, ""), cfg.Consolidate).
		WithEmbedder(NewOpenAIEmbedder(cfg.EmbeddingBaseURL, cfg.EmbeddingModel, cfg.EmbeddingAPIKey))
	cctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go cons.Run(cctx, done)
	cleanup = func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			log.Error().Msg("consolidator did not stop within 5s; closing connection anyway")
		}
		_ = conn.Close(context.Background())
	}
	return cleanup, true, nil
}
```

- [ ] **Step 4: Build + full suite**

Run: `go build ./... && set -a; . ./.env; set +a; go test -p 1 ./internal/memory/` → PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/memory/consolidator.go internal/memory/module.go internal/memory/consolidator_test.go
git commit -m "feat(memory/6b): buffered+timed consolidator trigger + MaybeStartConsolidator

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: Dual-circuit recall in `MMRContextBuilder`

**Files:**
- Modify: `internal/memory/context_builder.go`
- Test: `internal/memory/context_builder_test.go` (extend)

- [ ] **Step 1: Failing test — synthesis surfaced first, atoms fill via MMR**

Append to `context_builder_test.go`:
```go
func TestMMR_PrefersSynthesisThenAtoms(t *testing.T) {
	chunks := []RetrievedChunk{
		{Chunk: Chunk{ID: "atom1", Content: "the sweep archives stale rows", Metadata: map[string]any{"kind": "atom"}}, Score: 0.9},
		{Chunk: Chunk{ID: "syn1", Content: "how the GC works: decay then archive then purge", Metadata: map[string]any{"kind": "synthesis"}}, Score: 0.5},
		{Chunk: Chunk{ID: "atom2", Content: "purge deletes dead rows after grace", Metadata: map[string]any{"kind": "atom"}}, Score: 0.8},
	}
	b := MMRContextBuilder{Lambda: 0.7, K: 2, MaxRunes: 6000}
	pc := b.Build(Query{Text: "how does the GC work"}, chunks)
	if len(pc.SourceIDs) == 0 || pc.SourceIDs[0] != "syn1" {
		t.Fatalf("synthesis must be surfaced first; got %v", pc.SourceIDs)
	}
	// Remaining slot filled by an atom via MMR.
	if len(pc.SourceIDs) < 2 {
		t.Fatalf("expected synthesis + at least one atom, got %v", pc.SourceIDs)
	}
}

func TestMMR_NoSynthesisUnchanged(t *testing.T) {
	// Without any synthesis row, behavior is the pure-atom MMR from 6a.
	chunks := []RetrievedChunk{
		{Chunk: Chunk{ID: "a", Content: "alpha beta gamma"}, Score: 1.0},
		{Chunk: Chunk{ID: "b", Content: "delta epsilon zeta"}, Score: 0.8},
	}
	b := MMRContextBuilder{Lambda: 0.7, K: 2, MaxRunes: 6000}
	pc := b.Build(Query{Text: "q"}, chunks)
	if len(pc.SourceIDs) != 2 {
		t.Fatalf("want 2, got %v", pc.SourceIDs)
	}
}
```

- [ ] **Step 2: Run → FAIL → implement dual-circuit in `Build`**

Replace `MMRContextBuilder.Build` so it partitions synthesis vs atoms, emits the top synthesis first, then MMR-selects atoms for the remaining slots:

```go
func (b MMRContextBuilder) Build(q Query, chunks []RetrievedChunk) PromptContext {
	// Dual-circuit: a matching synthesis summary is the pre-compressed "how"; surface
	// the highest-scoring one first, then fill remaining slots with distinct atoms via MMR.
	var synth []RetrievedChunk
	var atoms []RetrievedChunk
	for _, c := range chunks {
		if kind, _ := c.Metadata["kind"].(string); kind == "synthesis" {
			synth = append(synth, c)
		} else {
			atoms = append(atoms, c)
		}
	}
	var ordered []RetrievedChunk
	k := b.K
	if len(synth) > 0 {
		top := synth[0] // candidates arrive ranked; the first synthesis is the best-ranked
		for _, s := range synth {
			if s.Score > top.Score {
				top = s
			}
		}
		ordered = append(ordered, top)
		if k > 0 {
			k--
		}
	}
	ordered = append(ordered, selectMMR(atoms, b.Lambda, k)...)

	var body strings.Builder
	var ids []string
	used := 0
	for i, c := range ordered {
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
```

> Eval-neutral: the eval bypasses `ContextBuilder` entirely (`RunRetrieval` → `Retrieve`). With no synthesis rows, `synth` is empty and behavior reduces to the 6a atom-only MMR.

- [ ] **Step 3: Run → PASS**

Run: `go test -p 1 -run TestMMR ./internal/memory/` → PASS (new + existing `TestMMR_DropsNearDuplicates`, `TestMMR_EmptyCandidates`). `go vet` clean.

- [ ] **Step 4: Commit**

```bash
git add internal/memory/context_builder.go internal/memory/context_builder_test.go
git commit -m "feat(memory/6b): dual-circuit recall — prefer synthesis summary, fill atoms via MMR

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 8: Multi-session synthesis eval harness + ~12 seed scenarios

**Files:**
- Create: `internal/memory/eval/synth.go`, `internal/memory/eval/synth_test.go`
- Create: `internal/memory/eval/testdata/synth_scenarios.json`
- Create: `cmd/memory-synth-eval/main.go`
- Modify: `Makefile`

- [ ] **Step 1: Scenario types + pure scoring logic (`synth.go`)**

```go
package eval

// SynthScenario is a multi-session consolidation test: a build-up of atoms across
// sessions (some correcting earlier ones), then a procedural query whose consolidated
// answer should reflect current truth. ExpectKeywords must ALL appear in the recalled
// summary; RejectKeywords must NOT (they are superseded/stale claims).
type SynthScenario struct {
	ID             string   `json:"id"`
	Subject        string   `json:"subject"`
	Project        string   `json:"project"`
	Sessions       [][]Turn `json:"sessions"`        // each session is a list of turns (user+assistant)
	Query          string   `json:"query"`           // the procedural recall query
	ExpectKeywords []string `json:"expect_keywords"` // must appear in the consolidated answer
	RejectKeywords []string `json:"reject_keywords"` // stale claims that must NOT appear
}

type Turn struct {
	User      string `json:"user"`
	Assistant string `json:"assistant"`
}

// ScoreSummary returns (allExpectedPresent, noRejectedPresent) for a recalled summary
// string, case-insensitive. Pure — unit-tested without a DB or LLM.
func ScoreSummary(summary string, expect, reject []string) (bool, bool) {
	lower := toLower(summary)
	for _, e := range expect {
		if !contains(lower, toLower(e)) {
			return false, true
		}
	}
	for _, r := range reject {
		if contains(lower, toLower(r)) {
			return true, false
		}
	}
	return true, true
}
```

Add small `toLower`/`contains` helpers (or use `strings`); keep `synth.go` self-contained. Add `LoadSynthScenarios(path string) ([]SynthScenario, error)` mirroring `LoadSeed`.

- [ ] **Step 2: Failing pure test (`synth_test.go`)**

```go
package eval

import "testing"

func TestScoreSummary(t *testing.T) {
	ok, clean := ScoreSummary("archive threshold is 0.1 and sweep is hourly", []string{"0.1", "hourly"}, []string{"0.2"})
	if !ok || !clean {
		t.Fatalf("want pass+clean, got ok=%v clean=%v", ok, clean)
	}
	ok, clean = ScoreSummary("archive threshold is 0.2", []string{"0.1"}, []string{"0.2"})
	if ok || clean {
		t.Fatalf("stale summary must fail both, got ok=%v clean=%v", ok, clean)
	}
}

func TestLoadSynthScenarios(t *testing.T) {
	ss, err := LoadSynthScenarios("testdata/synth_scenarios.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(ss) < 12 {
		t.Fatalf("want >= 12 seed scenarios, got %d", len(ss))
	}
	for _, s := range ss {
		if s.Query == "" || len(s.ExpectKeywords) == 0 || len(s.Sessions) == 0 {
			t.Fatalf("scenario %s malformed", s.ID)
		}
	}
}
```

Run: `go test -p 1 -run 'TestScoreSummary|TestLoadSynthScenarios' ./internal/memory/eval/` → FAIL (missing func + file), then PASS after Step 1 + Step 3.

- [ ] **Step 3: Author `testdata/synth_scenarios.json` (≥12 scenarios)**

Write at least 12 scenarios. Each: 2–3 sessions of turns building a topic, at least 3 with a mid-stream correction (so `reject_keywords` exercises supersession), then a procedural `query` with `expect_keywords` (current truth) and `reject_keywords` (superseded claims). Example element:
```json
{
  "id": "gc-threshold-correction",
  "subject": "userA",
  "project": "ptolemy",
  "sessions": [
    [{"user": "set the GC archive threshold to 0.2", "assistant": "the sweep will archive a project row when its decay score drops below 0.2"}],
    [{"user": "actually change the archive threshold to 0.1", "assistant": "updated: the sweep archives when the decay score drops below 0.1"}]
  ],
  "query": "what is the GC archive threshold on this project",
  "expect_keywords": ["0.1"],
  "reject_keywords": ["0.2"]
}
```
Cover varied topics (retention, recall ranking, auth, deploy target, schema decisions) so the set isn't monotone. Keep keywords unambiguous and present in the assistant text so the captured atom is grounded.

> **Below the spec's 20–40 target by design (confirmed decision #1):** this is the growable seed set; append more over time. Note this in a header comment of the JSON is not possible (JSON has no comments) — instead document it in `synth.go`'s package doc and the Makefile target.

- [ ] **Step 4: Live runner (`RunSynthEval`) + CLI (`cmd/memory-synth-eval/main.go`)**

`RunSynthEval(ctx, orch *memory.Orchestrator, cons *memory.Consolidator, scenarios []SynthScenario) (passed int, total int, failures []string, err error)`: for each scenario — capture every turn via the orchestrator's capture path (use `memory.NewCaptureHookFromConfig`-style construction, or directly call a capture hook's `processTurn` for each turn with a `cannedExtractor`-free **real** extractor since this is the live eval) → run `cons.ConsolidateSubjectProject(subject, project)` → query recall scoped to (subject,project) → take the recalled synthesis summary's content → `ScoreSummary`. Count pass/fail; collect failure descriptions.

The CLI mirrors `cmd/memory-eval/main.go`: `LoadConfig`, override `cfg.DatabaseURL` from `MEMORY_EVAL_DATABASE_URL`, build the module + a real `Consolidator` (BRAIN), load scenarios, run, print `synth-eval: passed N/M` and each failure. This uses the live BRAIN LLM, so it is NOT part of `go test`; it runs via `make eval-synth`.

> Keep `RunSynthEval` thin and put pure scoring in `ScoreSummary` (already unit-tested). The live runner is exercised by `make eval-synth`, not committed CI, because it needs the LLM.

- [ ] **Step 5: `Makefile` target**

```make
# Phase 6b synthesis eval. Uses the dedicated eval DB (MEMORY_EVAL_DATABASE_URL,
# falling back to DATABASE_URL) at the real EMBEDDING_DIM. Seed set is ~12 scenarios
# (growable toward the spec's 20-40). Needs .env (BRAIN_*, EMBEDDING_*).
eval-synth: build
	@set -a; . ./.env; set +a; \
	  $(BIN_DIR)/memory-synth-eval -scenarios internal/memory/eval/testdata/synth_scenarios.json
```
Add `go build -o $(BIN_DIR)/memory-synth-eval ./cmd/memory-synth-eval` to the `build` target.

- [ ] **Step 6: Run pure tests + build**

Run: `go build ./... && go test -p 1 ./internal/memory/eval/` → PASS (pure scenario + scoring tests; the live runner is not in `go test`).

- [ ] **Step 7: Commit**

```bash
git add internal/memory/eval/synth.go internal/memory/eval/synth_test.go internal/memory/eval/testdata/synth_scenarios.json cmd/memory-synth-eval/main.go Makefile
git commit -m "test(memory/6b): synthesis eval harness + ~12 seed scenarios + CLI

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 9: Measurement + smoke + final verification

**Files:**
- Create: `internal/memory/consolidator_smoke_test.go` (`//go:build smoke`)
- Modify: `Makefile` (`smoke-consolidate`)
- Verify only: eval baseline, decay/alias deltas, synthesis eval.

- [ ] **Step 1: Provision the dedicated eval DB**

Choose a dedicated DB (e.g. same server, db name `ptolemy_eval`) and export `MEMORY_EVAL_DATABASE_URL` (add it to `.env`). It starts empty; `NewModule` creates the schema at `EMBEDDING_DIM=768` on first run. This DB is **only** touched by the eval binaries, never by `freshDB` (which uses `DATABASE_URL`), so the vector(4)/vector(768) conflict is gone.

(If a separate DB cannot be created, the override falls back to `DATABASE_URL`; in that case a one-time `DROP TABLE chunks,chunk_audit,memory_schema_migrations CASCADE` is still needed before the eval — but the whole point of this task is to avoid that.)

- [ ] **Step 2: Eval baseline still 1.000 (decay + project filter eval-neutral)**

Run: `make eval-memory`
Expected: `mean recall@5 = 1.000 over 30 questions`. (Project filter + decay blend are eval-neutral; alias defaults off.)

- [ ] **Step 3: Alias-expansion delta (measured, gated)**

Run with alias on: `RAG_ALIAS_EXPANSION=true make eval-memory` and compare the overall + per-type recall to Step 2. Record the delta in the commit message / a short note. **Decision rule:** keep `RAG_ALIAS_EXPANSION` default **off** unless recall holds-or-improves AND a known alias query measurably benefits; the feature ships off-by-default regardless, so this is informational.

- [ ] **Step 4: BRAIN consolidation smoke (`//go:build smoke`)**

Create `internal/memory/consolidator_smoke_test.go`:
```go
//go:build smoke

package memory

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestConsolidatorSmoke(t *testing.T) {
	base := strings.TrimSpace(os.Getenv("BRAIN_BASE_URL"))
	model := strings.TrimSpace(os.Getenv("BRAIN_MODEL"))
	if base == "" || model == "" {
		t.Skip("BRAIN_* not set")
	}
	c := NewConsolidator(nil, nil, NewOpenAIGenerator(base, model, ""), ConsolidateConfig{MinAtoms: 1})
	syn, err := c.synthesize(context.Background(), "",
		[]Chunk{{ID: "a1", Content: "the archive threshold is 0.1"}, {ID: "a2", Content: "the sweep runs hourly"}})
	if err != nil {
		t.Fatalf("synthesize: %v", err)
	}
	t.Logf("summary=%q sources=%v", syn.Content, syn.SourceIDs)
	if strings.TrimSpace(syn.Content) == "" {
		t.Fatal("expected a non-empty summary from the real LLM")
	}
}
```
Add to `Makefile`:
```make
smoke-consolidate:
	@set -a; . ./.env; set +a; \
	  go test -p 1 -tags=smoke -run TestConsolidatorSmoke ./internal/memory/ -v
```
Run: `make smoke-consolidate` → logs a real summary; report it.

- [ ] **Step 5: Synthesis eval green**

Run: `make eval-synth`
Expected: `synth-eval: passed N/M` with N == M (all ~12 seed scenarios pass). If any fail, inspect the failure lines (recalled summary vs expect/reject keywords) and fix the consolidator/recall — do NOT loosen `ScoreSummary` or delete scenarios to go green. Record the pass count.

- [ ] **Step 6: Full repo verification**

Run:
```bash
go build ./...
set -a; . ./.env; set +a
go test -p 1 ./...
```
Expected: build OK; all packages PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/memory/consolidator_smoke_test.go Makefile
git commit -m "test(memory/6b): consolidation smoke target + eval/synth verification

eval-memory 1.000 (decay+project filter eval-neutral); alias delta <recorded>;
synth-eval passed N/M.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Acceptance criteria mapping (verify before declaring 6b done)

- Synthesis runs on a timer/threshold with a revision step resolving contradictions → **Task 5** (`TestConsolidator_RevisionSupersedesPriorSummary`) + **Task 6** (`TestConsolidator_DueRespectsBuffer`, `Run`).
- "How did I implement X" returns a coherent consolidated answer, not fragments → **Task 7** (dual-circuit `TestMMR_PrefersSynthesisThenAtoms`) + **Task 8** synth eval.
- `project_id`-scoped query returns only that project's rows + global KB → **Task 2** (`TestHybrid_ProjectScope`).
- Decay blend re-enabled; eval holds or improves → **Task 3** + **Task 9 Step 2** (1.000 preserved).
- Alias expansion retrieves a previously-missed synonym, with a recorded delta → **Task 4** + **Task 9 Step 3**.
- Synthesis NOT merged until the synthesis eval passes → **Task 8** + **Task 9 Step 5**.

## Scope boundaries (explicitly deferred / out of scope)

- Resolving "current project" from session metadata (no live caller; honor explicit `Query.ProjectID` only).
- Multi-topic synthesis per (subject,project) — v1 keeps **one** rolling summary per (subject,project); growable later.
- Durable capture outbox, inline extract retry, richer importance scoring (6a §6b-optional; only if measured as needed).
- Full 20–40 synthesis scenarios — seed at ~12, growable (confirmed decision #1).
- Wiring capture/consolidation into the agent request loop (constructors exist: `NewCaptureHookFromConfig`, `MaybeStartConsolidator`; no agent caller yet — same deferral as 6a).
