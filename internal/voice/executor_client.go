package voice

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/luannn010/ptolemy/internal/executor"
	"github.com/luannn010/ptolemy/internal/mcp"
	"github.com/luannn010/ptolemy/internal/session"
)

// ExecResult is the trimmed-down result the voice loop needs to report back to
// the user after running a spoken command on the executor. It also carries the
// request payload and round-trip timing so the observation block can show them.
type ExecResult struct {
	ExitCode       int
	Summary        string
	Success        bool
	RequestPayload string // the JSON body sent to /execute
	DurationMS     int64  // round-trip time for the /execute call
}

// ExecutorClient sends spoken shell commands to a Ptolemy worker's executor.
// It is an interface so the voice loop can be unit-tested without a live server.
type ExecutorClient interface {
	// OpenSession opens a worker session and returns its ID. The voice loop
	// opens one session lazily and reuses it for subsequent commands.
	OpenSession(ctx context.Context) (sessionID string, err error)
	// Execute runs command in the given open session via POST /execute.
	Execute(ctx context.Context, sessionID, command string) (ExecResult, error)
}

// httpExecutorClient talks to a workerd HTTP server (default
// http://127.0.0.1:8080) via the shared mcp.WorkerClient.
type httpExecutorClient struct {
	worker *mcp.WorkerClient
}

// NewHTTPExecutorClient builds an ExecutorClient pointed at baseURL, e.g. the
// value of WORKER_BASE_URL.
func NewHTTPExecutorClient(baseURL string) ExecutorClient {
	return &httpExecutorClient{worker: mcp.NewWorkerClient(baseURL)}
}

func (c *httpExecutorClient) OpenSession(ctx context.Context) (string, error) {
	_ = ctx // mcp.WorkerClient uses the default HTTP client; ctx reserved for future use.
	body, err := c.worker.Post("/sessions", session.CreateSessionRequest{
		Name:        "voice-session",
		Workspace:   ".",
		Description: "voice commands",
	})
	if err != nil {
		return "", fmt.Errorf("open session: %w", err)
	}
	var sess session.Session
	if err := json.Unmarshal(body, &sess); err != nil {
		return "", fmt.Errorf("decode session: %w", err)
	}
	if sess.ID == "" {
		return "", fmt.Errorf("worker returned an empty session id")
	}
	return sess.ID, nil
}

func (c *httpExecutorClient) Execute(ctx context.Context, sessionID, command string) (ExecResult, error) {
	_ = ctx
	req := executor.ExecuteRequest{
		SessionID: sessionID,
		Command:   command,
		Title:     "voice command",
		Reason:    "spoken command from the voice catcher",
	}
	payload, _ := json.Marshal(req)

	start := time.Now()
	body, err := c.worker.Post("/execute", req)
	elapsed := time.Since(start).Milliseconds()
	if err != nil {
		return ExecResult{RequestPayload: string(payload), DurationMS: elapsed}, fmt.Errorf("execute command: %w", err)
	}
	var resp executor.ExecuteResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return ExecResult{RequestPayload: string(payload), DurationMS: elapsed}, fmt.Errorf("decode execute response: %w", err)
	}
	return ExecResult{
		ExitCode:       resp.ExitCode,
		Summary:        resp.Summary,
		Success:        resp.Success,
		RequestPayload: string(payload),
		DurationMS:     elapsed,
	}, nil
}
