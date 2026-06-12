// Brain control plane: a LOOPBACK-ONLY HTTP surface for the brain lifecycle skill
// (start/stop/switch/status). It is never exposed on a public interface — it can
// stop GPU processes — and every action goes through policy.GuardedBrain, so
// denies/asks/audit all apply. Auto-wake (the /chat hook) lives in rag.go.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/luannn010/ptolemy/internal/apitypes"
	"github.com/luannn010/ptolemy/internal/brain"
	"github.com/luannn010/ptolemy/internal/policy"
)

// BrainSystemSession is the reserved session id brain decisions are audited
// under (brain ops aren't tied to a command session). cmd/workerd ensures the
// row exists so the policy_decisions FK holds.
const BrainSystemSession = "brain-system"

// BrainController is the slice of policy.GuardedBrain the control plane needs.
type BrainController interface {
	Wake(ctx context.Context, sessionID, model string, opts policy.CallOpts) error
	Stop(ctx context.Context, sessionID string, opts policy.CallOpts) error
	Switch(ctx context.Context, sessionID, model string, opts policy.CallOpts) error
	Status(ctx context.Context, sessionID string, opts policy.CallOpts) (brain.Status, error)
}

type BrainDeps struct {
	Brain BrainController
}

// NewBrainControlRouter serves loopback-only POST /brain/{wake,stop,switch} and
// GET /brain/status.
func NewBrainControlRouter(deps BrainDeps) http.Handler {
	r := chi.NewRouter()
	r.Use(loopbackOnly)

	r.Get("/brain/status", func(w http.ResponseWriter, req *http.Request) {
		st, err := deps.Brain.Status(req.Context(), BrainSystemSession, policy.CallOpts{})
		if writeBrainErr(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, st)
	})

	r.Post("/brain/wake", func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Model        string `json:"model"`
			ConfirmToken string `json:"confirm_token"`
		}
		if !decodeOptionalJSON(w, req, &body) {
			return
		}
		err := deps.Brain.Wake(req.Context(), BrainSystemSession, strings.TrimSpace(body.Model),
			policy.CallOpts{ConfirmToken: body.ConfirmToken})
		if writeBrainErr(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Post("/brain/switch", func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Model        string `json:"model"`
			ConfirmToken string `json:"confirm_token"`
		}
		if !decodeOptionalJSON(w, req, &body) {
			return
		}
		if strings.TrimSpace(body.Model) == "" {
			writeJSON(w, http.StatusBadRequest, apitypes.ErrorResponse{Error: "model is required"})
			return
		}
		err := deps.Brain.Switch(req.Context(), BrainSystemSession, strings.TrimSpace(body.Model),
			policy.CallOpts{ConfirmToken: body.ConfirmToken})
		if writeBrainErr(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Post("/brain/stop", func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			ConfirmToken string `json:"confirm_token"`
		}
		if !decodeOptionalJSON(w, req, &body) {
			return
		}
		err := deps.Brain.Stop(req.Context(), BrainSystemSession, policy.CallOpts{ConfirmToken: body.ConfirmToken})
		if writeBrainErr(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	return r
}

// loopbackOnly rejects any request not originating from the loopback interface.
func loopbackOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			writeJSON(w, http.StatusForbidden, apitypes.ErrorResponse{Error: "loopback only"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// decodeOptionalJSON decodes the body if present; an empty body is allowed (the
// zero struct). Malformed JSON writes 400 and returns false.
func decodeOptionalJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	if r.ContentLength == 0 {
		return true
	}
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		writeJSON(w, http.StatusBadRequest, apitypes.ErrorResponse{Error: "invalid json"})
		return false
	}
	return true
}

// writeBrainErr maps guard errors: deny->403, needs-confirmation->202, other
// (process/exec failure)->502. Returns true when it wrote a response.
func writeBrainErr(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	var denied policy.ErrDenied
	if errors.As(err, &denied) {
		writeJSON(w, http.StatusForbidden, apitypes.ErrorResponse{Error: denied.Reason, RuleID: denied.RuleID})
		return true
	}
	var needs policy.ErrNeedsConfirmation
	if errors.As(err, &needs) {
		writeJSON(w, http.StatusAccepted, apitypes.NeedsConfirmation{
			Status:    "needs_confirmation",
			Channel:   needs.Channel,
			PendingID: needs.PendingID,
			Reason:    needs.Reason,
		})
		return true
	}
	writeJSON(w, http.StatusBadGateway, apitypes.ErrorResponse{Error: err.Error()})
	return true
}
