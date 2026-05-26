# Guarded Adapters Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `GuardedFileOps`, `GuardedGit`, `GuardedWorktree` to `internal/policy`, refactor `GuardedRunner` onto a unified `CallOpts` API, and wire all four guards through `cmd/workerd` and the existing httpapi/mcp delivery surface.

**Architecture:** A shared private `guardCore` + `gate()` helper inside `internal/policy` performs Authorize → record-to-`policy_decisions` → Allow/Ask/Deny / confirmed-retry. Each new guard struct embeds `guardCore`, holds its raw adapter, and exposes one method per adapter op. Methods take a `CallOpts{ConfirmToken}`. On Allow → call raw. On Ask → park approval, return `ErrNeedsConfirmation{PendingID=intentHash}`. On confirmed retry → verify `opts.ConfirmToken == intentHash` and `ConsumeApproved` succeeds, then call raw. `pendingID == intentHash` defeats swap-intent attacks.

**Tech Stack:** Go 1.25, `database/sql` + `modernc.org/sqlite`, `github.com/google/uuid`, `chi/v5`, existing `internal/policy` engine (no new deps).

**Spec:** [docs/superpowers/specs/2026-05-26-guarded-adapters-design.md](../specs/2026-05-26-guarded-adapters-design.md)

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `internal/policy/guard.go` | Modify | Add `CallOpts`, `guardCore`, `gate()`; add `GuardedFileOps`, `GuardedGit`, `GuardedWorktree`; refactor `GuardedRunner` to opts pattern |
| `internal/policy/guard_test.go` | Modify | Extend with per-guard happy/deny/ask-confirmed + swap-intent regression |
| `internal/command/command.go` | Modify | Update `GuardedRunner` interface signature; thread `CallOpts` through `Service.Run` |
| `internal/command/service_test.go` | Modify | Update `fakeGuardedRunner` to match new signature |
| `cmd/workerd/main.go` | Modify | Construct raw `fileops`/`gitops`/`worktree` adapters and all four guards; hand guards to services |
| `internal/httpapi/router.go` | Modify | Confirm `confirm_token` round-trip survives the refactor (request DTO already has `pending_id`) |
| `internal/mcp/tools.go` | Modify | Same — verify the MCP tool surface still routes `pending_id` correctly |

Unchanged: `internal/fileops/`, `internal/gitops/`, `internal/worktree/`, `internal/domain/`, `.ptolemy/policy.json`, `internal/policy/{engine,rules,approve,match}.go`.

---

## Task 1: Refactor `GuardedRunner` onto `CallOpts` + introduce `gate()`

**Files:**
- Modify: `internal/policy/guard.go`
- Modify: `internal/policy/guard_test.go`
- Modify: `internal/command/command.go`
- Modify: `internal/command/service_test.go`

- [ ] **Step 1: Update the failing tests for the new signature**

Replace the two existing tests in `internal/policy/guard_test.go` with the same coverage against the new API:

```go
package policy

import (
	"context"
	"errors"
	"testing"

	"github.com/luannn010/ptolemy/internal/store"
	"github.com/luannn010/ptolemy/internal/terminal"
)

type fakeRunner struct {
	calls int
	res   terminal.Result
}

func (f *fakeRunner) Run(_ context.Context, _ string, _ string, _ int) terminal.Result {
	f.calls++
	return f.res
}

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/db.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err := s.DB.Exec(`INSERT INTO sessions(id,name,status,workspace,description,created_at,updated_at)
		VALUES('s1','n','open','.','','x','x')`); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestGuardedRunnerDenyDoesNotExecute(t *testing.T) {
	s := openTestStore(t)
	r := &fakeRunner{}
	g := NewGuardedRunner(NewEngine(DefaultRuleset()), NewApprovals(), r, s.DB)

	_, err := g.Run(context.Background(), "s1", "cat ./.env", ".", 5, CallOpts{})
	var denied ErrDenied
	if !errors.As(err, &denied) {
		t.Fatalf("expected ErrDenied, got %v", err)
	}
	if r.calls != 0 {
		t.Fatalf("raw runner must not be called on deny, got %d", r.calls)
	}
}

func TestGuardedRunnerAllowExecutes(t *testing.T) {
	s := openTestStore(t)
	r := &fakeRunner{res: terminal.Result{ExitCode: 0}}
	g := NewGuardedRunner(NewEngine(DefaultRuleset()), NewApprovals(), r, s.DB)

	_, err := g.Run(context.Background(), "s1", "go test ./...", ".", 5, CallOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.calls != 1 {
		t.Fatalf("expected one execution, got %d", r.calls)
	}
}

func TestGuardedRunnerAskThenConfirm(t *testing.T) {
	s := openTestStore(t)
	r := &fakeRunner{res: terminal.Result{ExitCode: 0}}
	g := NewGuardedRunner(NewEngine(DefaultRuleset()), NewApprovals(), r, s.DB)

	_, err := g.Run(context.Background(), "s1", "git push origin main", ".", 5, CallOpts{})
	var needs ErrNeedsConfirmation
	if !errors.As(err, &needs) {
		t.Fatalf("expected ErrNeedsConfirmation, got %v", err)
	}
	if !g.approvals.Approve(needs.PendingID) {
		t.Fatalf("approve failed")
	}
	if _, err := g.Run(context.Background(), "s1", "git push origin main", ".", 5, CallOpts{ConfirmToken: needs.PendingID}); err != nil {
		t.Fatalf("confirmed retry failed: %v", err)
	}
	if r.calls != 1 {
		t.Fatalf("expected one execution after confirmation, got %d", r.calls)
	}
}
```

- [ ] **Step 2: Run the tests; expect compile failure**

Run: `go test ./internal/policy/...`
Expected: build errors — `CallOpts undefined`, `GuardedRunner.Run` arity mismatch.

- [ ] **Step 3: Refactor `internal/policy/guard.go`**

Replace the file contents with:

```go
package policy

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/luannn010/ptolemy/internal/domain"
	"github.com/luannn010/ptolemy/internal/fileops"
	"github.com/luannn010/ptolemy/internal/gitops"
	"github.com/luannn010/ptolemy/internal/terminal"
	"github.com/luannn010/ptolemy/internal/worktree"
)

type CallOpts struct {
	ConfirmToken string
}

type ErrNeedsConfirmation struct {
	PendingID string
	Channel   domain.Channel
	Reason    string
}

func (e ErrNeedsConfirmation) Error() string { return "needs confirmation" }

type ErrDenied struct {
	RuleID string
	Reason string
}

func (e ErrDenied) Error() string { return "denied: " + e.Reason }

type guardCore struct {
	engine    *Engine
	approvals *Approvals
	db        *sql.DB
}

// gate runs Authorize → record → Allow/Ask/Deny / confirmed-retry. Returns nil
// when the caller should proceed with the raw adapter; ErrDenied / ErrNeedsConfirmation
// otherwise. pendingID == intentHash by design — the token IS the hash, so a
// swap-intent attack (approve A, retry with B) fails the equality check.
func (g *guardCore) gate(ctx context.Context, sessionID string, intent domain.Intent, opts CallOpts) error {
	decision := g.engine.Authorize(intent)
	hash := hashIntent(intent)

	if opts.ConfirmToken != "" {
		if opts.ConfirmToken != hash || !g.approvals.ConsumeApproved(opts.ConfirmToken) {
			return errors.New("approval not granted")
		}
		if _, err := g.db.ExecContext(ctx, `UPDATE policy_decisions SET confirmed = 1 WHERE id = ?`, hash); err != nil {
			return err
		}
		return nil
	}

	if err := g.recordDecision(ctx, hash, sessionID, intent, decision, hash, false); err != nil {
		return err
	}

	switch decision.Effect {
	case domain.EffectAllow:
		return nil
	case domain.EffectAsk:
		g.approvals.Park(hash)
		return ErrNeedsConfirmation{PendingID: hash, Channel: decision.Channel, Reason: decision.Reason}
	default:
		return ErrDenied{RuleID: decision.RuleID, Reason: decision.Reason}
	}
}

func (g *guardCore) recordDecision(ctx context.Context, id, sessionID string, intent domain.Intent, dec domain.Decision, intentHash string, confirmed bool) error {
	_, err := g.db.ExecContext(
		ctx,
		`INSERT INTO policy_decisions
		(id, session_id, intent_kind, program, target, effect, channel, rule_id, reason, intent_hash, confirmed, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, sessionID, intent.Kind, intent.Program, first(intent.Targets),
		string(dec.Effect), string(dec.Channel), dec.RuleID, dec.Reason,
		intentHash, boolToInt(confirmed),
		time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// ---------- GuardedRunner (refactored) ----------

type RawRunner interface {
	Run(ctx context.Context, command string, cwd string, timeoutSeconds int) terminal.Result
}

type GuardedRunner struct {
	core guardCore
	raw  RawRunner
	// expose approvals for tests; callers in production use Approvals via approve endpoint
	approvals *Approvals
}

func NewGuardedRunner(engine *Engine, approvals *Approvals, raw RawRunner, db *sql.DB) *GuardedRunner {
	return &GuardedRunner{
		core:      guardCore{engine: engine, approvals: approvals, db: db},
		raw:       raw,
		approvals: approvals,
	}
}

func (g *GuardedRunner) Run(ctx context.Context, sessionID, command, cwd string, timeoutSeconds int, opts CallOpts) (terminal.Result, error) {
	intent := domain.Intent{Kind: "command.exec", Program: "shell", Args: []string{command}, Targets: []string{cwd}}
	if err := g.core.gate(ctx, sessionID, intent, opts); err != nil {
		return terminal.Result{}, err
	}
	return g.raw.Run(ctx, command, cwd, timeoutSeconds), nil
}

// ---------- helpers ----------

func hashIntent(intent domain.Intent) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%s",
		intent.Kind, intent.Program,
		strings.Join(intent.Args, "|"),
		strings.Join(intent.Targets, "|"),
	)))
	return hex.EncodeToString(h[:])
}

func first(items []string) string {
	if len(items) == 0 {
		return ""
	}
	return items[0]
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// Re-export adapter package names so go vet doesn't complain about unused imports
// in this file when guards are added in later commits.
var _ = fileops.New
var _ = gitops.New
var _ = worktree.NewManager
```

- [ ] **Step 4: Update `internal/command/command.go`**

Update the `GuardedRunner` interface and `Service.Run` to use `CallOpts`:

```go
// Replace the GuardedRunner interface and runConfirmed helper
type GuardedRunner interface {
	Run(ctx context.Context, sessionID, command, cwd string, timeoutSeconds int, opts policy.CallOpts) (terminal.Result, error)
}

// Service.Run becomes:
func (s *Service) Run(ctx context.Context, sessionID string, req RunCommandRequest) (RunResult, error) {
	opts := policy.CallOpts{ConfirmToken: req.PendingID}
	runResult, err := s.runner.Run(ctx, sessionID, req.Command, req.CWD, req.Timeout, opts)
	if err != nil {
		var needs policy.ErrNeedsConfirmation
		if errors.As(err, &needs) {
			return RunResult{
				Confirmation: &Confirmation{
					PendingID: needs.PendingID,
					Channel:   needs.Channel,
					Reason:    needs.Reason,
				},
			}, nil
		}
		return RunResult{}, err
	}

	logItem, err := s.logs.Create(ctx, CommandLog{
		ID:          uuid.NewString(),
		SessionID:   sessionID,
		Command:     req.Command,
		CWD:         req.CWD,
		ExitCode:    runResult.ExitCode,
		Output:      runResult.Output,
		ErrorOutput: runResult.ErrorOutput,
		DurationMS:  runResult.DurationMS,
		CreatedAt:   time.Now().UTC(),
	})
	if err != nil {
		return RunResult{}, err
	}
	return RunResult{Log: &logItem}, nil
}
```

Delete `runConfirmed` — it's no longer needed (the single `Run` handles both cases via `opts`). Remove the unused `uuid` import only if it becomes unused; otherwise leave it.

- [ ] **Step 5: Update `internal/command/service_test.go`**

Update `fakeGuardedRunner` to match. Replace its two methods with:

```go
type fakeGuardedRunner struct {
	runCalls int
	res      terminal.Result
	err      error
}

func (f *fakeGuardedRunner) Run(_ context.Context, _ string, _ string, _ string, _ int, _ policy.CallOpts) (terminal.Result, error) {
	f.runCalls++
	return f.res, f.err
}
```

If existing tests instantiated the fake with both `Run` and `RunConfirmed` call counters, fold them into the single `runCalls`. Adjust any assertion that counted `runConfirmedCalls` to use `runCalls` instead.

Add `"github.com/luannn010/ptolemy/internal/policy"` to the test imports if not already present.

- [ ] **Step 6: Run all tests; expect green**

Run: `go build ./... && go test ./internal/policy/... ./internal/command/...`
Expected: PASS.

- [ ] **Step 7: Run the full suite**

Run: `make test`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/policy/guard.go internal/policy/guard_test.go internal/command/command.go internal/command/service_test.go
git commit -m "refactor(policy): introduce CallOpts and gate() helper on GuardedRunner

Collapse Run/RunConfirmed into a single Run(..., CallOpts) method
backed by a shared guardCore.gate() pipeline. pendingID is now the
intent SHA-256 hash so swap-intent attacks fail at the equality
check. This sets the API shape that GuardedFileOps/GuardedGit/
GuardedWorktree will mirror in subsequent commits.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: `GuardedFileOps`

**Files:**
- Modify: `internal/policy/guard.go`
- Modify: `internal/policy/guard_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/policy/guard_test.go`:

```go
type fakeFileOps struct {
	reads  int
	writes int
	lists  int
}

func (f *fakeFileOps) Resolve(path string) (string, error)             { return path, nil }
func (f *fakeFileOps) ReadFile(path string) (string, error)            { f.reads++; return "content", nil }
func (f *fakeFileOps) WriteFile(path, content string) error            { f.writes++; return nil }
func (f *fakeFileOps) ListDirectory(path string) ([]fileops.DirEntry, error) {
	f.lists++
	return nil, nil
}
func (f *fakeFileOps) Search(query string) (string, error)             { return "", nil }
func (f *fakeFileOps) ApplyPatch(path, newContent string) error        { return nil }
func (f *fakeFileOps) ReplaceBlock(path, oldS, newS string) error      { return nil }
func (f *fakeFileOps) InsertAfter(path, marker, content string) error  { return nil }

func TestGuardedFileOps_DenyPolicyWrite(t *testing.T) {
	s := openTestStore(t)
	f := &fakeFileOps{}
	g := NewGuardedFileOps(NewEngine(DefaultRuleset()), NewApprovals(), f, s.DB)

	err := g.WriteFile(context.Background(), "s1", ".ptolemy/policy.json", "{}", CallOpts{})
	var denied ErrDenied
	if !errors.As(err, &denied) || denied.RuleID != "deny-policy-write" {
		t.Fatalf("expected deny-policy-write, got %v", err)
	}
	if f.writes != 0 {
		t.Fatalf("raw adapter must not be called on deny")
	}
}

func TestGuardedFileOps_DenySecretRead(t *testing.T) {
	s := openTestStore(t)
	f := &fakeFileOps{}
	g := NewGuardedFileOps(NewEngine(DefaultRuleset()), NewApprovals(), f, s.DB)

	_, err := g.ReadFile(context.Background(), "s1", "./.env", CallOpts{})
	var denied ErrDenied
	if !errors.As(err, &denied) || denied.RuleID != "deny-secret-cmd" {
		t.Fatalf("expected deny-secret-cmd, got %v", err)
	}
	if f.reads != 0 {
		t.Fatalf("raw adapter must not be called on deny")
	}
}

func TestGuardedFileOps_AskThenConfirmList(t *testing.T) {
	s := openTestStore(t)
	f := &fakeFileOps{}
	g := NewGuardedFileOps(NewEngine(DefaultRuleset()), NewApprovals(), f, s.DB)

	_, err := g.ListDirectory(context.Background(), "s1", "/some/dir", CallOpts{})
	var needs ErrNeedsConfirmation
	if !errors.As(err, &needs) {
		t.Fatalf("expected ErrNeedsConfirmation, got %v", err)
	}
	if !g.core.approvals.Approve(needs.PendingID) {
		t.Fatalf("approve failed")
	}
	if _, err := g.ListDirectory(context.Background(), "s1", "/some/dir", CallOpts{ConfirmToken: needs.PendingID}); err != nil {
		t.Fatalf("confirmed retry failed: %v", err)
	}
	if f.lists != 1 {
		t.Fatalf("expected exactly one list call after confirmation, got %d", f.lists)
	}
}

func TestGate_SwapIntentRejected(t *testing.T) {
	s := openTestStore(t)
	f := &fakeFileOps{}
	g := NewGuardedFileOps(NewEngine(DefaultRuleset()), NewApprovals(), f, s.DB)

	// Ask for List on /a → token for /a
	_, err := g.ListDirectory(context.Background(), "s1", "/a", CallOpts{})
	var needs ErrNeedsConfirmation
	if !errors.As(err, &needs) {
		t.Fatalf("expected ask, got %v", err)
	}
	g.core.approvals.Approve(needs.PendingID)

	// Retry with token from /a but a different path /b → must reject
	_, err = g.ListDirectory(context.Background(), "s1", "/b", CallOpts{ConfirmToken: needs.PendingID})
	if err == nil {
		t.Fatalf("expected swap-intent to be rejected, got nil")
	}
	if f.lists != 0 {
		t.Fatalf("raw must not be called on swap-intent")
	}
}
```

Add `"github.com/luannn010/ptolemy/internal/fileops"` to the test imports.

- [ ] **Step 2: Run tests; expect compile failure**

Run: `go test ./internal/policy/...`
Expected: `NewGuardedFileOps undefined`, `GuardedFileOps undefined`.

- [ ] **Step 3: Implement `GuardedFileOps`**

In `internal/policy/guard.go`, replace the `var _ = fileops.New` line with:

```go
// ---------- GuardedFileOps ----------

type RawFileOps interface {
	Resolve(path string) (string, error)
	ReadFile(path string) (string, error)
	WriteFile(path, content string) error
	ListDirectory(path string) ([]fileops.DirEntry, error)
	Search(query string) (string, error)
	ApplyPatch(path, newContent string) error
	ReplaceBlock(path, old, new string) error
	InsertAfter(path, marker, content string) error
}

type GuardedFileOps struct {
	core guardCore
	raw  RawFileOps
}

func NewGuardedFileOps(engine *Engine, approvals *Approvals, raw RawFileOps, db *sql.DB) *GuardedFileOps {
	return &GuardedFileOps{core: guardCore{engine: engine, approvals: approvals, db: db}, raw: raw}
}

func (g *GuardedFileOps) fileIntent(kind, program string, path string, extraArgs ...string) domain.Intent {
	resolved, err := g.raw.Resolve(path)
	if err != nil {
		// fall back to raw path so policy still gets a chance to match on it
		resolved = path
	}
	return domain.Intent{Kind: kind, Program: program, Args: extraArgs, Targets: []string{resolved}}
}

func (g *GuardedFileOps) Resolve(ctx context.Context, sessionID, path string, opts CallOpts) (string, error) {
	intent := g.fileIntent("file.resolve", "resolve", path)
	if err := g.core.gate(ctx, sessionID, intent, opts); err != nil {
		return "", err
	}
	return g.raw.Resolve(path)
}

func (g *GuardedFileOps) ReadFile(ctx context.Context, sessionID, path string, opts CallOpts) (string, error) {
	intent := g.fileIntent("file.read", "read", path)
	if err := g.core.gate(ctx, sessionID, intent, opts); err != nil {
		return "", err
	}
	return g.raw.ReadFile(path)
}

func (g *GuardedFileOps) WriteFile(ctx context.Context, sessionID, path, content string, opts CallOpts) error {
	intent := g.fileIntent("file.write", "write", path)
	if err := g.core.gate(ctx, sessionID, intent, opts); err != nil {
		return err
	}
	return g.raw.WriteFile(path, content)
}

func (g *GuardedFileOps) ListDirectory(ctx context.Context, sessionID, path string, opts CallOpts) ([]fileops.DirEntry, error) {
	intent := g.fileIntent("file.list", "list", path)
	if err := g.core.gate(ctx, sessionID, intent, opts); err != nil {
		return nil, err
	}
	return g.raw.ListDirectory(path)
}

func (g *GuardedFileOps) Search(ctx context.Context, sessionID, query string, opts CallOpts) (string, error) {
	intent := domain.Intent{Kind: "file.search", Program: "search", Args: []string{query}}
	if err := g.core.gate(ctx, sessionID, intent, opts); err != nil {
		return "", err
	}
	return g.raw.Search(query)
}

func (g *GuardedFileOps) ApplyPatch(ctx context.Context, sessionID, path, newContent string, opts CallOpts) error {
	intent := g.fileIntent("file.patch", "patch", path)
	if err := g.core.gate(ctx, sessionID, intent, opts); err != nil {
		return err
	}
	return g.raw.ApplyPatch(path, newContent)
}

func (g *GuardedFileOps) ReplaceBlock(ctx context.Context, sessionID, path, oldS, newS string, opts CallOpts) error {
	intent := g.fileIntent("file.replace", "replace", path)
	if err := g.core.gate(ctx, sessionID, intent, opts); err != nil {
		return err
	}
	return g.raw.ReplaceBlock(path, oldS, newS)
}

func (g *GuardedFileOps) InsertAfter(ctx context.Context, sessionID, path, marker, content string, opts CallOpts) error {
	intent := g.fileIntent("file.insert", "insert", path, marker)
	if err := g.core.gate(ctx, sessionID, intent, opts); err != nil {
		return err
	}
	return g.raw.InsertAfter(path, marker, content)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/policy/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/policy/guard.go internal/policy/guard_test.go
git commit -m "feat(policy): add GuardedFileOps wrapping fileops adapter

All 8 fileops methods now route through guardCore.gate() before
calling the raw adapter. The fileIntent helper resolves paths via
raw.Resolve so Targets carry the absolute path, letting existing
rules (deny-policy-write, deny-secret-cmd) catch adapter-side
access of .ptolemy/policy.json and .env. Tests cover deny on
policy-write and secret-read, ask-then-confirm on a default
intent, and swap-intent rejection.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: `GuardedGit`

**Files:**
- Modify: `internal/policy/guard.go`
- Modify: `internal/policy/guard_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/policy/guard_test.go`:

```go
type fakeGitOps struct {
	pushes  int
	statuses int
}

func (f *fakeGitOps) Status(_ context.Context) gitops.Result                     { f.statuses++; return gitops.Result{} }
func (f *fakeGitOps) Diff(_ context.Context) gitops.Result                       { return gitops.Result{} }
func (f *fakeGitOps) Log(_ context.Context) gitops.Result                        { return gitops.Result{} }
func (f *fakeGitOps) CurrentBranch(_ context.Context) gitops.Result              { return gitops.Result{} }
func (f *fakeGitOps) CurrentCommitSHA(_ context.Context) gitops.Result           { return gitops.Result{} }
func (f *fakeGitOps) ChangedFiles(_ context.Context) gitops.Result               { return gitops.Result{} }
func (f *fakeGitOps) Checkout(_ context.Context, _ string) gitops.Result         { return gitops.Result{} }
func (f *fakeGitOps) CreateBranch(_ context.Context, _ string) gitops.Result     { return gitops.Result{} }
func (f *fakeGitOps) EnsureBranch(_ context.Context, _ string) gitops.Result     { return gitops.Result{} }
func (f *fakeGitOps) CreateOrResetBranchFrom(_ context.Context, _, _ string) gitops.Result {
	return gitops.Result{}
}
func (f *fakeGitOps) StageFiles(_ context.Context, _ []string) gitops.Result            { return gitops.Result{} }
func (f *fakeGitOps) CommitConventional(_ context.Context, _ string) gitops.Result      { return gitops.Result{} }
func (f *fakeGitOps) CommitStagedConventional(_ context.Context, _ string) gitops.Result { return gitops.Result{} }
func (f *fakeGitOps) MergeNoFF(_ context.Context, _ string) gitops.Result               { return gitops.Result{} }
func (f *fakeGitOps) Push(_ context.Context, _, _ string) gitops.Result {
	f.pushes++
	return gitops.Result{}
}
func (f *fakeGitOps) CreatePullRequest(_ context.Context, _, _, _, _ string) gitops.Result {
	return gitops.Result{}
}

func TestGuardedGit_AskOnPush(t *testing.T) {
	s := openTestStore(t)
	gops := &fakeGitOps{}
	g := NewGuardedGit(NewEngine(DefaultRuleset()), NewApprovals(), gops, "/repo", s.DB)

	_, err := g.Push(context.Background(), "s1", "origin", "main", CallOpts{})
	var needs ErrNeedsConfirmation
	if !errors.As(err, &needs) {
		t.Fatalf("expected ask, got %v", err)
	}
	if gops.pushes != 0 {
		t.Fatalf("raw push must not run before approval")
	}

	g.core.approvals.Approve(needs.PendingID)
	if _, err := g.Push(context.Background(), "s1", "origin", "main", CallOpts{ConfirmToken: needs.PendingID}); err != nil {
		t.Fatalf("confirmed push failed: %v", err)
	}
	if gops.pushes != 1 {
		t.Fatalf("expected one push after confirm, got %d", gops.pushes)
	}
}

func TestGuardedGit_DefaultAskOnStatus(t *testing.T) {
	s := openTestStore(t)
	gops := &fakeGitOps{}
	g := NewGuardedGit(NewEngine(DefaultRuleset()), NewApprovals(), gops, "/repo", s.DB)

	_, err := g.Status(context.Background(), "s1", CallOpts{})
	var needs ErrNeedsConfirmation
	if !errors.As(err, &needs) {
		t.Fatalf("expected default ask on git.read, got %v", err)
	}
	if gops.statuses != 0 {
		t.Fatalf("raw status must not run before approval")
	}
}
```

Add `"github.com/luannn010/ptolemy/internal/gitops"` to the test imports.

- [ ] **Step 2: Run tests; expect compile failure**

Run: `go test ./internal/policy/...`
Expected: `NewGuardedGit undefined`.

- [ ] **Step 3: Implement `GuardedGit`**

Append to `internal/policy/guard.go` (after `GuardedFileOps`):

```go
// ---------- GuardedGit ----------

type RawGitOps interface {
	Status(ctx context.Context) gitops.Result
	Diff(ctx context.Context) gitops.Result
	Log(ctx context.Context) gitops.Result
	CurrentBranch(ctx context.Context) gitops.Result
	CurrentCommitSHA(ctx context.Context) gitops.Result
	ChangedFiles(ctx context.Context) gitops.Result
	Checkout(ctx context.Context, branch string) gitops.Result
	CreateBranch(ctx context.Context, branch string) gitops.Result
	EnsureBranch(ctx context.Context, branch string) gitops.Result
	CreateOrResetBranchFrom(ctx context.Context, branch, startPoint string) gitops.Result
	StageFiles(ctx context.Context, files []string) gitops.Result
	CommitConventional(ctx context.Context, message string) gitops.Result
	CommitStagedConventional(ctx context.Context, message string) gitops.Result
	MergeNoFF(ctx context.Context, branch string) gitops.Result
	Push(ctx context.Context, remote, branch string) gitops.Result
	CreatePullRequest(ctx context.Context, base, head, title, bodyFile string) gitops.Result
}

type GuardedGit struct {
	core     guardCore
	raw      RawGitOps
	repoPath string
}

func NewGuardedGit(engine *Engine, approvals *Approvals, raw RawGitOps, repoPath string, db *sql.DB) *GuardedGit {
	return &GuardedGit{core: guardCore{engine: engine, approvals: approvals, db: db}, raw: raw, repoPath: repoPath}
}

func (g *GuardedGit) gitIntent(kind, subcmd string, args ...string) domain.Intent {
	return domain.Intent{
		Kind:    kind,
		Program: "git",
		Args:    append([]string{subcmd}, args...),
		Targets: []string{g.repoPath},
	}
}

// Read-only group → kind "git.read"
func (g *GuardedGit) Status(ctx context.Context, sessionID string, opts CallOpts) (gitops.Result, error) {
	if err := g.core.gate(ctx, sessionID, g.gitIntent("git.read", "status"), opts); err != nil {
		return gitops.Result{}, err
	}
	return g.raw.Status(ctx), nil
}
func (g *GuardedGit) Diff(ctx context.Context, sessionID string, opts CallOpts) (gitops.Result, error) {
	if err := g.core.gate(ctx, sessionID, g.gitIntent("git.read", "diff"), opts); err != nil {
		return gitops.Result{}, err
	}
	return g.raw.Diff(ctx), nil
}
func (g *GuardedGit) Log(ctx context.Context, sessionID string, opts CallOpts) (gitops.Result, error) {
	if err := g.core.gate(ctx, sessionID, g.gitIntent("git.read", "log"), opts); err != nil {
		return gitops.Result{}, err
	}
	return g.raw.Log(ctx), nil
}
func (g *GuardedGit) CurrentBranch(ctx context.Context, sessionID string, opts CallOpts) (gitops.Result, error) {
	if err := g.core.gate(ctx, sessionID, g.gitIntent("git.read", "branch"), opts); err != nil {
		return gitops.Result{}, err
	}
	return g.raw.CurrentBranch(ctx), nil
}
func (g *GuardedGit) CurrentCommitSHA(ctx context.Context, sessionID string, opts CallOpts) (gitops.Result, error) {
	if err := g.core.gate(ctx, sessionID, g.gitIntent("git.read", "rev-parse"), opts); err != nil {
		return gitops.Result{}, err
	}
	return g.raw.CurrentCommitSHA(ctx), nil
}
func (g *GuardedGit) ChangedFiles(ctx context.Context, sessionID string, opts CallOpts) (gitops.Result, error) {
	if err := g.core.gate(ctx, sessionID, g.gitIntent("git.read", "diff-files"), opts); err != nil {
		return gitops.Result{}, err
	}
	return g.raw.ChangedFiles(ctx), nil
}

// Mutating group
func (g *GuardedGit) Checkout(ctx context.Context, sessionID, branch string, opts CallOpts) (gitops.Result, error) {
	if err := g.core.gate(ctx, sessionID, g.gitIntent("git.branch", "checkout", branch), opts); err != nil {
		return gitops.Result{}, err
	}
	return g.raw.Checkout(ctx, branch), nil
}
func (g *GuardedGit) CreateBranch(ctx context.Context, sessionID, branch string, opts CallOpts) (gitops.Result, error) {
	if err := g.core.gate(ctx, sessionID, g.gitIntent("git.branch", "branch", branch), opts); err != nil {
		return gitops.Result{}, err
	}
	return g.raw.CreateBranch(ctx, branch), nil
}
func (g *GuardedGit) EnsureBranch(ctx context.Context, sessionID, branch string, opts CallOpts) (gitops.Result, error) {
	if err := g.core.gate(ctx, sessionID, g.gitIntent("git.branch", "ensure", branch), opts); err != nil {
		return gitops.Result{}, err
	}
	return g.raw.EnsureBranch(ctx, branch), nil
}
func (g *GuardedGit) CreateOrResetBranchFrom(ctx context.Context, sessionID, branch, startPoint string, opts CallOpts) (gitops.Result, error) {
	if err := g.core.gate(ctx, sessionID, g.gitIntent("git.branch", "reset-from", branch, startPoint), opts); err != nil {
		return gitops.Result{}, err
	}
	return g.raw.CreateOrResetBranchFrom(ctx, branch, startPoint), nil
}
func (g *GuardedGit) StageFiles(ctx context.Context, sessionID string, files []string, opts CallOpts) (gitops.Result, error) {
	if err := g.core.gate(ctx, sessionID, g.gitIntent("git.stage", "add", files...), opts); err != nil {
		return gitops.Result{}, err
	}
	return g.raw.StageFiles(ctx, files), nil
}
func (g *GuardedGit) CommitConventional(ctx context.Context, sessionID, message string, opts CallOpts) (gitops.Result, error) {
	if err := g.core.gate(ctx, sessionID, g.gitIntent("git.commit", "commit"), opts); err != nil {
		return gitops.Result{}, err
	}
	return g.raw.CommitConventional(ctx, message), nil
}
func (g *GuardedGit) CommitStagedConventional(ctx context.Context, sessionID, message string, opts CallOpts) (gitops.Result, error) {
	if err := g.core.gate(ctx, sessionID, g.gitIntent("git.commit", "commit-staged"), opts); err != nil {
		return gitops.Result{}, err
	}
	return g.raw.CommitStagedConventional(ctx, message), nil
}
func (g *GuardedGit) MergeNoFF(ctx context.Context, sessionID, branch string, opts CallOpts) (gitops.Result, error) {
	if err := g.core.gate(ctx, sessionID, g.gitIntent("git.merge", "merge", branch), opts); err != nil {
		return gitops.Result{}, err
	}
	return g.raw.MergeNoFF(ctx, branch), nil
}
func (g *GuardedGit) Push(ctx context.Context, sessionID, remote, branch string, opts CallOpts) (gitops.Result, error) {
	if err := g.core.gate(ctx, sessionID, g.gitIntent("git.push", "push", remote, branch), opts); err != nil {
		return gitops.Result{}, err
	}
	return g.raw.Push(ctx, remote, branch), nil
}
func (g *GuardedGit) CreatePullRequest(ctx context.Context, sessionID, base, head, title, bodyFile string, opts CallOpts) (gitops.Result, error) {
	intent := domain.Intent{Kind: "git.pr", Program: "gh", Args: []string{"pr", "create", base, head, title}, Targets: []string{g.repoPath}}
	if err := g.core.gate(ctx, sessionID, intent, opts); err != nil {
		return gitops.Result{}, err
	}
	return g.raw.CreatePullRequest(ctx, base, head, title, bodyFile), nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/policy/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/policy/guard.go internal/policy/guard_test.go
git commit -m "feat(policy): add GuardedGit wrapping gitops adapter

All 16 gitops methods now route through guardCore.gate(). Read-only
ops share kind git.read; mutating ops get distinct kinds (git.branch,
git.stage, git.commit, git.merge, git.push, git.pr) so rules can
target Push independently of Commit. Tests cover ask-then-confirm on
Push (rule ask-push-cmd) and default-ask on Status.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: `GuardedWorktree`

**Files:**
- Modify: `internal/policy/guard.go`
- Modify: `internal/policy/guard_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/policy/guard_test.go`:

```go
type fakeWorktree struct {
	creates int
	removes int
}

func (f *fakeWorktree) Create(_ context.Context, _, _ string) worktree.Result {
	f.creates++
	return worktree.Result{}
}
func (f *fakeWorktree) AddExisting(_ context.Context, _, _ string) worktree.Result {
	return worktree.Result{}
}
func (f *fakeWorktree) Remove(_ context.Context, _ string) worktree.Result {
	f.removes++
	return worktree.Result{}
}
func (f *fakeWorktree) List(_ context.Context) worktree.Result { return worktree.Result{} }

func TestGuardedWorktree_AskOnCreate(t *testing.T) {
	s := openTestStore(t)
	w := &fakeWorktree{}
	g := NewGuardedWorktree(NewEngine(DefaultRuleset()), NewApprovals(), w, "/wt", s.DB)

	_, err := g.Create(context.Background(), "s1", "x", "main", CallOpts{})
	var needs ErrNeedsConfirmation
	if !errors.As(err, &needs) {
		t.Fatalf("expected default ask, got %v", err)
	}
	if w.creates != 0 {
		t.Fatalf("raw create must not run before approval")
	}

	g.core.approvals.Approve(needs.PendingID)
	if _, err := g.Create(context.Background(), "s1", "x", "main", CallOpts{ConfirmToken: needs.PendingID}); err != nil {
		t.Fatalf("confirmed create failed: %v", err)
	}
	if w.creates != 1 {
		t.Fatalf("expected one create after confirm, got %d", w.creates)
	}
}
```

Add `"github.com/luannn010/ptolemy/internal/worktree"` to the test imports.

- [ ] **Step 2: Run tests; expect compile failure**

Run: `go test ./internal/policy/...`
Expected: `NewGuardedWorktree undefined`.

- [ ] **Step 3: Implement `GuardedWorktree`**

Append to `internal/policy/guard.go`:

```go
// ---------- GuardedWorktree ----------

type RawWorktree interface {
	Create(ctx context.Context, name, branch string) worktree.Result
	AddExisting(ctx context.Context, name, branch string) worktree.Result
	Remove(ctx context.Context, name string) worktree.Result
	List(ctx context.Context) worktree.Result
}

type GuardedWorktree struct {
	core         guardCore
	raw          RawWorktree
	worktreePath string
}

func NewGuardedWorktree(engine *Engine, approvals *Approvals, raw RawWorktree, worktreePath string, db *sql.DB) *GuardedWorktree {
	return &GuardedWorktree{core: guardCore{engine: engine, approvals: approvals, db: db}, raw: raw, worktreePath: worktreePath}
}

func (g *GuardedWorktree) wtIntent(kind string, args ...string) domain.Intent {
	return domain.Intent{Kind: kind, Program: "worktree", Args: args, Targets: []string{g.worktreePath}}
}

func (g *GuardedWorktree) Create(ctx context.Context, sessionID, name, branch string, opts CallOpts) (worktree.Result, error) {
	if err := g.core.gate(ctx, sessionID, g.wtIntent("worktree.create", "create", name, branch), opts); err != nil {
		return worktree.Result{}, err
	}
	return g.raw.Create(ctx, name, branch), nil
}

func (g *GuardedWorktree) AddExisting(ctx context.Context, sessionID, name, branch string, opts CallOpts) (worktree.Result, error) {
	if err := g.core.gate(ctx, sessionID, g.wtIntent("worktree.add", "add", name, branch), opts); err != nil {
		return worktree.Result{}, err
	}
	return g.raw.AddExisting(ctx, name, branch), nil
}

func (g *GuardedWorktree) Remove(ctx context.Context, sessionID, name string, opts CallOpts) (worktree.Result, error) {
	if err := g.core.gate(ctx, sessionID, g.wtIntent("worktree.remove", "remove", name), opts); err != nil {
		return worktree.Result{}, err
	}
	return g.raw.Remove(ctx, name), nil
}

func (g *GuardedWorktree) List(ctx context.Context, sessionID string, opts CallOpts) (worktree.Result, error) {
	if err := g.core.gate(ctx, sessionID, g.wtIntent("worktree.list", "list"), opts); err != nil {
		return worktree.Result{}, err
	}
	return g.raw.List(ctx), nil
}
```

Finally, remove the no-longer-needed `var _ = ...` re-exports at the bottom of `guard.go` (the packages are now used by the guard types).

- [ ] **Step 4: Run tests**

Run: `go test ./internal/policy/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/policy/guard.go internal/policy/guard_test.go
git commit -m "feat(policy): add GuardedWorktree wrapping worktree adapter

All 4 worktree methods (Create, AddExisting, Remove, List) route
through guardCore.gate(). Default-ask path is the expected verdict
today; once policy is tuned per plan §7, allows can be added for
List and AddExisting.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: Wire all four guards in `cmd/workerd/main.go`

**Files:**
- Modify: `cmd/workerd/main.go`

- [ ] **Step 1: Read the current main.go**

Run: `cat cmd/workerd/main.go`
Note where `rawRunner` is constructed and where `guardedRunner` is built (around line 41).

- [ ] **Step 2: Update main.go**

Around the existing `guardedRunner` construction, replace and extend with:

```go
// Raw adapters
rawRunner := terminal.NewRunner() // (existing — keep whatever constructor is used)
rawFileOps := fileops.New(cfg.WorkspaceRoot) // pick the existing workspace-root config field
rawGit := gitops.New(cfg.WorkspaceRoot)
rawWorktrees := worktree.NewManager(cfg.WorkspaceRoot, cfg.WorktreeDir) // if WorktreeDir doesn't exist, pass filepath.Join(cfg.WorkspaceRoot, ".worktrees")

// Guards — the only place raw adapters are visible to services
guardedRunner   := policy.NewGuardedRunner(engine, approvals, rawRunner, baseStore.SQLDB())
guardedFileOps  := policy.NewGuardedFileOps(engine, approvals, rawFileOps, baseStore.SQLDB())
guardedGit      := policy.NewGuardedGit(engine, approvals, rawGit, cfg.WorkspaceRoot, baseStore.SQLDB())
guardedWorktree := policy.NewGuardedWorktree(engine, approvals, rawWorktrees, cfg.WorkspaceRoot, baseStore.SQLDB())

// Silence unused-variable errors for guards not yet consumed by services.
// These will be threaded into session/workspace/command services in follow-up work.
_ = guardedFileOps
_ = guardedGit
_ = guardedWorktree
```

Add imports as needed: `"github.com/luannn010/ptolemy/internal/fileops"`, `"github.com/luannn010/ptolemy/internal/gitops"`, `"github.com/luannn010/ptolemy/internal/worktree"`.

The `_ = ...` lines are deliberate — they prove the wiring compiles without forcing a parallel rewrite of every service in this PR. The follow-up task (not in this PR) is to thread these into the relevant services.

- [ ] **Step 3: Build the binary**

Run: `go build ./cmd/workerd`
Expected: clean build.

- [ ] **Step 4: Run full suite**

Run: `make build && make test`
Expected: PASS across the board.

- [ ] **Step 5: Commit**

```bash
git add cmd/workerd/main.go
git commit -m "feat(workerd): wire GuardedFileOps/Git/Worktree in main

Construct all four guards at the main() boundary, the only place
raw adapters exist. GuardedFileOps/Git/Worktree are intentionally
held as variables but not yet handed to services — the service-
side rewrite is a follow-up task per plan §11. This commit
guarantees the construction wiring compiles and binds correctly.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: Verify `confirm_token` round-trip through httpapi/mcp

**Files:**
- Read-only: `internal/httpapi/router.go`
- Read-only: `internal/mcp/tools.go`
- Modify (if and only if a regression appears): the same files

- [ ] **Step 1: Inspect the delivery layer**

Run: `grep -nE "PendingID|pending_id|Confirmation" internal/httpapi/*.go internal/mcp/*.go`
Note every place a pending_id is read from the request DTO or written to a response.

- [ ] **Step 2: Trace the current command flow**

The `command.RunCommandRequest.PendingID` field already exists and is threaded into `Service.Run`. After the Task 1 refactor, that field is mapped into `CallOpts{ConfirmToken: req.PendingID}` inside `Service.Run`. No delivery-layer change is required *unless* the existing httpapi/mcp tests have started failing.

- [ ] **Step 3: Run the delivery-layer tests**

Run: `go test ./internal/httpapi/... ./internal/mcp/...`
Expected: PASS without modification.

- [ ] **Step 4: If a test fails, fix it**

If any test fails, the most likely cause is a stale assertion against the old `RunConfirmed` code path. Update the test to assert the unified `Run` path (which now handles both first-call and confirmed-retry via `PendingID`). Do not add new endpoint surface.

- [ ] **Step 5: Run the full suite once more**

Run: `make build && make test`
Expected: PASS.

- [ ] **Step 6: Run the policy demo as a sanity check**

Run: `go run ./cmd/policy-demo`
Expected: the same verdict table the spec promised (allow `go test`, ask `git push`, deny `git push --force`, deny `cat .env`, etc.) — unchanged.

- [ ] **Step 7: Commit (only if files changed)**

If httpapi/mcp files were modified:

```bash
git add internal/httpapi internal/mcp
git commit -m "feat(mcp,httpapi): confirm_token round-trip through unified Run

Adjusts delivery-layer tests for the GuardedRunner.Run + CallOpts
refactor. No new routes or tools; the existing pending_id field
on RunCommandRequest is now mapped to CallOpts.ConfirmToken inside
the command service.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

If nothing changed, skip the commit — Task 6 was just verification.

---

## Final: Open PR to `main`

- [ ] **Step 1: Confirm clean state**

Run: `git status --short && git log --oneline main..HEAD`
Expected: working tree clean; 5–6 commits ahead of `main` (Tasks 1–4 always commit; Task 5 always commits; Task 6 sometimes).

- [ ] **Step 2: Push branch**

Confirm with the user first; pushing is a remote-visible action.

```bash
git push -u origin ptolemy/guarded-adapters
```

- [ ] **Step 3: Open the PR via the GitHub plugin tooling**

Per AGENTS.md §GitHub Tooling Policy: use the GitHub plugin, **not** `gh`. PR title:

> `feat(policy): GuardedFileOps, GuardedGit, GuardedWorktree`

PR body (fill the template at `.github/pull_request_template.md`):

```markdown
## Summary
- Adds GuardedFileOps, GuardedGit, GuardedWorktree to internal/policy, each routing through a shared guardCore.gate() pipeline.
- Refactors GuardedRunner onto the same CallOpts API; pendingID is now the SHA-256 intent hash (defeats swap-intent).
- Wires all four guards in cmd/workerd/main.go.

## Spec
docs/superpowers/specs/2026-05-26-guarded-adapters-design.md

## Test plan
- [ ] `make build` clean
- [ ] `make test` green
- [ ] `go run ./cmd/policy-demo` verdict table unchanged
- [ ] New tests cover: deny-policy-write via WriteFile, deny-secret-cmd via ReadFile, ask-then-confirm on Push and List, swap-intent rejection
```

---

## Self-Review

**Spec coverage:**
- Spec §5.1 `CallOpts` + `gate()` → Task 1.
- Spec §5.2 three guard structs → Tasks 2–4.
- Spec §5.3 `GuardedRunner` refactor → Task 1.
- Spec §6.1–6.3 intent taxonomy → Tasks 2–4 (each method's `Intent` matches the table).
- Spec §7 rule-coverage assertions → covered by tests in Tasks 2 (deny-policy-write, deny-secret-cmd) and 3 (ask-push-cmd, default-ask Status).
- Spec §8 file list → matches File Structure table above.
- Spec §9 test plan → each test from §9 maps to a step in Tasks 1–4. `TestGate_SwapIntentRejected` lands in Task 2 (it uses `GuardedFileOps` but exercises the shared `gate()`).
- Spec §10 commit phasing → Tasks 1–6 commit messages match the six phases.
- Spec §11 open follow-ups → explicitly out of scope; not in any task.

**Placeholder scan:** No "TBD", no "implement later", every code step shows the actual code. The one conditional step (Task 6 Step 4) explains exactly what to fix if a test fails.

**Type consistency:** `CallOpts` used in every guard. `guardCore.gate(ctx, sessionID, intent, opts)` signature used identically across Tasks 1–4. Each guard's `core` field is the unexported `guardCore`; tests reach into `g.core.approvals` consistently. `pendingID == intentHash` is enforced inside `gate()` exactly once.

No gaps; no fixups needed.

---

**Plan complete and saved to `docs/superpowers/plans/2026-05-26-guarded-adapters.md`. Two execution options:**

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration.

**2. Inline Execution** — I execute tasks in this session using executing-plans, batch with checkpoints.

**Which approach?**
