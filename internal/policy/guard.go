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

	"github.com/google/uuid"
	"github.com/luannn010/ptolemy/internal/domain"
	"github.com/luannn010/ptolemy/internal/terminal"
)

type RawRunner interface {
	Run(ctx context.Context, command string, cwd string, timeoutSeconds int) terminal.Result
}

type ErrNeedsConfirmation struct {
	PendingID string
	Channel   domain.Channel
	Reason    string
}

func (e ErrNeedsConfirmation) Error() string {
	return "needs confirmation"
}

type ErrDenied struct {
	RuleID string
	Reason string
}

func (e ErrDenied) Error() string {
	return "denied: " + e.Reason
}

type GuardedRunner struct {
	engine    *Engine
	approvals *Approvals
	raw       RawRunner
	db        *sql.DB
}

func NewGuardedRunner(engine *Engine, approvals *Approvals, raw RawRunner, db *sql.DB) *GuardedRunner {
	return &GuardedRunner{engine: engine, approvals: approvals, raw: raw, db: db}
}

func (g *GuardedRunner) Run(ctx context.Context, sessionID string, command string, cwd string, timeoutSeconds int) (terminal.Result, error) {
	intent := domain.Intent{Kind: "command.exec", Program: shellProgram(), Args: []string{command}, Targets: []string{cwd}}
	decision := g.engine.Authorize(intent)
	intentHash := hashIntent(intent)
	pendingID := uuid.NewString()

	if err := g.recordDecision(ctx, pendingID, sessionID, intent, decision, intentHash, false); err != nil {
		return terminal.Result{ExitCode: 1, ErrorOutput: err.Error()}, err
	}

	switch decision.Effect {
	case domain.EffectAllow:
		return g.raw.Run(ctx, command, cwd, timeoutSeconds), nil
	case domain.EffectAsk:
		g.approvals.Park(pendingID)
		return terminal.Result{}, ErrNeedsConfirmation{
			PendingID: pendingID,
			Channel:   decision.Channel,
			Reason:    decision.Reason,
		}
	default:
		return terminal.Result{}, ErrDenied{RuleID: decision.RuleID, Reason: decision.Reason}
	}
}

func (g *GuardedRunner) RunConfirmed(ctx context.Context, pendingID string, sessionID string, command string, cwd string, timeoutSeconds int) (terminal.Result, error) {
	if !g.approvals.ConsumeApproved(pendingID) {
		return terminal.Result{}, errors.New("approval not granted")
	}
	if _, err := g.db.ExecContext(ctx, `UPDATE policy_decisions SET confirmed = 1 WHERE id = ?`, pendingID); err != nil {
		return terminal.Result{}, err
	}
	return g.raw.Run(ctx, command, cwd, timeoutSeconds), nil
}

func (g *GuardedRunner) recordDecision(ctx context.Context, id string, sessionID string, intent domain.Intent, dec domain.Decision, intentHash string, confirmed bool) error {
	_, err := g.db.ExecContext(
		ctx,
		`INSERT INTO policy_decisions
		(id, session_id, intent_kind, program, target, effect, channel, rule_id, reason, intent_hash, confirmed, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id,
		sessionID,
		intent.Kind,
		intent.Program,
		first(intent.Targets),
		string(dec.Effect),
		string(dec.Channel),
		dec.RuleID,
		dec.Reason,
		intentHash,
		boolToInt(confirmed),
		time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

func hashIntent(intent domain.Intent) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%s", intent.Kind, intent.Program, strings.Join(intent.Args, "|"), strings.Join(intent.Targets, "|"))))
	return hex.EncodeToString(h[:])
}

func first(items []string) string {
	if len(items) == 0 {
		return ""
	}
	return items[0]
}

func shellProgram() string {
	return "shell"
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
