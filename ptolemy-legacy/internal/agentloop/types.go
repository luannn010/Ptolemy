package agentloop

import "time"

const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusPaused    = "paused"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
)

type Run struct {
	ID                   string    `json:"id"`
	SessionID            string    `json:"session_id"`
	TaskID               string    `json:"task_id"`
	TaskFile             string    `json:"task_file"`
	Workspace            string    `json:"workspace"`
	Branch               string    `json:"branch"`
	WorktreePath         string    `json:"worktree_path"`
	FinalizationMode     string    `json:"finalization_mode"`
	Status               string    `json:"status"`
	CurrentStep          int       `json:"current_step"`
	MaxSteps             int       `json:"max_steps"`
	InvalidJSONCount     int       `json:"invalid_json_count"`
	SameActionRetryCount int       `json:"same_action_retry_count"`
	NoProgressCount      int       `json:"no_progress_count"`
	CurrentPhase         string    `json:"current_phase"`
	LastError            string    `json:"last_error"`
	FinalReportPath      string    `json:"final_report_path"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type Observation struct {
	ID           string    `json:"id"`
	RunID        string    `json:"run_id"`
	ActionID     string    `json:"action_id"`
	Step         int       `json:"step"`
	Source       string    `json:"source"`
	Summary      string    `json:"summary"`
	ArtifactPath string    `json:"artifact_path"`
	RawPayload   string    `json:"raw_payload"`
	CreatedAt    time.Time `json:"created_at"`
}
