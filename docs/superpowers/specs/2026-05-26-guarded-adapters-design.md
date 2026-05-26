# Guarded Adapters — Design Spec

**Date:** 2026-05-26
**Branch:** `ptolemy/guarded-adapters`
**Plan reference:** Bootstrap plan §4 step 4 — extend `guard.go` with `GuardedFileOps`, `GuardedGit`, `GuardedWorktree`, mirroring `GuardedRunner`.

## 1. Problem

The Ptolemy v2 policy harness today gates only shell command execution via `GuardedRunner`. The other side-effecting adapters in `internal/` — `fileops`, `gitops`, `worktree` — have no guard, so any service that holds them can bypass the policy engine entirely. Plan §4 step 4 calls this out as the required next scope. Without these guards:

- `fileops.WriteFile(".ptolemy/policy.json", …)` evades the `deny-policy-write` rule.
- `fileops.ReadFile(".env")` evades `deny-secret-cmd`.
- `gitops.Push(…)` evades `ask-push-cmd` and `deny-destructive` (force push).
- `worktree.Create/Remove(…)` has no rule at all and therefore should fall through to the fail-safe `default → ask/oob`, but currently isn't even consulted.

## 2. Goals

1. Every side-effecting adapter is reachable from services **only** through a `Guarded*` wrapper that calls `Engine.Authorize` and records to `policy_decisions` before any raw call.
2. Existing `.ptolemy/policy.json` rules cover both shell-side and adapter-side paths without rule changes — `deny-secret-cmd` (`.env`) catches `cat .env` *and* `fileops.ReadFile(".env")`.
3. The harness API converges on a single shape (`CallOpts` with optional `ConfirmToken`), eliminating the 1:1 `Method` + `MethodConfirmed` doubling that would otherwise produce ~54 methods across three new guards.
4. No new external dependencies; no schema changes; `policy_decisions` continues to be the sole audit table.

## 3. Non-goals

- Adding new policy rules. The existing rules in `.ptolemy/policy.json` are sufficient to demonstrate end-to-end gating. New rules are a future tuning task per plan §7.
- Wiring `navigator` through a guard. Per plan §4 step 6, navigator is read-only KB access and intentionally bypasses the harness.
- New HTTP routes or MCP tools. This work threads the confirm-token round-trip through existing endpoints only.
- Refactoring adapters themselves. `fileops`, `gitops`, `worktree` source files are untouched.

## 4. Locked design decisions (from brainstorming)

| # | Question | Decision |
|---|---|---|
| 1 | Scope per branch | All three guards in one branch → single PR to `main`. |
| 2 | Read-only ops (gitops Status/Diff/Log…, fileops Read/List/Search) | Gate uniformly — every call hits `Authorize` and lands in `policy_decisions`. A hostile rule that breaks reads is a policy-file bug, not an engine bug. |
| 3 | API shape | Unified opts struct (`CallOpts{ConfirmToken}`). One method per adapter op. As part of this work, refactor existing `GuardedRunner.Run/RunConfirmed` to the same shape. |

## 5. Architecture

### 5.1 `CallOpts` and `gate()` helper

A single private helper inside `internal/policy` becomes the only place that runs the Authorize → record → Allow/Ask/Deny pipeline. All four guards delegate to it. Adding a new guarded method becomes ~5 lines: build the `Intent`, call `gate`, on success call the raw adapter.

```go
// internal/policy/guard.go

type CallOpts struct {
    ConfirmToken string // empty on first call; equal to pendingID on the retry after approval
}

type guardCore struct {
    engine    *Engine
    approvals *Approvals
    db        *sql.DB
}

// gate runs the Authorize/audit/decision pipeline. On Allow it returns nil and
// the caller proceeds with the raw adapter. On Ask it parks the approval and
// returns ErrNeedsConfirmation. On Deny it returns ErrDenied. On the
// confirmed-retry path (opts.ConfirmToken != "") it verifies the token equals
// the intent hash and is approved, then records confirmed=1 and returns nil.
func (g *guardCore) gate(ctx context.Context, sessionID string, intent domain.Intent, opts CallOpts) error
```

The key invariant: **pendingID == intentHash**. The confirm-token IS the hash of the intent it approves, so a swap-intent attack (approve `git log`, retry with `git push`) fails the equality check inside `ConsumeApproved`+hash comparison. Single-use is enforced by `Approvals.ConsumeApproved`.

### 5.2 Guard types

Three new structs, each embedding `guardCore` and holding the raw adapter:

```go
type GuardedFileOps struct { core guardCore; raw *fileops.FileOps }
type GuardedGit       struct { core guardCore; raw *gitops.GitOps   }
type GuardedWorktree  struct { core guardCore; raw *worktree.Manager }
```

Each public method:
1. Resolves any user-supplied path (fileops only — via `raw.Resolve`) so `Targets` always carry the absolute resolved path.
2. Builds the `domain.Intent` per §6 taxonomy.
3. Calls `core.gate(ctx, sessionID, intent, opts)`.
4. On nil error, delegates to the raw adapter and returns its result.

### 5.3 `GuardedRunner` refactor

`Run` and `RunConfirmed` collapse into a single method:

```go
func (g *GuardedRunner) Run(
    ctx context.Context,
    sessionID, command, cwd string,
    timeoutSeconds int,
    opts CallOpts,
) (terminal.Result, error)
```

Three callers update: `internal/command/command.go` (interface), `internal/command/service.go` (call sites), `internal/command/service_test.go` (fake).

## 6. Intent taxonomy

`Intent.Kind` follows `<package>.<verb>`. `Program` is the semantic verb (not the OS binary) so existing string-contains rules like `git push` or `.env` match across shell and adapter paths.

### 6.1 fileops — `Targets = [resolvedPath]`

| Method | Kind | Program | Args |
|---|---|---|---|
| `Resolve` | `file.resolve` | `resolve` | — |
| `ReadFile` | `file.read` | `read` | — |
| `WriteFile` | `file.write` | `write` | — |
| `ListDirectory` | `file.list` | `list` | — |
| `Search` | `file.search` | `search` | `[query]` |
| `ApplyPatch` | `file.patch` | `patch` | — |
| `ReplaceBlock` | `file.replace` | `replace` | — |
| `InsertAfter` | `file.insert` | `insert` | `[marker]` |

### 6.2 gitops — `Targets = [repoPath]`

| Method(s) | Kind | Program | Args |
|---|---|---|---|
| `Status`, `Diff`, `Log`, `CurrentBranch`, `CurrentCommitSHA`, `ChangedFiles` | `git.read` | `git` | `[<subcmd>]` |
| `Checkout`, `CreateBranch`, `EnsureBranch`, `CreateOrResetBranchFrom` | `git.branch` | `git` | `[<subcmd>, <branch>]` |
| `StageFiles` | `git.stage` | `git` | `[add, …files]` |
| `CommitConventional`, `CommitStagedConventional` | `git.commit` | `git` | `[commit]` |
| `MergeNoFF` | `git.merge` | `git` | `[merge, <branch>]` |
| `Push` | `git.push` | `git` | `[push, <remote>, <branch>]` |
| `CreatePullRequest` | `git.pr` | `gh` | `[pr, create, …]` |

Read-only ops share one kind `git.read` because the policy file shouldn't enumerate them; mutating ops get distinct kinds so rules can target `git.push` independently of `git.commit`.

### 6.3 worktree — `Targets = [worktreePath]`

| Method | Kind | Program | Args |
|---|---|---|---|
| `Create` | `worktree.create` | `worktree` | `[create, <name>, <branch>]` |
| `AddExisting` | `worktree.add` | `worktree` | `[add, <name>, <branch>]` |
| `Remove` | `worktree.remove` | `worktree` | `[remove, <name>]` |
| `List` | `worktree.list` | `worktree` | `[list]` |

## 7. Rule coverage validation

With existing `.ptolemy/policy.json`, the new guards should produce these verdicts on day one:

| Guard call | Expected rule | Effect |
|---|---|---|
| `GuardedFileOps.WriteFile(".ptolemy/policy.json", …)` | `deny-policy-write` | deny |
| `GuardedFileOps.ReadFile(".env")` | `deny-secret-cmd` | deny |
| `GuardedFileOps.ReadFile("README.md")` | (no match) → default | ask/oob |
| `GuardedGit.Push("origin", "main")` | `ask-push-cmd` | ask/oob |
| `GuardedGit.Push` after `--force`-style arg | `deny-destructive` | deny |
| `GuardedGit.Status()` | (no match) → default | ask/oob |
| `GuardedWorktree.Create("x", "b")` | (no match) → default | ask/oob |

The fail-safe defaults for innocuous reads (`README.md`, `git status`, `worktree list`) are noisy by design until the policy is tuned per plan §7 — that's the agreed-upon trade-off of decision (a) in §4.

## 8. Files changed

```
internal/policy/
  guard.go              # add CallOpts, guardCore, gate(); add Guarded{FileOps,Git,Worktree}; refactor GuardedRunner
  guard_test.go         # extend with per-guard happy/deny/ask-confirmed tests + swap-intent regression

internal/command/
  command.go            # update GuardedRunner interface signature
  service.go            # thread CallOpts{}
  service_test.go       # update fakeGuardedRunner

cmd/workerd/
  main.go               # construct raw fileops/gitops/worktree adapters, build all four guards, hand guards to services

internal/httpapi/      # thread confirm_token through existing command endpoint
internal/mcp/          # thread confirm_token through existing tool surface
```

No changes under `internal/fileops/`, `internal/gitops/`, `internal/worktree/`, `internal/domain/`, or `.ptolemy/policy.json`.

## 9. Test plan

Driven by TDD per CLAUDE.md superpowers harness.

| Test | Asserts |
|---|---|
| `TestGuardedFileOps_DenyPolicyWrite` | `WriteFile(".ptolemy/policy.json")` returns `ErrDenied{RuleID:"deny-policy-write"}`; raw adapter NOT called |
| `TestGuardedFileOps_DenySecretRead` | `ReadFile(".env")` returns `ErrDenied{RuleID:"deny-secret-cmd"}` |
| `TestGuardedFileOps_AllowReadAfterConfirm` | Default-ask intent: first call returns `ErrNeedsConfirmation`, second call with matching `ConfirmToken` succeeds |
| `TestGuardedGit_AskOnPush` | `Push("origin","main")` returns `ErrNeedsConfirmation{Channel:"oob"}` |
| `TestGuardedGit_AllowReadStatus` | `Status()` → default ask path; confirmed retry runs |
| `TestGuardedWorktree_AskCreate` | `Create("x","b")` → default ask; confirmed retry runs |
| `TestGate_SwapIntentRejected` | Approve hash A, retry with intent hashing to B → token mismatch error; raw NOT called |
| `TestGuardedRunner_OptsRefactor` | Existing GuardedRunner deny + allow behaviour preserved after API change |

Each guard test uses a fake raw adapter to assert zero calls on deny and exactly one call on allow/confirmed.

## 10. Commit phasing

Per AGENTS.md §Commit hygiene — explicit files only, one phase per commit, each leaves the tree green.

1. `refactor(policy): introduce CallOpts and gate() helper on GuardedRunner`
2. `feat(policy): add GuardedFileOps wrapping fileops adapter`
3. `feat(policy): add GuardedGit wrapping gitops adapter`
4. `feat(policy): add GuardedWorktree wrapping worktree adapter`
5. `feat(workerd): wire all four guards in main()`
6. `feat(mcp,httpapi): thread confirm_token through delivery layer`

Verification gate before PR: `make build && make test` clean; `policy-demo` verdicts unchanged.

## 11. Open follow-ups (NOT in this spec)

- OOB approval transport (worker console prompt vs loopback endpoint). The harness only needs `Approve(id)` from a trusted place; the channel choice is plan §5 "open follow-up".
- Tuning `.ptolemy/policy.json` to allow common innocuous ops (`git status`, `file.list`) per plan §7.
- Restoring `docs/Architecture.md` and adding per-package paragraphs for the three new guards (per CLAUDE.md DoD).
