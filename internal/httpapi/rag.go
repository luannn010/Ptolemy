// RAG HTTP listener: exposes the memory module's agentic RAG (Orchestrator.Answer)
// to local sub-services as plain HTTP on its own port (RAG_PORT, all interfaces).
//
// One endpoint, POST /chat: question in, grounded answer + citations out, with an
// optional step-by-step retrieval/reasoning trace (trace:true). gave_up:true is a
// successful 200 — an honest "not in the KB" per memory.Answer semantics. Memory
// stays in its in-process carve-out (read-mostly; its only writes are reinforce
// counters in the memory Postgres DB), so no Guarded* wrapper is involved.
package httpapi

import (
	"context"
	"net/http"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"

	"github.com/luannn010/ptolemy/internal/apitypes"
	"github.com/luannn010/ptolemy/internal/memory"
)

// Answerer is the slice of *memory.Orchestrator the RAG endpoint needs.
type Answerer interface {
	Answer(ctx context.Context, q memory.Query) (memory.Answer, error)
}

// Waker, when set, is invoked before each /chat answer to ensure the brain is
// loaded (the auto-wake / JIT-load path). nil disables auto-wake.
type Waker interface {
	EnsureAwake(ctx context.Context) error
}

// RAGDeps wires the /chat handler. DefaultSubject/DefaultProject scope queries
// that omit subject_id/project_id (sourced from PTOLEMY_MEMORY_* in cmd/workerd).
type RAGDeps struct {
	Answerer       Answerer
	Waker          Waker // optional brain auto-wake; nil = disabled
	DefaultSubject string
	DefaultProject string
}

// NewSerialAnswerer serializes Answer calls. memory.NewModule hands back a single
// *pgx.Conn, which is not safe for concurrent use; the stdio MCP server is
// naturally serial but an HTTP listener is not, so the mutex is load-bearing.
func NewSerialAnswerer(a Answerer) Answerer {
	return &serialAnswerer{inner: a}
}

type serialAnswerer struct {
	mu    sync.Mutex
	inner Answerer
}

func (s *serialAnswerer) Answer(ctx context.Context, q memory.Query) (memory.Answer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.Answer(ctx, q)
}

// maxChatBodyBytes caps the /chat request body. The listener binds all
// interfaces, so this bounds the memory an unauthenticated caller can force the
// JSON decoder to allocate. A query payload is tiny; 1 MiB is generous.
const maxChatBodyBytes = 1 << 20

// NewRAGRouter serves POST /chat and a static GET /health liveness probe.
func NewRAGRouter(deps RAGDeps) http.Handler {
	r := chi.NewRouter()

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "workerd-rag"})
	})

	r.Post("/chat", func(w http.ResponseWriter, req *http.Request) {
		// Bound the request body before decoding (all-interfaces listener).
		req.Body = http.MaxBytesReader(w, req.Body, maxChatBodyBytes)
		var body apitypes.ChatRequest
		if !decodeJSON(w, req, &body) {
			return
		}
		text := strings.TrimSpace(body.Query)
		if text == "" {
			writeJSON(w, http.StatusBadRequest, apitypes.ErrorResponse{Error: "query is required"})
			return
		}

		subject := strings.TrimSpace(body.SubjectID)
		if subject == "" {
			subject = deps.DefaultSubject
		}
		project := strings.TrimSpace(body.ProjectID)
		if project == "" {
			project = deps.DefaultProject
		}
		q := memory.Query{Text: text, K: body.K, Trace: body.Trace}
		if subject != "" {
			q.SubjectID = &subject
		}
		if project != "" {
			q.ProjectID = &project
		}

		// Auto-wake the brain (JIT load) before answering. A wake failure means
		// the brain is unavailable — same 502 class as an answer failure.
		if deps.Waker != nil {
			if err := deps.Waker.EnsureAwake(req.Context()); err != nil {
				writeJSON(w, http.StatusBadGateway, apitypes.ErrorResponse{Error: err.Error()})
				return
			}
		}

		ans, err := deps.Answerer.Answer(req.Context(), q)
		if err != nil {
			// The handler has no failure modes of its own: an Answer error means an
			// upstream dependency (brain LLM, embedder, memory Postgres) failed.
			writeJSON(w, http.StatusBadGateway, apitypes.ErrorResponse{Error: err.Error()})
			return
		}

		resp := apitypes.ChatResponse{
			Answer:    ans.Text,
			Citations: ans.Citations,
			GaveUp:    ans.GaveUp,
		}
		if resp.Citations == nil {
			resp.Citations = []string{}
		}
		if ans.Trace != nil {
			resp.Mode = ans.Trace.Mode
			resp.Steps = ans.Trace.Steps
		}
		writeJSON(w, http.StatusOK, resp)
	})

	return r
}
