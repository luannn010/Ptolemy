# CLAUDE.md — Ptolemy v2 Clean-Room Rebuild

This file is the Claude Code overlay on top of [AGENTS.md](AGENTS.md). AGENTS.md is authoritative for branching, commit hygiene, PR rules, and the Ptolemy-First execution model — read it first. CLAUDE.md adds the Claude-specific harness wiring and the project context that should shape every response.

## Project context

Ptolemy v2 is a **clean-room rebuild** following the bootstrap plan in conversation history (`docs/Architecture.md` once restored). Three locked choices:

- **Strategy**: clean-room rebuild — port only the keep-list packages from `ptolemy-legacy/`, never import it.
- **Approval model**: hybrid — in-band token for low-risk Ask, **out-of-band** (loopback / worker console) for high-risk Ask. The agent never self-approves.
- **Default stance**: fail-safe **ask/oob**. Anything not explicitly listed in `.ptolemy/policy.json` pauses for a human.

The `internal/policy/` harness is the trust root. Every side-effecting adapter (`shellcmd`, `terminal`, `fileops`, `gitops`, `worktree`, `workspace`, `inspect`) must be reachable from services **only** via a Guarded* wrapper. `GuardedRunner` exists today; `GuardedFileOps` / `GuardedGit` / `GuardedWorktree` are the next scope per plan §4 step 4.

`navigator` is the only side-effecting package that intentionally bypasses the harness — it is read-only knowledge-base access. The `internal/memory` package shares this carve-out: it is reached in-process by the memory MCP tools and the `ptolemy-memory` CLI, and its only writes land in the **memory Postgres DB** (never the guarded workspace, shell, or git), so it is exempt from the `Guarded*` rule. No other package may claim this exemption.

## Superpowers skill harness

Skills are loaded automatically by the harness; you must invoke them via the `Skill` tool before responding. For this project the priority order is:

1. **`superpowers:brainstorming`** — before any new feature, refactor, or behavior change. Confirm intent and design before code.
2. **`superpowers:writing-plans`** — multi-step work gets a written plan first.
3. **`superpowers:test-driven-development`** — tests precede implementation. Non-negotiable for `internal/policy` and any guard.
4. **`superpowers:systematic-debugging`** — before proposing any fix for a bug, test failure, or unexpected verdict.
5. **`superpowers:verification-before-completion`** — run the relevant tests and report output before claiming done.
6. **`superpowers:requesting-code-review`** — before opening any PR.
7. **`superpowers:receiving-code-review`** — process review feedback through this skill, not ad-hoc.

Use `superpowers:dispatching-parallel-agents` or `superpowers:subagent-driven-development` only for genuinely independent tasks with non-overlapping file ownership.

Use `superpowers:using-git-worktrees` when isolation is needed (e.g., porting a legacy package without touching the working tree).

## Mandatory: ask before starting new implementation

Before writing implementation code for **any new feature, package port, or guard**, you MUST:

1. Ask the user: **"Do you want me to create a new branch for this work?"**
   - If yes: create `ptolemy/<task-slug>` from the current branch and commit per phase.
   - If no: stay on current branch and still commit per phase.
2. Run `superpowers:brainstorming` to confirm scope (unless the user explicitly says "skip brainstorming").
3. Run `superpowers:writing-plans` if the work is multi-step.
4. Only then begin TDD per `superpowers:test-driven-development`.

This rule applies to:
- porting any `internal/*` package from `ptolemy-legacy/`
- adding any `Guarded*` wrapper
- adding routes to `internal/httpapi` or tools to `internal/mcp`
- editing `.ptolemy/policy.json` rules (which are themselves protected by `deny-policy-write`)

It does **not** apply to:
- trivial fixes (typos, formatting, single-file bugfix < 20 LOC)
- restoring accidentally-deleted files from git history
- answering questions or auditing scope without code changes

When unsure whether something counts as "new implementation", ask.

## Memory & context recall (via MCP)

Ptolemy has a working conversational memory/RAG system (`internal/memory`) exposed through three MCP tools and a CLI. Use it instead of asking the user to paste context.

- **Recall before you ask.** At the start of a task — or when you need project background you don't have — call `ptolemy_memory_recall` with a focused query. It returns a compact synthesis summary plus a few supporting atoms and citations, not a context dump. Prefer this over re-reading large swaths of the repo or asking the user to re-explain.
- **Capture what you learn.** When a durable fact, decision, or constraint emerges (a port number, an architectural choice, a gotcha), call `ptolemy_memory_capture` with the relevant user/assistant exchange so it survives into future sessions.
- **Consolidate periodically.** At task end (or after several captures) call `ptolemy_memory_consolidate` to fold loose atoms into a single reconciled summary for the subject/project.

Scope (`subject_id` / `project_id`) defaults from `PTOLEMY_MEMORY_SUBJECT` / `PTOLEMY_MEMORY_PROJECT`; pass args only to override. The tools call `internal/memory` **in-process** (no workerd hop) under the carve-out noted above. If `DATABASE_URL` is unset the memory tools are simply absent — the rest of the MCP surface still works.

Hooks in `.claude/settings.local.json` (host-specific, gitignored) automate the common path: `SessionStart` runs `ptolemy-memory recall` to seed project context, and `Stop` runs `ptolemy-memory capture` on the last exchange.

## Policy & safety guardrails

- **Never** loosen a `deny` rule in `.ptolemy/policy.json`. Add `allow` or `ask` rules; leave denies and the two self-protection rules (`deny-policy-write`, `deny-secret-*`) untouched.
- **Never** import from `ptolemy-legacy/`. Copy + adapt + test.
- **Never** wire a raw adapter into a service. Services hold `Guarded*` types only. The only place raw adapters exist is `cmd/workerd/main.go`.
- **Never** run destructive shell commands (`rm -rf`, `git push --force`, `git reset --hard`) without explicit user approval, even if the policy engine would allow them via your local rules.
- **Never** use `git add .` or `git add -A`. Stage explicit files only (per AGENTS.md §Commit hygiene).
- **Never** use the `gh` CLI; use the GitHub plugin tooling (per AGENTS.md §GitHub Tooling Policy).

## Repository conventions

- Module path: `github.com/luannn010/ptolemy`. Go 1.25.
- Four-table schema only: `sessions`, `command_logs`, `policy_decisions`, `schema_migrations`. Do not add tables without a written plan.
- Build: `make build` produces `workerd`, `ptolemy-mcp`, `ptolemy`, and `ptolemy-memory` (a thin alias for `ptolemy memory recall|capture`). The former `policy-demo` and `memory-*` demo/eval binaries are now subcommands: `ptolemy policy check` and `ptolemy memory demo|eval|synth-eval`.
- Tests: `make test` (uses `-p 1`). Policy bypass suite lives at [internal/policy/engine_test.go](internal/policy/engine_test.go).
- Audit: every `Authorize` result is logged via zerolog and persisted to `policy_decisions` from inside the guard, before any side effect.

## Definition of done (per package port)

A ported package is done only when:
- it compiles in the new module,
- its own tests pass,
- if it touches files/shell/git/network, it is reachable **only** through a guard,
- it has a one-paragraph note in `docs/Architecture.md`.
