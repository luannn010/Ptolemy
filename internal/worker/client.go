package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

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
}

type RunCommandRequest struct {
	Command string `json:"command"`
	CWD     string `json:"cwd,omitempty"`
	Timeout int    `json:"timeout,omitempty"`
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

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
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
