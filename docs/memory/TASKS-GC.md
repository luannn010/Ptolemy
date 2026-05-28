# Memory GC — Build Plan (ordered)

Implement in this order. Each phase is independently testable and reversible. You have a working,
safe system after Phase 3. Phases 4–5 add capability; build only as needed.

Mark items `[x]` as you complete them. Keep `internal/memory/score_test.go` passing throughout.

---

## Phase 0 · Schema + audit  — DO NOT DEFER

- [ ] Apply `migrations/0001_init.sql` (enable `pg_trgm`, create `memories` + `memory_audit`).
- [ ] Verify all four indexes exist: `idx_active` (partial), `idx_fts`, `idx_trgm`, `idx_fact`.
- [ ] Wire up `pgxpool` from `DATABASE_URL`. Add a thin `Store` holding the pool.

## Phase 1 · Soft-delete + scope gate  — DO NOT DEFER

- [ ] Implement `Ingest()` in `store.go`: plain INSERT, tags `scope`, sets `status='active'`.
- [ ] Implement `Retrieve()` in `store.go`: the union BM25 query from SPEC §4, filtering
      `status='active'`. Confirm `global` rows are NEVER touched by any decay/archive path.
- [ ] Add the counts-by-(scope,status) dashboard query (SPEC §7) as a `Stats()` method.

## Phase 2 · Lazy decay + reinforcement

- [ ] `decayScore()` in `score.go` is already implemented — confirm `score_test.go` passes.
- [ ] Implement `Reinforce()` in `store.go`: bump `access_count` + `last_accessed_at`.
- [ ] Call `Reinforce()` from EVERY read path that returns a memory. No exceptions — a read path
      that forgets to reinforce lets useful memories silently rot.
- [ ] Blend decay into ranking (already in the `Retrieve` query). **✓ Usable system here.**

## Phase 3 · The sweep

- [ ] Implement `Run()` in `gc.go`: a `time.Ticker` loop (interval from config, default 1h),
      cancellable via `context.Context`.
- [ ] Implement `archiveDecayed()`: the archive pass from SPEC §6, writing to `memory_audit`.
      MUST include `WHERE scope = 'project' AND NOT pinned`.
- [ ] Add a `purgeDead()` pass: deletes rows `dead` for 30d+. Gate behind `GC_PURGE_ENABLED`
      (default false). This is the ONLY destructive operation — be conservative.
- [ ] Add tests: insert old/unaccessed project rows → run archive → assert archived; insert a
      `global` row with the same age → assert it is UNTOUCHED.

## Phase 4 · Supersession

- [ ] Implement `Supersede()` in `store.go`: transactional — insert new row with `supersedes` +
      `version+1`, mark old `superseded` with `superseded_by`, log to audit. (See SPEC §5.)
- [ ] Implement `History()`: the recursive version-chain query.
- [ ] At ingest, if `fact_subject`+`fact_predicate` are set, check for an existing fact with the
      same subject+predicate: same value → treat as duplicate (reinforce); different value →
      `Supersede()`. This is the zero-cost structured path.
- [ ] Wire `confidence` for the news/verification flow (low on first report, high on verify).
- [ ] Tests: supersede a row → assert old hidden from `Retrieve`, new visible, chain walkable,
      and the whole thing reversible.

## Phase 5 · Dedup in the sweep  — WHEN NEEDED

- [ ] Implement `dedupRecent()` in `gc.go`: trigram (`%` / `similarity()`) over rows inserted
      since last sweep, WITHIN scope. Above threshold (config, default 0.7):
      - structured/clear duplicate → reinforce survivor, mark loser `dead` reason `'duplicate'`.
      - ambiguous (possible contradiction) → DO NOT collapse; keep both (safe fallback).
- [ ] Add to the `Run()` pass order: dedup → supersession → archive → purge.
- [ ] Tests: insert near-identical project rows → run dedup → assert one survivor reinforced,
      loser dead; insert a contradiction pair → assert BOTH kept.

---

## Acceptance checklist (the whole thing is done when…)

- [ ] Ingest and read paths issue no trigram/decay comparison and no external calls.
- [ ] `global` rows are provably never archived (test asserts it).
- [ ] Every status change appears in `memory_audit`.
- [ ] Archiving is reversible by a single UPDATE; only `purgeDead` deletes, and only on 30d+ dead.
- [ ] `lambda`, archive threshold, similarity threshold, sweep interval, purge grace, and
      `GC_PURGE_ENABLED` are all CONFIG, not magic numbers in queries.
- [ ] Contradictions are never collapsed as duplicates (test asserts it).

## Reminder to flag to the user

The default tuning values (`lambda=0.05`, archive `<0.1`, trigram `0.7`, sweep `1h`) are
PLACEHOLDERS. Tell the user explicitly they must be tuned against real chat volume using the
`Stats()` counts before being trusted.
