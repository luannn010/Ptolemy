// Package memorytools exposes the conversational memory/RAG system
// (internal/memory) as Ptolemy MCP tools: recall, capture, and consolidate.
//
// Unlike the executor/session/file tool groups, these handlers do NOT proxy to
// workerd over HTTP. They hold the memory Orchestrator/capture-hook/consolidator
// directly (injected via Deps) and call them in-process. This mirrors the
// navigator carve-out documented in CLAUDE.md: memory is read-mostly knowledge
// access whose only writes land in the memory Postgres DB — never the guarded
// workspace, shell, or git — so it is exempt from the Guarded* harness. The
// *WorkerClient argument required by mcp.ToolHandler is intentionally ignored.
package memorytools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/luannn010/ptolemy/internal/mcp"
	"github.com/luannn010/ptolemy/internal/memory"
)

// Recaller is the recall slice of *memory.Orchestrator. Recall is the fast
// retrieval-only path (no LLM); Answer adds an LLM-synthesized prose response.
type Recaller interface {
	Answer(ctx context.Context, q memory.Query) (memory.Answer, error)
	Recall(ctx context.Context, q memory.Query) (memory.RecallResult, error)
}

// Capturer is the sync-capture slice of *memory.PerTurnCaptureHook.
type Capturer interface {
	Capture(ctx context.Context, ex memory.Exchange) error
}

// Consolidator is the per-subject/project slice of *memory.Consolidator.
type Consolidator interface {
	ConsolidateSubjectProject(ctx context.Context, subject, project string) error
}

// Deps carries the memory dependencies plus the default subject/project used
// when a tool call omits them (sourced from env in cmd/ptolemy-mcp).
type Deps struct {
	Recall      Recaller
	Capture     Capturer
	Consolidate Consolidator
	Subject     string
	Project     string
}

func Tools() []mcp.Tool {
	return []mcp.Tool{
		mcp.NewTool("ptolemy_memory_recall",
			"Recall durable project context: returns a compact synthesis summary plus supporting atoms and citations. Fast retrieval-only by default (no LLM); set generate=true for a synthesized prose answer. Prefer this over asking the user to paste context.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":      map[string]any{"type": "string", "description": "What to recall."},
					"k":          map[string]any{"type": "integer", "description": "Max results (default from config)."},
					"subject_id": map[string]any{"type": "string", "description": "Owner scope; defaults to the configured subject."},
					"project_id": map[string]any{"type": "string", "description": "Project scope; defaults to the configured project."},
					"generate":   map[string]any{"type": "boolean", "description": "Synthesize a prose answer via the LLM (slower). Default false returns retrieval-only context."},
				},
				"required": []string{"query"},
			}),
		mcp.NewTool("ptolemy_memory_capture",
			"Capture a user/assistant exchange as durable memory atoms for later recall. Provide at least one of user_text or assistant_text.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"user_text":      map[string]any{"type": "string"},
					"assistant_text": map[string]any{"type": "string"},
					"subject_id":     map[string]any{"type": "string", "description": "Owner scope; defaults to the configured subject."},
					"session_id":     map[string]any{"type": "string"},
					"project_id":     map[string]any{"type": "string", "description": "Project scope; defaults to the configured project."},
				},
			}),
		mcp.NewTool("ptolemy_memory_consolidate",
			"Consolidate a subject/project's atoms into a single reconciled synthesis summary. Run periodically or at task end.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"subject_id": map[string]any{"type": "string", "description": "Owner scope; defaults to the configured subject."},
					"project_id": map[string]any{"type": "string", "description": "Project scope; defaults to the configured project."},
				},
			}),
	}
}

// NewHandler returns an mcp.ToolHandler bound to the given memory deps. The
// *WorkerClient argument is ignored — memory is called in-process.
func NewHandler(deps Deps) mcp.ToolHandler {
	return func(name string, args map[string]any, _ *mcp.WorkerClient) (map[string]any, bool, error) {
		switch name {
		case "ptolemy_memory_recall":
			return deps.handleRecall(args)
		case "ptolemy_memory_capture":
			return deps.handleCapture(args)
		case "ptolemy_memory_consolidate":
			return deps.handleConsolidate(args)
		}
		return nil, false, nil
	}
}

func (d Deps) handleRecall(args map[string]any) (map[string]any, bool, error) {
	query := strings.TrimSpace(argStr(args, "query"))
	if query == "" {
		return nil, true, errors.New("ptolemy_memory_recall: query is required")
	}
	subject := d.resolve(args, "subject_id", d.Subject)
	project := d.resolve(args, "project_id", d.Project)
	q := memory.Query{Text: query}
	if k, ok := args["k"].(float64); ok {
		q.K = int(k)
	}
	if subject != "" {
		q.SubjectID = &subject
	}
	if project != "" {
		q.ProjectID = &project
	}
	if gen, _ := args["generate"].(bool); gen {
		ans, err := d.Recall.Answer(context.Background(), q)
		if err != nil {
			return nil, true, err
		}
		return jsonResult(map[string]any{"text": ans.Text, "citations": ans.Citations}), true, nil
	}
	res, err := d.Recall.Recall(context.Background(), q)
	if err != nil {
		return nil, true, err
	}
	return jsonResult(map[string]any{"text": res.Context, "citations": res.SourceIDs}), true, nil
}

func (d Deps) handleCapture(args map[string]any) (map[string]any, bool, error) {
	userText := argStr(args, "user_text")
	assistantText := argStr(args, "assistant_text")
	if strings.TrimSpace(userText) == "" && strings.TrimSpace(assistantText) == "" {
		return nil, true, errors.New("ptolemy_memory_capture: provide at least one of user_text or assistant_text")
	}
	ex := memory.Exchange{
		UserText:      userText,
		AssistantText: assistantText,
		SubjectID:     d.resolve(args, "subject_id", d.Subject),
		SessionID:     argStr(args, "session_id"),
		ProjectID:     d.resolve(args, "project_id", d.Project),
	}
	if err := d.Capture.Capture(context.Background(), ex); err != nil {
		return nil, true, err
	}
	return jsonResult(map[string]any{"captured": true}), true, nil
}

func (d Deps) handleConsolidate(args map[string]any) (map[string]any, bool, error) {
	subject := d.resolve(args, "subject_id", d.Subject)
	project := d.resolve(args, "project_id", d.Project)
	if err := d.Consolidate.ConsolidateSubjectProject(context.Background(), subject, project); err != nil {
		return nil, true, err
	}
	return jsonResult(map[string]any{"ok": true}), true, nil
}

// resolve returns the arg value if present and non-empty, else the fallback.
func (d Deps) resolve(args map[string]any, key, fallback string) string {
	if v := strings.TrimSpace(argStr(args, key)); v != "" {
		return v
	}
	return fallback
}

func argStr(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

func jsonResult(payload map[string]any) map[string]any {
	body, _ := json.Marshal(payload)
	return mcp.TextResult(body)
}
