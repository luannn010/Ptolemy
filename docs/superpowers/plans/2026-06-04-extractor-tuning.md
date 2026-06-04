# Extractor Tuning Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `ptolemy_memory_capture` reliably persist atoms by grammar-pinning `fact_predicate` to the allowed taxonomy and rewriting the extraction prompt (v2) to extract more and produce substring-safe content.

**Architecture:** Approach A — add a GBNF enum for `fact_predicate` in `atom.gbnf` (mirroring the existing `action.gbnf` `actiontype` enum), guarded by a DRY sync test against `allowedPredicates`; replace the embedded prompt with `extract_v2.txt` and bump `ExtractorVersion`. No validator changes.

**Tech Stack:** Go 1.26, `internal/memory` (embedded GBNF grammar + prompt, OpenAI-compatible brain LLM). Deterministic tests via `go test`; prompt quality verified empirically via `make smoke-capture` + a live MCP capture→recall smoke.

**Spec:** `docs/superpowers/specs/2026-06-04-extractor-tuning-design.md`

---

## File Structure

- **Modify** `internal/memory/grammar/atom.gbnf` — add `factpredicate` enum; use it for the `fact_predicate` field.
- **Modify** `internal/memory/grammar_test.go` — add `TestPredicateGrammarMatchesTaxonomy` (DRY guard).
- **Create** `internal/memory/prompts/extract_v2.txt` — rewritten extraction prompt.
- **Modify** `internal/memory/extractor.go` — embed `extract_v2.txt`; bump `ExtractorVersion`.
- **Modify** `internal/memory/extractor_test.go` — add `TestExtractorPromptV2_Wiring`.
- **Modify** `docs/Architecture.md` — one-paragraph note.

Reference (do not edit): `internal/memory/validators.go:74-86` holds `allowedPredicates`; `internal/memory/grammar/action.gbnf` is the enum precedent.

---

## Task 1: Grammar-pin `fact_predicate` + DRY sync test

**Files:**
- Modify: `internal/memory/grammar/atom.gbnf`
- Test: `internal/memory/grammar_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/memory/grammar_test.go`:

```go
func TestPredicateGrammarMatchesTaxonomy(t *testing.T) {
	b, err := os.ReadFile("grammar/atom.gbnf")
	if err != nil {
		t.Fatalf("read grammar: %v", err)
	}
	// Find the single-line `factpredicate ::= ...` alternation rule.
	var rule string
	for _, ln := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(strings.TrimSpace(ln), "factpredicate ::=") {
			rule = ln
			break
		}
	}
	if rule == "" {
		t.Fatal("grammar missing `factpredicate ::=` rule")
	}
	// Forward: every allowed predicate must appear as a quoted alternative.
	// The .gbnf escapes quotes as \" ; unescape to compare literal tokens.
	unesc := strings.ReplaceAll(rule, `\"`, `"`)
	for p := range allowedPredicates {
		lit := `"` + p + `"` // p=="" -> `""`
		if !strings.Contains(unesc, lit) {
			t.Fatalf("factpredicate rule missing taxonomy value %q (looked for %s)", p, lit)
		}
	}
	// Reverse: alternative count must equal taxonomy size (guards against extras).
	alts := strings.Count(rule, "|") + 1
	if alts != len(allowedPredicates) {
		t.Fatalf("factpredicate has %d alternatives, taxonomy has %d", alts, len(allowedPredicates))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/memory/ -run TestPredicateGrammarMatchesTaxonomy -v`
Expected: FAIL — `grammar missing 'factpredicate ::=' rule` (the rule does not exist yet).

- [ ] **Step 3: Edit the grammar**

In `internal/memory/grammar/atom.gbnf`, change the `fact_predicate` field in the `atom` rule from `string` to `factpredicate`, and add the `factpredicate` rule after `perspective`. The full file becomes:

```gbnf
root ::= ws object ws
object ::= "{" ws "\"atoms\"" ws ":" ws array ws "}"
array ::= "[" ws (atom (ws "," ws atom)*)? ws "]"
atom ::= "{" ws
         "\"content\"" ws ":" ws string ws "," ws
         "\"perspective\"" ws ":" ws perspective ws "," ws
         "\"fact_subject\"" ws ":" ws string ws "," ws
         "\"fact_predicate\"" ws ":" ws factpredicate
         ws "}"
perspective ::= "\"factual\"" | "\"relational\""
factpredicate ::= "\"\"" | "\"uses\"" | "\"requires\"" | "\"runs_on\"" | "\"stores_in\"" | "\"listens_on\"" | "\"decides\"" | "\"prefers\"" | "\"archives\"" | "\"implements\"" | "\"configured_as\""

string ::= "\"" chars "\""
chars ::= char*
char ::= [^"\\\x00-\x1F] | escape
escape ::= "\\" (["\\/bfnrt] | unicode)
unicode ::= "u" hex hex hex hex
hex ::= [0-9a-fA-F]
ws ::= [ \t\n\r]*
```

Keep `factpredicate` on a single line — `TestPredicateGrammarMatchesTaxonomy` counts its `|` separators on one line.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/memory/ -run 'Grammar|PredicateGrammar' -v`
Expected: PASS — `TestPredicateGrammarMatchesTaxonomy`, plus existing `TestGrammarFile_ValidGBNF` and `TestGrammarMatchesAtomStruct` still pass (the `fact_predicate` json tag is still present in the atom rule).

- [ ] **Step 5: Commit**

```bash
git add internal/memory/grammar/atom.gbnf internal/memory/grammar_test.go
git commit -m "feat(memory): grammar-pin fact_predicate to the allowed taxonomy

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 2: Prompt v2 + version bump + wiring test

**Files:**
- Create: `internal/memory/prompts/extract_v2.txt`
- Modify: `internal/memory/extractor.go`
- Test: `internal/memory/extractor_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/memory/extractor_test.go` (add `"strings"` to its imports if not present):

```go
func TestExtractorPromptV2_Wiring(t *testing.T) {
	if ExtractorVersion != "extract_v2" {
		t.Fatalf("ExtractorVersion = %q, want extract_v2", ExtractorVersion)
	}
	if strings.TrimSpace(extractPromptTemplate) == "" {
		t.Fatal("embedded extract prompt is empty")
	}
	// v2 must advertise the predicate vocabulary so the model emits taxonomy values.
	for _, tok := range []string{"uses", "runs_on", "listens_on", "stores_in", "configured_as"} {
		if !strings.Contains(extractPromptTemplate, tok) {
			t.Fatalf("extract prompt missing predicate vocabulary token %q", tok)
		}
	}
	// v2 must instruct substring-safe content (copy from source, do not paraphrase).
	if !strings.Contains(strings.ToLower(extractPromptTemplate), "copy") {
		t.Fatal("extract prompt should instruct copying content from the source turn")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/memory/ -run TestExtractorPromptV2_Wiring -v`
Expected: FAIL — `ExtractorVersion = "extract_v1", want extract_v2`.

- [ ] **Step 3a: Create the v2 prompt**

Create `internal/memory/prompts/extract_v2.txt` with exactly this content:

```
You extract durable memory entries from one conversation turn (a user message and the assistant's reply).

Return ONLY a JSON object, no prose:
{"atoms":[{"content": string, "perspective": "factual"|"relational", "fact_subject": string, "fact_predicate": string}]}

WHAT TO CAPTURE
- Capture every concrete, durable fact, decision, configuration value, dependency, or stated preference. Examples: which tool/port/model/library is used, an architectural choice, a setting value, a person's preference or working style.
- Capture nothing ONLY for truly trivial turns: greetings, acknowledgments ("ok", "thanks"), or pure meta-chatter.
- When in doubt, capture it. Return {"atoms":[]} only if there is genuinely nothing durable.

HOW TO WRITE EACH ATOM
- ATOMIC: exactly one fact per entry.
- CONTENT MUST COME FROM THE SOURCE: copy the clause that states the fact directly from the turn, trimming only conversational filler ("I think", "maybe", "let's"). Do NOT paraphrase and do NOT introduce words that are not in the turn. The content must be findable as text in the turn.
- ATTRIBUTE correctly: the user decided/asked; the assistant supplied the detail.
- PERSPECTIVE: "factual" for durable facts/decisions/config; "relational" for preferences, working style, softer context.
- STRUCTURED: set fact_subject to the entity the fact is about (you MAY resolve a pronoun here, e.g. "it" -> "the brain server"). Set fact_predicate to the attribute, choosing the closest value from this FIXED vocabulary:
    uses, requires, runs_on, stores_in, listens_on, decides, prefers, archives, implements, configured_as
  If none fits, use "" (empty). Never use a predicate outside this list.

EXAMPLES

Turn:
USER:
Let's run the brain server on port 9000.
ASSISTANT:
Done - the brain server now runs on port 9000.
Output:
{"atoms":[{"content":"the brain server now runs on port 9000","perspective":"factual","fact_subject":"brain server","fact_predicate":"runs_on"}]}

Turn:
USER:
We use PostgreSQL with pgvector for the memory store.
ASSISTANT:
Got it.
Output:
{"atoms":[{"content":"We use PostgreSQL with pgvector for the memory store","perspective":"factual","fact_subject":"memory store","fact_predicate":"uses"}]}

Turn:
USER:
thanks!
ASSISTANT:
You're welcome.
Output:
{"atoms":[]}

The next message contains the turn, formatted as:
USER:
<the user's message>

ASSISTANT:
<the assistant's reply>
```

- [ ] **Step 3b: Re-point the embed and bump the version**

In `internal/memory/extractor.go`, change the version constant and the embed directive:

```go
// ExtractorVersion is stamped into every captured row's metadata so entries are
// auditable and selectively re-extractable when the prompt changes.
const ExtractorVersion = "extract_v2"

//go:embed prompts/extract_v2.txt
var extractPromptTemplate string
```

(`prompts/extract_v1.txt` stays on disk for history; it is simply no longer embedded.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/memory/ -run 'TestExtractor' -v`
Expected: PASS — `TestExtractorPromptV2_Wiring` and the existing extractor tests (`TestExtractor_SendsGrammarInRequest`, etc.).

- [ ] **Step 5: Commit**

```bash
git add internal/memory/prompts/extract_v2.txt internal/memory/extractor.go internal/memory/extractor_test.go
git commit -m "feat(memory): extract_v2 prompt — vocabulary, less-conservative, substring-safe content

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 3: Architecture note + verification (deterministic + live)

**Files:**
- Modify: `docs/Architecture.md`

- [ ] **Step 1: Add the doc note**

Append to `docs/Architecture.md` (match the existing `## Package (path)` heading style):

```markdown
## Capture extractor tuning (`internal/memory`)

The capture extractor is grammar-constrained and prompt-versioned. `fact_predicate`
is pinned by `grammar/atom.gbnf` to the fixed taxonomy in `validators.go`
(`allowedPredicates`) — the model cannot emit an out-of-taxonomy predicate
(`TestPredicateGrammarMatchesTaxonomy` guards the two lists against drift, mirroring
the `action.gbnf` precedent). The `extract_v2` prompt (`prompts/extract_v2.txt`,
stamped as `ExtractorVersion`) lists that vocabulary, extracts less conservatively,
and requires `content` to be copied from the source turn (trim filler, no paraphrase)
so atoms survive `EvidenceInSourceValidator`; the resolved entity goes in
`fact_subject`. No validator was changed.
```

- [ ] **Step 2: Run the full memory suite + build (deterministic gate)**

Run: `go test ./internal/memory/... && go build ./...`
Expected: PASS, build clean.

- [ ] **Step 3: Commit the deterministic work**

```bash
git add docs/Architecture.md
git commit -m "docs(memory): note grammar-pinned predicate + extract_v2 prompt

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

- [ ] **Step 4: Live empirical verification (requires brain + embeddings + Postgres up)**

Rebuild the MCP binary from this branch and run the live capture→recall smoke under an isolated `smoke-test` scope. This is the real proof the tuning works end-to-end; it is NOT a `go test`.

```bash
# build the mcp binary from the current branch
go build -o bin/ptolemy-mcp.exe ./cmd/ptolemy-mcp   # drop .exe on Linux

# capture a taxonomy-valid fact, then recall it with the trace
printf '%s\n' \
 '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' \
 '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"ptolemy_memory_capture","arguments":{"user_text":"We use PostgreSQL with pgvector for the memory store.","assistant_text":"Got it.","subject_id":"smoke-test","project_id":"smoke-test"}}}' \
 '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"ptolemy_memory_recall","arguments":{"query":"what does the memory store use","subject_id":"smoke-test","project_id":"smoke-test","trace":true}}}' \
 | AGENT_LOOP_ENABLED=true ./bin/ptolemy-mcp.exe
```

Expected: id 2 returns `{"captured":true}` AND the server's stderr shows an
*accepted* atom (no "all extracted atoms rejected"); id 3 returns a grounded
answer with at least one retrieved chunk in `steps` (not `give_up: no chunks`).

If the extractor still returns 0 atoms or all-rejected for a clearly-durable fact,
that is a prompt-quality finding to iterate on (adjust `extract_v2.txt` wording /
examples and re-run this step) — not a code defect. `make smoke-capture` is an
alternate live check that logs the extractor's atoms directly.

---

## Self-Review

**Spec coverage:**
- Grammar-pin `fact_predicate` → Task 1 (grammar edit + sync test).
- Prompt v2: vocabulary, less-conservative, substring-safe content, examples → Task 2 (`extract_v2.txt`).
- Version bump / provenance → Task 2 (`ExtractorVersion = "extract_v2"`, embed re-point).
- No validator changes → respected (only grammar/prompt/extractor touched).
- DRY guard between grammar and `allowedPredicates` → Task 1 (`TestPredicateGrammarMatchesTaxonomy`, both directions).
- Architecture note → Task 3.
- Empirical verification (`make smoke-capture` + live capture→recall) → Task 3 Step 4.

**Placeholder scan:** none — every code/step shows complete content and an exact command with expected result.

**Type consistency:** `allowedPredicates` (existing map, `validators.go`), `ExtractorVersion` and `extractPromptTemplate` (existing identifiers, `extractor.go`), grammar rule name `factpredicate`, and test names `TestPredicateGrammarMatchesTaxonomy` / `TestExtractorPromptV2_Wiring` are used identically across tasks. The grammar enum's 11 values (`""` + the 10 named) match `allowedPredicates` exactly.
