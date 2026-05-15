package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const DefaultWorkerURL = "http://127.0.0.1:1088"

type Client struct {
	baseURL    string
	httpClient *http.Client
}

type CreateSessionRequest struct {
	Name        string `json:"name"`
	Workspace   string `json:"workspace"`
	Description string `json:"description,omitempty"`
}

type Session struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Workspace string `json:"workspace"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	ClosedAt  string `json:"closed_at,omitempty"`
}

type HealthResponse struct {
	Status    string `json:"status"`
	Service   string `json:"service"`
	Timestamp string `json:"timestamp,omitempty"`
}

type RunCommandRequest struct {
	Command       string `json:"command"`
	CWD           string `json:"cwd,omitempty"`
	Timeout       int    `json:"timeout,omitempty"`
	Title         string `json:"title,omitempty"`
	Purpose       string `json:"purpose,omitempty"`
	ReasoningStep string `json:"reasoning_step,omitempty"`
	Target        string `json:"target,omitempty"`
}

type CommandResult struct {
	ID          string `json:"id"`
	SessionID   string `json:"session_id"`
	Command     string `json:"command"`
	CWD         string `json:"cwd"`
	ExitCode    int    `json:"exit_code"`
	Output      string `json:"output"`
	ErrorOutput string `json:"error_output"`
	DurationMS  int64  `json:"duration_ms"`
	CreatedAt   string `json:"created_at"`
}
type ReadFileRequest struct {
	SessionID string `json:"session_id"`
	Path      string `json:"path"`
}

type ReadFileResponse struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type WriteFileRequest struct {
	SessionID string `json:"session_id"`
	Path      string `json:"path"`
	Content   string `json:"content"`
}

type WriteFileResponse struct {
	Path string `json:"path"`
}

type AgentRunRequest struct {
	SessionID        string `json:"session_id,omitempty"`
	TaskID           string `json:"task_id,omitempty"`
	TaskFile         string `json:"task_file,omitempty"`
	Workspace        string `json:"workspace,omitempty"`
	Branch           string `json:"branch,omitempty"`
	WorktreePath     string `json:"worktree_path,omitempty"`
	FinalizationMode string `json:"finalization_mode,omitempty"`
	MaxSteps         int    `json:"max_steps,omitempty"`
	CurrentPhase     string `json:"current_phase,omitempty"`
	FinalReportPath  string `json:"final_report_path,omitempty"`
	AutoStart        bool   `json:"auto_start,omitempty"`
}

type AgentRun struct {
	ID               string `json:"id"`
	SessionID        string `json:"session_id"`
	TaskID           string `json:"task_id"`
	TaskFile         string `json:"task_file"`
	Workspace        string `json:"workspace"`
	Branch           string `json:"branch"`
	WorktreePath     string `json:"worktree_path"`
	FinalizationMode string `json:"finalization_mode"`
	Status           string `json:"status"`
	CurrentStep      int    `json:"current_step"`
	MaxSteps         int    `json:"max_steps"`
	CurrentPhase     string `json:"current_phase"`
	LastError        string `json:"last_error"`
	FinalReportPath  string `json:"final_report_path"`
}

func NewClient(baseURL string) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultWorkerURL
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func WorkerURLFromEnv(getenv func(string) string) string {
	if getenv == nil {
		getenv = os.Getenv
	}
	if value := strings.TrimSpace(getenv("PTOLEMY_WORKER_URL")); value != "" {
		return strings.TrimRight(value, "/")
	}
	return DefaultWorkerURL
}

func (c *Client) Health(ctx context.Context) (*HealthResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("health check failed with status %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var result HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) ListSessions(ctx context.Context) ([]Session, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/sessions", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("list sessions failed with status %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var sessions []Session
	if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
		return nil, err
	}
	return sessions, nil
}

func (c *Client) CreateSession(ctx context.Context, reqBody CreateSessionRequest) (*Session, error) {
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/sessions", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("create session failed with status %s", resp.Status)
	}

	var session Session
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return nil, err
	}

	return &session, nil
}

func (c *Client) RunCommand(ctx context.Context, sessionID string, reqBody RunCommandRequest) (*CommandResult, error) {
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/sessions/%s/commands", c.baseURL, sessionID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		var errBody map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		return nil, fmt.Errorf("run command failed with status %s: %v", resp.Status, errBody)
	}

	var result CommandResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}
func (c *Client) ReadFile(ctx context.Context, reqBody ReadFileRequest) (*ReadFileResponse, error) {
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/file/read", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		var errBody map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		return nil, fmt.Errorf("read file failed with status %s: %v", resp.Status, errBody)
	}

	var result ReadFileResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (c *Client) WriteFile(ctx context.Context, reqBody WriteFileRequest) (*WriteFileResponse, error) {
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/file/write", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		var errBody map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		return nil, fmt.Errorf("write file failed with status %s: %v", resp.Status, errBody)
	}

	var result WriteFileResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (c *Client) CreateAgentRun(ctx context.Context, reqBody AgentRunRequest) (*AgentRun, error) {
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/agent-runs", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		var errBody map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		return nil, fmt.Errorf("create agent run failed with status %s: %v", resp.Status, errBody)
	}

	var result AgentRun
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (c *Client) ResumeAgentRun(ctx context.Context, runID string) (*AgentRun, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/agent-runs/%s/resume", c.baseURL, runID), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		var errBody map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		return nil, fmt.Errorf("resume agent run failed with status %s: %v", resp.Status, errBody)
	}

	var result AgentRun
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}
