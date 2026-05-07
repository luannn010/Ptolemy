package command

import "time"

type CommandLog struct {
	ID          string    `json:"id"`
	SessionID   string    `json:"session_id"`
	Command     string    `json:"command"`
	CWD         string    `json:"cwd"`
	ExitCode    int       `json:"exit_code"`
	Output      string    `json:"output"`
	ErrorOutput string    `json:"error_output"`
	DurationMS  int64     `json:"duration_ms"`
	CreatedAt   time.Time `json:"created_at"`
}

type RunCommandRequest struct {
	Command       string `json:"command"`
	CWD           string `json:"cwd"`
	Timeout       int    `json:"timeout"`
	Title         string `json:"title,omitempty"`
	Purpose       string `json:"purpose,omitempty"`
	ReasoningStep string `json:"reasoning_step,omitempty"`
	Target        string `json:"target,omitempty"`
}
