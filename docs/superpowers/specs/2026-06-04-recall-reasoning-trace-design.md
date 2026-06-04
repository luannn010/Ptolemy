# Recall Reasoning Trace — Design

**Date:** 2026-06-04
**Branch:** `ptolemy/recall-trace`
**Status:** Approved (design); pending implementation plan.

## Problem

When `ptolemy_memory_recall` is called with `generate=true` (and especially with
`AGENT_LOOP_ENABLED=true`, routing through the agentic RAG loop), the retrieval
and reasoning are entirely opaque. The caller sees only the final answer text and
citations. There is no way to observe *how* the LLM retrieved data (which queries
it issued, what chunks came back, with what scores) or *how it reasoned* (the
planner's per-step decisions, why it gave up or was judged ungrounded).

We want the MCP caller (e.g. Codex) to be able to **see the retrieval + reasoning
trace** for a recall, on demand.

## Constraint

MCP stdio tool calls are **request → response**: a tool returns exactly one
result; it does not natively stream tokens or progress into the client mid-call.
Therefore "streaming" here means **a structured reasoning trace returned inside
the recall result**, not live token streaming. (Live streaming via MCP
notifications or a CLI were considered and explicitly deferred — see Non-Goals.)

## Goals

- Opt-in `trace` flag on `ptolemy_memory_recall`. Default behavior is byte-for-byte unchanged.
- When `trace=true`, the result carries a `steps` array describing each step of the recall:
  planner action, its reason, the query it issued, the chunks it retrieved (ID, score,
  short snippet), and the terminal outcome (answered + grounding result, or give-up + reason).
- Works for both the **agentic** path (multi-step loop) and the **legacy** single-shot
  generate path (a simple retrieve → answer trace).
- Trace is strictly **additive** — it can never alter or break the answer.

## Non-Goals (YAGNI)

- No live token streaming into Codex.
- No MCP progress/logging notifications.
- No streaming CLI command.
- No trace for the retrieval-only path (`generate=false`) — there is no LLM reasoning to show.

## Chosen Approach — A: flag-on-`Query`, trace-on-`Answer`

Add a `Trace bool` to `Query` and a `Trace *RecallTrace` to `Answer`. The agent
loop and the legacy generate path populate it as they run; the MCP handler
surfaces it as `steps`. The trace is `nil` by default, so existing callers and
the existing result shape are unaffected.

Rejected alternatives:
- **B — injected tracer callback** (reuse the loop's `onState` hook): more
  decoupled, but needs a tracer plumbed through orchestrator → loop and does not
  naturally cover the legacy path.
- **C — new `AnswerTraced` method**: leaves `Answer` untouched but adds an
  interface method and largely duplicates the pipeline. More surface, no gain.

## Data Model

New file `internal/memory/trace.go`:

```go
// RecallTrace is the optional reasoning trace returned when Query.Trace is set.
type RecallTrace struct {
    Mode  string      // "agentic" | "legacy"
    Steps []TraceStep
}

// TraceStep is one step of the recall: a planner action plus its effects.
type TraceStep struct {
    Index     int          // 0-based step ordinal
    Action    string       // "retrieve" | "answer" | "give_up"
    Reason    string       // planner's reason (give_up) or empty
    Query     string       // query the step issued (retrieve) or empty
    Retrieved []TraceChunk // populated on retrieve steps (the per-step delta)
    GaveUp    bool         // terminal give_up
    GroundingOK bool       // terminal answer: did the grounding check pass
}

// TraceChunk is a compact view of one retrieved chunk.
type TraceChunk struct {
    ID      string
    Score   float64
    Snippet string // first ~120 runes of content, single-lined
}
```

A small helper `snippet(s string, n int) string` (single-line, rune-bounded)
lives in the same file.

## Data Flow

```
Codex
  └─ ptolemy_memory_recall { query, generate:true, trace:true }
       └─ memorytools.handleRecall
            sets q.Trace = true; routes to Recaller.Answer (generate path)
              └─ Orchestrator.Answer
                   ├─ AGENT_LOOP_ENABLED → AgentLoop.Run     (Mode="agentic")
                   │     records a TraceStep per planner action,
                   │     capturing the per-step retrieved delta (ID/score/snippet)
                   │     and the terminal outcome (answer+grounding / give_up+reason)
                   └─ legacy path → retrieve → build → generate (Mode="legacy")
                         records a 2-step trace (retrieve, answer)
              └─ returns Answer{ Text, Citations, GaveUp, Trace }
       └─ handler serializes { text, citations, steps }   (steps omitted when trace not set)
```

Key detail: `AgentLoop.doRetrieve` currently appends to
`AccumulatedChunks`. The trace must record the **per-step delta** (the chunks
that step retrieved), so the step capture uses the slice returned by
`Retriever.Retrieve` for that step, not the cumulative total.

## Components & Responsibilities

- **`trace.go`** — trace types + snippet helper. Pure data; no dependencies.
- **`agent_loop.go`** — when `q.Trace`, build `RecallTrace{Mode:"agentic"}`:
  one `TraceStep` per appended action; retrieve steps carry the delta chunks;
  `doAnswer` sets `GroundingOK`; `doGiveUp`/budget-exhaustion set `GaveUp` + reason.
  Attach to the returned `Answer`.
- **`orchestrator.go`** — legacy `Answer` path: when `q.Trace`, emit a 2-step
  `RecallTrace{Mode:"legacy"}` (retrieve with delta, answer with grounding).
  Pass `Query` through to the loop unchanged (it already carries the flag).
- **`types.go`** — add `Query.Trace bool` and `Answer.Trace *RecallTrace`.
- **`memorytools/tools.go`** — add `trace` boolean to the recall tool schema;
  in `handleRecall`, read it, set `q.Trace`, force the generate path when set,
  and include `steps` in the JSON result (a serialized form of the trace).

## Error Handling

The trace is assembled entirely from data the loop/orchestrator already hold in
memory. It performs no I/O and no parsing that can fail. If, defensively, trace
assembly encounters anything unexpected, the recall answer is returned
regardless — tracing never returns an error and never changes `Text`/`Citations`.

## Testing (TDD — tests precede implementation)

`internal/memory`:
- Agent loop, retrieve → answer: `q.Trace=true` yields a 2-step trace; retrieve
  step carries chunks with IDs/scores/snippets; answer step has `GroundingOK=true`.
- Agent loop, ungrounded answer → trace shows the answer step with
  `GroundingOK=false` and a terminal give-up.
- Agent loop, explicit `give_up` → trace shows the give_up step + reason.
- Agent loop, budget exhausted → trace shows N retrieve steps + terminal
  give_up("step budget exhausted").
- Legacy `Answer` path: `q.Trace=true` yields `Mode:"legacy"` 2-step trace.
- Trace off (`q.Trace=false`): `Answer.Trace == nil` (regression guard).

`internal/mcp/memorytools`:
- `handleRecall` with `trace=true` includes a `steps` key with the expected shape.
- `handleRecall` with `trace=false` (or absent) returns a result **identical** to
  today (no `steps` key) — byte-for-byte regression guard.
- `trace=true, generate=false` → handler routes through the generate/answer path.

## Definition of Done

- New tests above pass (`go test ./internal/memory/... ./internal/mcp/...`).
- `make test` green (or `go test -p 1 ./...`).
- Default recall output unchanged when `trace` is not set.
- One-paragraph note added to `docs/Architecture.md` (memory/recall section).
- Memory remains in-process under its CLAUDE.md carve-out; no guard changes.
