# Extractor Tuning (prompt + grammar) — Design

**Date:** 2026-06-04
**Branch:** `ptolemy/extractor-tuning`
**Status:** Approved (design); pending implementation plan.

## Problem

Live smoke testing showed `ptolemy_memory_capture` rarely persists atoms:

1. **Predicate taxonomy rejection (observed).** `atom.gbnf` lets `fact_predicate`
   be any string and the prompt never states the allowed vocabulary, so the model
   emits free-form predicates (e.g. `"runs on port 9000"`). `PredicateTaxonomyValidator`
   then drops the whole atom (`internal/memory/validators.go:74-97`).
2. **Under-extraction (observed: 0 atoms).** The small brain model returns
   `{"atoms":[]}` for real facts. The terse prompt + "Capture nothing for trivial
   turns" make it too conservative.

A third issue exists — `EvidenceInSourceValidator` requires `content` to be a
verbatim substring of the source, which conflicts with the prompt's "resolve all
pronouns / rewrite" instruction — but per the scope decision below we do **not**
change that validator; we adapt the prompt to it instead.

## Scope decision

- **Prompt + grammar only.** No validator changes.
- **Content style: substring-safe (yield-first).** When the source uses pronouns,
  `content` stays a faithful source clause (so it survives `evidence_in_source`);
  the resolved entity goes in `fact_subject`. We accept that `content` may retain a
  pronoun in exchange for higher capture yield under the unchanged validator.

## Goals

- The model can no longer emit an out-of-taxonomy `fact_predicate` (grammar-enforced).
- The prompt extracts durable facts/decisions/config/preferences instead of
  defaulting to `{"atoms":[]}`.
- `content` is copied faithfully from the source so atoms pass `evidence_in_source`.
- Row provenance is preserved: the prompt version stamped into each row is bumped
  so old and new atoms remain auditable / selectively re-extractable.

## Non-Goals (YAGNI)

- No change to `EvidenceInSourceValidator` or any other validator.
- No new predicates unless a smoke turn genuinely needs one (the taxonomy stays
  as-is: `uses, requires, runs_on, stores_in, listens_on, decides, prefers,
  archives, implements, configured_as`, plus `""`).
- No runtime/generated grammar (rejected approach B); no Go-side predicate
  normalization (rejected approach C).

## Chosen Approach — A: static GBNF enum + prompt v2 + sync test

Mirrors the existing `action.gbnf` precedent, where `actiontype ::= "retrieve" |
"answer" | "give_up"` is a GBNF enum verified by a test. We do the same for
`fact_predicate`.

## Components

### 1. Grammar — `internal/memory/grammar/atom.gbnf`

Replace the `fact_predicate` field's `string` rule with an enum rule:

```gbnf
atom ::= "{" ws
         "\"content\"" ws ":" ws string ws "," ws
         "\"perspective\"" ws ":" ws perspective ws "," ws
         "\"fact_subject\"" ws ":" ws string ws "," ws
         "\"fact_predicate\"" ws ":" ws factpredicate
         ws "}"
perspective ::= "\"factual\"" | "\"relational\""
factpredicate ::= "\"\"" | "\"uses\"" | "\"requires\"" | "\"runs_on\"" | "\"stores_in\"" | "\"listens_on\"" | "\"decides\"" | "\"prefers\"" | "\"archives\"" | "\"implements\"" | "\"configured_as\""
```

The model is now structurally unable to emit an invalid predicate. `fact_subject`
and `content` remain free `string`.

### 2. Prompt — `internal/memory/prompts/extract_v2.txt` (new)

Rewrite of `extract_v1.txt` with these changes:
- **Vocabulary block:** list the allowed `fact_predicate` values and one-line
  meanings; instruct the model to choose the closest one, or `""` for a durable
  fact that fits none.
- **Less conservative:** "trivial" is narrowed to greetings, acknowledgments,
  meta-chatter. Any concrete fact, decision, config value, dependency, or stated
  preference is durable and should be captured.
- **Substring-safe content:** "`content` MUST be copied from the source text —
  reuse the source's wording for the fact, trimming only conversational filler.
  Do NOT paraphrase or invent words not present in the turn. Put the entity the
  fact is about in `fact_subject` (you may resolve a pronoun there)."
- **2–3 few-shot examples**, each consistent with the above: e.g.
  - a config fact → `fact_predicate:"runs_on"`, `content` a verbatim clause;
  - a durable fact with no taxonomy fit → `fact_predicate:""`;
  - a greeting → `{"atoms":[]}`.
- Keep the strict JSON-only output contract and the USER/ASSISTANT turn framing.

### 3. Versioning — `internal/memory/extractor.go`

- `const ExtractorVersion = "extract_v2"` (was `"extract_v1"`).
- `//go:embed prompts/extract_v2.txt` (was `extract_v1.txt`).
- `extract_v1.txt` is kept on disk for history; no longer embedded.

## Data Flow (unchanged)

`capture` → `Extractor.Extract` (prompt v2 + grammar-pinned predicate) →
`ValidatorChain.Filter` (now the predicate always passes taxonomy; content passes
`evidence_in_source` because it is a source substring) → embed → store. The
`ExtractorVersion` stamped into each row's metadata becomes `extract_v2`.

## Error Handling

No new failure modes. The grammar enum is enforced by the model server (same
mechanism as `action.gbnf`); if a model ignored the grammar, the existing
`PredicateTaxonomyValidator` still backstops it. Prompt changes cannot break
compilation; their effect is empirical and verified by smoke.

## Testing

**Deterministic (unit):**
- New `TestPredicateGrammarMatchesTaxonomy` (in `grammar_test.go`): parse the
  `factpredicate ::= ...` alternation from `atom.gbnf`; assert its set of quoted
  values equals the keys of `allowedPredicates` — **both directions** (no grammar
  value missing from the map; no map key missing from the grammar). This is the
  DRY guard that keeps grammar and validator in sync.
- Existing `TestGrammarFile_ValidGBNF`, `TestGrammarMatchesAtomStruct`,
  `TestExtractor_SendsGrammarInRequest` must stay green.

**Empirical (live brain — not part of `go test`):**
- `make smoke-capture` (runs the real `BRAIN_*` extractor on a sample exchange and
  logs extracted atoms) must produce ≥1 accepted atom.
- Live MCP capture→recall smoke (the JSON-RPC pipe used during this session): a
  taxonomy-valid fact is captured, then `recall {trace:true}` returns it with a
  grounded answer and chunks in the trace.

## Definition of Done

- `TestPredicateGrammarMatchesTaxonomy` and all existing memory tests pass
  (`go test ./internal/memory/...`).
- `go build ./...` clean.
- `make smoke-capture` yields ≥1 accepted atom against the live brain.
- A live capture→recall smoke persists and recalls a fact.
- One-paragraph note in `docs/Architecture.md` (extractor/capture section) recording
  the grammar-pinned predicate, the `extract_v2` prompt, and the substring-safe
  content policy.
- No validator changes; memory stays in-process under its CLAUDE.md carve-out.
