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
// when the caller should proceed with the raw adapter; ErrDenied or
// ErrNeedsConfirmation otherwise. pendingID == intentHash by design — the
// token IS the hash, so a swap-intent attack (approve A, retry with B) fails
// the equality check inside the confirmed-retry path.
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

// ---------- GuardedRunner ----------

type RawRunner interface {
	Run(ctx context.Context, command string, cwd string, timeoutSeconds int) terminal.Result
}

type GuardedRunner struct {
	core      guardCore
	raw       RawRunner
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

func (g *GuardedFileOps) fileIntent(kind, program, path string, extraArgs ...string) domain.Intent {
	resolved, err := g.raw.Resolve(path)
	if err != nil {
		resolved = path
	}
	return domain.Intent{Kind: kind, Program: program, Args: extraArgs, Targets: []string{resolved}}
}

func (g *GuardedFileOps) Resolve(ctx context.Context, sessionID, path string, opts CallOpts) (string, error) {
	if err := g.core.gate(ctx, sessionID, g.fileIntent("file.resolve", "resolve", path), opts); err != nil {
		return "", err
	}
	return g.raw.Resolve(path)
}

func (g *GuardedFileOps) ReadFile(ctx context.Context, sessionID, path string, opts CallOpts) (string, error) {
	if err := g.core.gate(ctx, sessionID, g.fileIntent("file.read", "read", path), opts); err != nil {
		return "", err
	}
	return g.raw.ReadFile(path)
}

func (g *GuardedFileOps) WriteFile(ctx context.Context, sessionID, path, content string, opts CallOpts) error {
	if err := g.core.gate(ctx, sessionID, g.fileIntent("file.write", "write", path), opts); err != nil {
		return err
	}
	return g.raw.WriteFile(path, content)
}

func (g *GuardedFileOps) ListDirectory(ctx context.Context, sessionID, path string, opts CallOpts) ([]fileops.DirEntry, error) {
	if err := g.core.gate(ctx, sessionID, g.fileIntent("file.list", "list", path), opts); err != nil {
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
	if err := g.core.gate(ctx, sessionID, g.fileIntent("file.patch", "patch", path), opts); err != nil {
		return err
	}
	return g.raw.ApplyPatch(path, newContent)
}

func (g *GuardedFileOps) ReplaceBlock(ctx context.Context, sessionID, path, oldS, newS string, opts CallOpts) error {
	if err := g.core.gate(ctx, sessionID, g.fileIntent("file.replace", "replace", path), opts); err != nil {
		return err
	}
	return g.raw.ReplaceBlock(path, oldS, newS)
}

func (g *GuardedFileOps) InsertAfter(ctx context.Context, sessionID, path, marker, content string, opts CallOpts) error {
	if err := g.core.gate(ctx, sessionID, g.fileIntent("file.insert", "insert", path, marker), opts); err != nil {
		return err
	}
	return g.raw.InsertAfter(path, marker, content)
}

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
