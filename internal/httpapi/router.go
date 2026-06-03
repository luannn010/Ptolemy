package httpapi

import (
	"encoding/json"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/luannn010/ptolemy/internal/apitypes"
	"github.com/luannn010/ptolemy/internal/command"
	"github.com/luannn010/ptolemy/internal/health"
	"github.com/luannn010/ptolemy/internal/policy"
	"github.com/luannn010/ptolemy/internal/session"
)

type RouterDeps struct {
	Sessions  *session.Store
	Commands  *command.Service
	CommandDB *command.Store
	Health    *health.Aggregator
}

func NewRouter(deps RouterDeps) http.Handler {
	r := chi.NewRouter()
	r.Get("/health", func(w http.ResponseWriter, req *http.Request) {
		if deps.Health == nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"status":    "ok",
				"service":   "workerd",
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			})
			return
		}
		report, code := deps.Health.Run(req.Context())
		writeJSON(w, code, report)
	})

	r.Post("/sessions", func(w http.ResponseWriter, r *http.Request) {
		var req apitypes.CreateSessionRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		created, err := deps.Sessions.Create(r.Context(), session.CreateSessionRequest{
			Name:        req.Name,
			Workspace:   req.Workspace,
			Description: req.Description,
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, apitypes.ErrorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, mapSession(created))
	})

	r.Get("/sessions", func(w http.ResponseWriter, r *http.Request) {
		items, err := deps.Sessions.List(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, apitypes.ErrorResponse{Error: err.Error()})
			return
		}
		resp := apitypes.SessionListResponse{Sessions: make([]apitypes.SessionResponse, 0, len(items))}
		for _, item := range items {
			resp.Sessions = append(resp.Sessions, mapSession(item))
		}
		writeJSON(w, http.StatusOK, resp)
	})

	r.Get("/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		sess, err := deps.Sessions.Get(r.Context(), chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusNotFound, apitypes.ErrorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, mapSession(sess))
	})

	r.Post("/sessions/{id}/close", func(w http.ResponseWriter, r *http.Request) {
		sess, err := deps.Sessions.CloseSession(r.Context(), chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusNotFound, apitypes.ErrorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, mapSession(sess))
	})

	r.Post("/sessions/{id}/commands", func(w http.ResponseWriter, r *http.Request) {
		var req apitypes.RunCommandRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		result, err := deps.Commands.Run(r.Context(), chi.URLParam(r, "id"), command.RunCommandRequest{
			Command: req.Command,
			CWD:     req.CWD,
			Timeout: req.Timeout,
			Target:  "",
			Title:   "",
			Purpose: "",
		})
		if err != nil {
			var denied policy.ErrDenied
			if ok := asDenied(err, &denied); ok {
				writeJSON(w, http.StatusForbidden, apitypes.ErrorResponse{Error: denied.Reason, RuleID: denied.RuleID})
				return
			}
			writeJSON(w, http.StatusInternalServerError, apitypes.ErrorResponse{Error: err.Error()})
			return
		}
		if result.Confirmation != nil {
			writeJSON(w, http.StatusAccepted, apitypes.NeedsConfirmation{
				Status:    "needs_confirmation",
				Channel:   result.Confirmation.Channel,
				PendingID: result.Confirmation.PendingID,
				Reason:    result.Confirmation.Reason,
			})
			return
		}
		writeJSON(w, http.StatusOK, result.Log)
	})

	r.Get("/sessions/{id}/commands", func(w http.ResponseWriter, r *http.Request) {
		logs, err := deps.CommandDB.ListBySession(r.Context(), chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, apitypes.ErrorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, logs)
	})

	r.Post("/execute", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			SessionID    string `json:"session_id"`
			Command      string `json:"command"`
			CWD          string `json:"cwd"`
			Timeout      int    `json:"timeout"`
			ConfirmToken string `json:"confirm_token,omitempty"`
			PendingID    string `json:"pending_id,omitempty"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		req := command.RunCommandRequest{Command: body.Command, CWD: body.CWD, Timeout: body.Timeout, Target: "", Title: "", Purpose: ""}
		if body.ConfirmToken != "" {
			req.PendingID = body.ConfirmToken
		}
		if body.PendingID != "" {
			req.PendingID = body.PendingID
		}
		result, err := deps.Commands.Run(r.Context(), body.SessionID, req)
		if err != nil {
			var denied policy.ErrDenied
			if ok := asDenied(err, &denied); ok {
				writeJSON(w, http.StatusForbidden, apitypes.ErrorResponse{Error: denied.Reason, RuleID: denied.RuleID})
				return
			}
			writeJSON(w, http.StatusInternalServerError, apitypes.ErrorResponse{Error: err.Error()})
			return
		}
		if result.Confirmation != nil {
			writeJSON(w, http.StatusAccepted, apitypes.NeedsConfirmation{
				Status:    "needs_confirmation",
				Channel:   result.Confirmation.Channel,
				PendingID: result.Confirmation.PendingID,
				Reason:    result.Confirmation.Reason,
			})
			return
		}
		writeJSON(w, http.StatusOK, result.Log)
	})

	return r
}

func NewApproveRouter(approvals *policy.Approvals) http.Handler {
	r := chi.NewRouter()
	r.Post("/approve/{id}", func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			writeJSON(w, http.StatusForbidden, apitypes.ErrorResponse{Error: "loopback only"})
			return
		}
		if !approvals.Approve(chi.URLParam(r, "id")) {
			writeJSON(w, http.StatusNotFound, apitypes.ErrorResponse{Error: "pending approval not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "approved"})
	})
	return r
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		writeJSON(w, http.StatusBadRequest, apitypes.ErrorResponse{Error: "invalid json"})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func mapSession(s session.Session) apitypes.SessionResponse {
	return apitypes.SessionResponse{
		ID:          s.ID,
		Name:        s.Name,
		Status:      string(s.Status),
		Workspace:   s.Workspace,
		Description: s.Description,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
		ClosedAt:    s.ClosedAt,
	}
}

func asDenied(err error, denied *policy.ErrDenied) bool {
	if err == nil {
		return false
	}
	v, ok := err.(policy.ErrDenied)
	if !ok {
		return false
	}
	*denied = v
	return true
}
