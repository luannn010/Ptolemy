package packstudio

import (
	"time"

	"github.com/luannn010/ptolemy/internal/agentloop"
)

const (
	StatusPending        = "pending"
	StatusPlanning       = "planning"
	StatusRunning        = "running"
	StatusWaitingOnAgent = "waiting_on_agent"
	StatusCompleted      = "completed"
	StatusFailed         = "failed"
	StatusCancelled      = "cancelled"
)

type ChecklistItem struct {
	Text    string `json:"text"`
	Checked bool   `json:"checked"`
}

type PackCatalogItem struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Path             string   `json:"path"`
	TaskCount        int      `json:"task_count"`
	Valid            bool     `json:"valid"`
	ValidationErrors []string `json:"validation_errors,omitempty"`
}

type ProgramCatalogItem struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Path             string   `json:"path"`
	PackCount        int      `json:"pack_count"`
	Valid            bool     `json:"valid"`
	ValidationErrors []string `json:"validation_errors,omitempty"`
}

type PackTaskSummary struct {
	ID               string          `json:"id"`
	Title            string          `json:"title"`
	Path             string          `json:"path"`
	Branch           string          `json:"branch"`
	Status           string          `json:"status"`
	DependsOn        []string        `json:"depends_on"`
	AllowedFiles     []string        `json:"allowed_files"`
	Validation       []string        `json:"validation"`
	Checklist        []ChecklistItem `json:"checklist"`
	ValidationErrors []string        `json:"validation_errors,omitempty"`
}

type PackDetail struct {
	PackCatalogItem
	Goal     string            `json:"goal"`
	Readme   string            `json:"readme"`
	Manifest map[string]string `json:"manifest,omitempty"`
	Tasks    []PackTaskSummary `json:"tasks"`
}

type ProgramPackRef struct {
	PackID    string   `json:"pack_id"`
	DependsOn []string `json:"depends_on,omitempty"`
	Order     int      `json:"order"`
}

type ProgramDefinition struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Path        string           `json:"path"`
	Packs       []ProgramPackRef `json:"packs"`
}

type PackTaskInput struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Summary        string   `json:"summary"`
	Branch         string   `json:"branch"`
	DependsOn      []string `json:"depends_on"`
	AllowedFiles   []string `json:"allowed_files"`
	Validation     []string `json:"validation"`
	Scripts        []string `json:"scripts,omitempty"`
	Snippets       []string `json:"snippets,omitempty"`
	ExecutionGroup string   `json:"execution_group,omitempty"`
}

type CreatePackInput struct {
	PackID            string          `json:"pack_id"`
	Name              string          `json:"name"`
	Description       string          `json:"description"`
	Goal              string          `json:"goal"`
	CreatedBy         string          `json:"created_by"`
	Requires          []string        `json:"requires,omitempty"`
	Validation        []string        `json:"validation,omitempty"`
	MaxAllowedFiles   int             `json:"max_allowed_files"`
	RequireValidation bool            `json:"require_validation"`
	RequireBranch     bool            `json:"require_branch"`
	StopOnFailure     bool            `json:"stop_on_failure"`
	Tasks             []PackTaskInput `json:"tasks"`
}

type CreateProgramInput struct {
	ProgramID   string           `json:"program_id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Packs       []ProgramPackRef `json:"packs"`
}

type ProgramRun struct {
	ID              string    `json:"id"`
	ProgramID       string    `json:"program_id"`
	ProgramName     string    `json:"program_name"`
	Mode            string    `json:"mode"`
	Status          string    `json:"status"`
	Workspace       string    `json:"workspace"`
	CurrentPackID   string    `json:"current_pack_id"`
	CurrentTaskID   string    `json:"current_task_id"`
	PercentComplete float64   `json:"percent_complete"`
	TotalPacks      int       `json:"total_packs"`
	CompletedPacks  int       `json:"completed_packs"`
	TotalTasks      int       `json:"total_tasks"`
	CompletedTasks  int       `json:"completed_tasks"`
	LastError       string    `json:"last_error"`
	CreatedAt       time.Time `json:"created_at"`
	StartedAt       time.Time `json:"started_at"`
	FinishedAt      time.Time `json:"finished_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type PackRun struct {
	ID              string    `json:"id"`
	ProgramRunID    string    `json:"program_run_id"`
	PackID          string    `json:"pack_id"`
	PackName        string    `json:"pack_name"`
	PackPath        string    `json:"pack_path"`
	Status          string    `json:"status"`
	Position        int       `json:"position"`
	CurrentTaskID   string    `json:"current_task_id"`
	PercentComplete float64   `json:"percent_complete"`
	TotalTasks      int       `json:"total_tasks"`
	CompletedTasks  int       `json:"completed_tasks"`
	LastError       string    `json:"last_error"`
	CreatedAt       time.Time `json:"created_at"`
	StartedAt       time.Time `json:"started_at"`
	FinishedAt      time.Time `json:"finished_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type PackRunTask struct {
	ID             string    `json:"id"`
	PackRunID      string    `json:"pack_run_id"`
	TaskID         string    `json:"task_id"`
	Title          string    `json:"title"`
	TaskPath       string    `json:"task_path"`
	Branch         string    `json:"branch"`
	Status         string    `json:"status"`
	Position       int       `json:"position"`
	AgentRunID     string    `json:"agent_run_id"`
	SessionID      string    `json:"session_id"`
	ExecutionGroup string    `json:"execution_group"`
	LastError      string    `json:"last_error"`
	CreatedAt      time.Time `json:"created_at"`
	StartedAt      time.Time `json:"started_at"`
	FinishedAt     time.Time `json:"finished_at"`
	UpdatedAt      time.Time `json:"updated_at"`

	DependsOnRaw    string `json:"-"`
	ValidationRaw   string `json:"-"`
	AllowedFilesRaw string `json:"-"`
	ChecklistRaw    string `json:"-"`
}

type RunEvent struct {
	ID            string    `json:"id"`
	ProgramRunID  string    `json:"program_run_id"`
	PackRunID     string    `json:"pack_run_id"`
	PackRunTaskID string    `json:"pack_run_task_id"`
	AgentRunID    string    `json:"agent_run_id"`
	SessionID     string    `json:"session_id"`
	EventType     string    `json:"event_type"`
	Message       string    `json:"message"`
	Data          string    `json:"data"`
	ArtifactPath  string    `json:"artifact_path"`
	CreatedAt     time.Time `json:"created_at"`
}

type RunTaskDetail struct {
	PackRunTask
	DependsOn    []string        `json:"depends_on"`
	Validation   []string        `json:"validation"`
	AllowedFiles []string        `json:"allowed_files"`
	Checklist    []ChecklistItem `json:"checklist"`
}

type PackRunDetail struct {
	PackRun
	Tasks []RunTaskDetail `json:"tasks"`
}

type ProgramRunDetail struct {
	ProgramRun ProgramRun         `json:"program_run"`
	Program    *ProgramDefinition `json:"program,omitempty"`
	Packs      []PackRunDetail    `json:"packs"`
	Events     []RunEvent         `json:"events"`
}

type TerminalState struct {
	Status              string    `json:"status"`
	Message             string    `json:"message"`
	SessionID           string    `json:"session_id"`
	TaskID              string    `json:"task_id"`
	TaskTitle           string    `json:"task_title"`
	Phase               string    `json:"phase"`
	Source              string    `json:"source"`
	HasSession          bool      `json:"has_session"`
	CaptureAvailable    bool      `json:"capture_available"`
	SessionBootstrapped bool      `json:"session_bootstrapped"`
	Snapshot            string    `json:"snapshot"`
	LastUpdated         time.Time `json:"last_updated"`
}

type RunOperatorState struct {
	AgentRun *agentloop.RunInspection `json:"agent_run,omitempty"`
	Terminal TerminalState            `json:"terminal"`
}
