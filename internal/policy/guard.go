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
