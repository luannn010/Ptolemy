// Brain control plane: a LOOPBACK-ONLY HTTP surface for the brain controller
// (models/status/load/resume/hibernate/stop). It is never exposed on a public
// interface — it can stop GPU processes — and every action goes through
// policy.GuardedBrain, so denies/asks/audit all apply. Auto-wake (the /chat hook)
// lives in rag.go.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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
	Load(ctx context.Context, sessionID string, spec brain.Spec, opts policy.CallOpts) error
	Resume(ctx context.Context, sessionID string, opts policy.CallOpts) error
	Hibernate(ctx context.Context, sessionID string, opts policy.CallOpts) error
	Stop(ctx context.Context, sessionID string, opts policy.CallOpts) error
	Status(ctx context.Context, sessionID string, opts policy.CallOpts) (brain.Status, error)
	ListModels(ctx context.Context, sessionID string, opts policy.CallOpts) ([]brain.DiscoveredModel, error)
}

type BrainDeps struct {
	Brain BrainController
}

// NewBrainControlRouter serves loopback-only GET /brain/{models,status} and
// POST /brain/{load,resume,hibernate,stop}.
func NewBrainControlRouter(deps BrainDeps) http.Handler {
	r := chi.NewRouter()
	r.Use(loopbackOnly)

	r.Get("/brain/models", func(w http.ResponseWriter, req *http.Request) {
		models, err := deps.Brain.ListModels(req.Context(), BrainSystemSession, policy.CallOpts{})
		if writeBrainErr(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"models": models})
	})

	r.Get("/brain/status", func(w http.ResponseWriter, req *http.Request) {
		st, err := deps.Brain.Status(req.Context(), BrainSystemSession, policy.CallOpts{})
		if writeBrainErr(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, st)
	})

	r.Post("/brain/load", func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Binary       string   `json:"binary"`
			GGUF         string   `json:"gguf"`
			Host         string   `json:"host"`
			Port         string   `json:"port"`
			Args         []string `json:"args"`
			ConfirmToken string   `json:"confirm_token"`
		}
		if !decodeOptionalJSON(w, req, &body) {
			return
		}
		if strings.TrimSpace(body.GGUF) == "" {
			writeJSON(w, http.StatusBadRequest, apitypes.ErrorResponse{Error: "gguf is required"})
			return
		}
		spec := brain.Spec{Binary: body.Binary, GGUF: body.GGUF, Host: body.Host, Port: body.Port, Args: body.Args}
		err := deps.Brain.Load(req.Context(), BrainSystemSession, spec, policy.CallOpts{ConfirmToken: body.ConfirmToken})
		if writeBrainErr(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Post("/brain/resume", func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			ConfirmToken string `json:"confirm_token"`
		}
		if !decodeOptionalJSON(w, req, &body) {
			return
		}
		err := deps.Brain.Resume(req.Context(), BrainSystemSession, policy.CallOpts{ConfirmToken: body.ConfirmToken})
		if writeBrainErr(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Post("/brain/hibernate", func(w http.ResponseWriter, req *http.Request) {
		err := deps.Brain.Hibernate(req.Context(), BrainSystemSession, policy.CallOpts{})
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
// zero struct). An empty body decodes to io.EOF — this covers both
// Content-Length: 0 and an empty chunked body (where ContentLength is -1), so we
// must key off EOF rather than ContentLength. Malformed JSON writes 400.
func decodeOptionalJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	err := json.NewDecoder(r.Body).Decode(target)
	if errors.Is(err, io.EOF) {
		return true // no body — leave target as the zero struct
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apitypes.ErrorResponse{Error: "invalid json"})
		return false
	}
	return true
}

// writeBrainErr maps guard/manager errors: deny->403, needs-confirmation->202,
// no-model->409, other (process/exec failure)->502. Returns true when it wrote a
// response.
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
	if errors.Is(err, brain.ErrNoModelLoaded) {
		writeJSON(w, http.StatusConflict, apitypes.ErrorResponse{Error: err.Error()})
		return true
	}
	writeJSON(w, http.StatusBadGateway, apitypes.ErrorResponse{Error: err.Error()})
	return true
}
