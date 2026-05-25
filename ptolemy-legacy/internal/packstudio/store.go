package packstudio

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrProgramRunNotFound = errors.New("program run not found")

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) MarkRunningRunsFailed(ctx context.Context, message string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`UPDATE program_runs
		 SET status = ?, last_error = ?, finished_at = ?, updated_at = ?
		 WHERE status IN (?, ?, ?)`,
		StatusFailed,
		message,
		now,
		now,
		StatusPlanning,
		StatusRunning,
		StatusWaitingOnAgent,
	)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE pack_runs
		 SET status = ?, last_error = ?, finished_at = ?, updated_at = ?
		 WHERE status IN (?, ?, ?)`,
		StatusFailed,
		message,
		now,
		now,
		StatusPlanning,
		StatusRunning,
		StatusWaitingOnAgent,
	)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE pack_run_tasks
		 SET status = ?, last_error = ?, finished_at = ?, updated_at = ?
		 WHERE status IN (?, ?)`,
		StatusFailed,
		message,
		now,
		now,
		StatusRunning,
		StatusWaitingOnAgent,
	)
	return err
}

func (s *Store) HasActiveProgramRun(ctx context.Context) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM program_runs WHERE status IN (?, ?, ?)`,
		StatusPlanning,
		StatusRunning,
		StatusWaitingOnAgent,
	).Scan(&count)
	return count > 0, err
}

func (s *Store) CreateProgramRun(ctx context.Context, run ProgramRun) (ProgramRun, error) {
	now := time.Now().UTC()
	run.ID = uuid.NewString()
	run.CreatedAt = now
	run.UpdatedAt = now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO program_runs
		(id, program_id, program_name, mode, status, workspace, current_pack_id, current_task_id,
		 percent_complete, total_packs, completed_packs, total_tasks, completed_tasks, last_error,
		 created_at, started_at, finished_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.ProgramID, run.ProgramName, run.Mode, run.Status, run.Workspace, run.CurrentPackID, run.CurrentTaskID,
		run.PercentComplete, run.TotalPacks, run.CompletedPacks, run.TotalTasks, run.CompletedTasks, run.LastError,
		formatTime(run.CreatedAt), formatTime(run.StartedAt), formatTime(run.FinishedAt), formatTime(run.UpdatedAt),
	)
	return run, err
}

func (s *Store) UpdateProgramRun(ctx context.Context, run ProgramRun) (ProgramRun, error) {
	run.UpdatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE program_runs
		 SET program_id = ?, program_name = ?, mode = ?, status = ?, workspace = ?, current_pack_id = ?, current_task_id = ?,
		     percent_complete = ?, total_packs = ?, completed_packs = ?, total_tasks = ?, completed_tasks = ?, last_error = ?,
		     started_at = ?, finished_at = ?, updated_at = ?
		 WHERE id = ?`,
		run.ProgramID, run.ProgramName, run.Mode, run.Status, run.Workspace, run.CurrentPackID, run.CurrentTaskID,
		run.PercentComplete, run.TotalPacks, run.CompletedPacks, run.TotalTasks, run.CompletedTasks, run.LastError,
		formatTime(run.StartedAt), formatTime(run.FinishedAt), formatTime(run.UpdatedAt), run.ID,
	)
	if err != nil {
		return ProgramRun{}, err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ProgramRun{}, ErrProgramRunNotFound
	}
	return s.GetProgramRun(ctx, run.ID)
}

func (s *Store) GetProgramRun(ctx context.Context, id string) (ProgramRun, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, program_id, program_name, mode, status, workspace, current_pack_id, current_task_id,
		        percent_complete, total_packs, completed_packs, total_tasks, completed_tasks, last_error,
		        created_at, started_at, finished_at, updated_at
		   FROM program_runs WHERE id = ?`, id)
	run, err := scanProgramRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ProgramRun{}, ErrProgramRunNotFound
	}
	return run, err
}

func (s *Store) ListProgramRuns(ctx context.Context, limit int) ([]ProgramRun, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, program_id, program_name, mode, status, workspace, current_pack_id, current_task_id,
		        percent_complete, total_packs, completed_packs, total_tasks, completed_tasks, last_error,
		        created_at, started_at, finished_at, updated_at
		   FROM program_runs
		  ORDER BY created_at DESC
		  LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProgramRun{}
	for rows.Next() {
		run, scanErr := scanProgramRun(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

func (s *Store) CreatePackRun(ctx context.Context, run PackRun) (PackRun, error) {
	now := time.Now().UTC()
	run.ID = uuid.NewString()
	run.CreatedAt = now
	run.UpdatedAt = now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO pack_runs
		(id, program_run_id, pack_id, pack_name, pack_path, status, position, current_task_id,
		 percent_complete, total_tasks, completed_tasks, last_error,
		 created_at, started_at, finished_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.ProgramRunID, run.PackID, run.PackName, run.PackPath, run.Status, run.Position, run.CurrentTaskID,
		run.PercentComplete, run.TotalTasks, run.CompletedTasks, run.LastError,
		formatTime(run.CreatedAt), formatTime(run.StartedAt), formatTime(run.FinishedAt), formatTime(run.UpdatedAt),
	)
	return run, err
}

func (s *Store) UpdatePackRun(ctx context.Context, run PackRun) (PackRun, error) {
	run.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE pack_runs
		 SET status = ?, current_task_id = ?, percent_complete = ?, total_tasks = ?, completed_tasks = ?, last_error = ?,
		     started_at = ?, finished_at = ?, updated_at = ?
		 WHERE id = ?`,
		run.Status, run.CurrentTaskID, run.PercentComplete, run.TotalTasks, run.CompletedTasks, run.LastError,
		formatTime(run.StartedAt), formatTime(run.FinishedAt), formatTime(run.UpdatedAt), run.ID,
	)
	if err != nil {
		return PackRun{}, err
	}
	return s.GetPackRun(ctx, run.ID)
}

func (s *Store) GetPackRun(ctx context.Context, id string) (PackRun, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, program_run_id, pack_id, pack_name, pack_path, status, position, current_task_id,
		        percent_complete, total_tasks, completed_tasks, last_error,
		        created_at, started_at, finished_at, updated_at
		   FROM pack_runs WHERE id = ?`, id)
	return scanPackRun(row)
}

func (s *Store) ListPackRunsByProgram(ctx context.Context, programRunID string) ([]PackRun, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, program_run_id, pack_id, pack_name, pack_path, status, position, current_task_id,
		        percent_complete, total_tasks, completed_tasks, last_error,
		        created_at, started_at, finished_at, updated_at
		   FROM pack_runs
		  WHERE program_run_id = ?
		  ORDER BY position ASC`, programRunID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PackRun{}
	for rows.Next() {
		item, scanErr := scanPackRun(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) CreatePackRunTask(ctx context.Context, task PackRunTask) (PackRunTask, error) {
	now := time.Now().UTC()
	task.ID = uuid.NewString()
	task.CreatedAt = now
	task.UpdatedAt = now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO pack_run_tasks
		(id, pack_run_id, task_id, title, task_path, branch, status, position, agent_run_id, session_id,
		 execution_group, last_error, depends_on_json, validation_json, allowed_files_json, checklist_json,
		 created_at, started_at, finished_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.ID, task.PackRunID, task.TaskID, task.Title, task.TaskPath, task.Branch, task.Status, task.Position,
		task.AgentRunID, task.SessionID, task.ExecutionGroup, task.LastError,
		task.DependsOnRaw, task.ValidationRaw, task.AllowedFilesRaw, task.ChecklistRaw,
		formatTime(task.CreatedAt), formatTime(task.StartedAt), formatTime(task.FinishedAt), formatTime(task.UpdatedAt),
	)
	return task, err
}

func (s *Store) UpdatePackRunTask(ctx context.Context, task PackRunTask) (PackRunTask, error) {
	task.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE pack_run_tasks
		 SET status = ?, agent_run_id = ?, session_id = ?, last_error = ?, started_at = ?, finished_at = ?, updated_at = ?
		 WHERE id = ?`,
		task.Status, task.AgentRunID, task.SessionID, task.LastError,
		formatTime(task.StartedAt), formatTime(task.FinishedAt), formatTime(task.UpdatedAt), task.ID,
	)
	if err != nil {
		return PackRunTask{}, err
	}
	return s.GetPackRunTask(ctx, task.ID)
}

func (s *Store) GetPackRunTask(ctx context.Context, id string) (PackRunTask, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, pack_run_id, task_id, title, task_path, branch, status, position, agent_run_id, session_id,
		        execution_group, last_error, depends_on_json, validation_json, allowed_files_json, checklist_json,
		        created_at, started_at, finished_at, updated_at
		   FROM pack_run_tasks WHERE id = ?`, id)
	return scanPackRunTask(row)
}

func (s *Store) ListPackRunTasks(ctx context.Context, packRunID string) ([]PackRunTask, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, pack_run_id, task_id, title, task_path, branch, status, position, agent_run_id, session_id,
		        execution_group, last_error, depends_on_json, validation_json, allowed_files_json, checklist_json,
		        created_at, started_at, finished_at, updated_at
		   FROM pack_run_tasks
		  WHERE pack_run_id = ?
		  ORDER BY position ASC`, packRunID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PackRunTask{}
	for rows.Next() {
		item, scanErr := scanPackRunTask(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) CreateEvent(ctx context.Context, event RunEvent) (RunEvent, error) {
	now := time.Now().UTC()
	event.ID = uuid.NewString()
	event.CreatedAt = now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO run_events
		(id, program_run_id, pack_run_id, pack_run_task_id, agent_run_id, session_id, event_type, message, data, artifact_path, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.ID, event.ProgramRunID, event.PackRunID, event.PackRunTaskID, event.AgentRunID, event.SessionID,
		event.EventType, event.Message, event.Data, event.ArtifactPath, formatTime(event.CreatedAt),
	)
	return event, err
}

func (s *Store) ListEvents(ctx context.Context, programRunID string, limit int) ([]RunEvent, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, program_run_id, pack_run_id, pack_run_task_id, agent_run_id, session_id,
		        event_type, message, data, artifact_path, created_at
		   FROM run_events
		  WHERE program_run_id = ?
		  ORDER BY created_at ASC
		  LIMIT ?`, programRunID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RunEvent{}
	for rows.Next() {
		item, scanErr := scanRunEvent(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func scanProgramRun(row interface{ Scan(dest ...any) error }) (ProgramRun, error) {
	var item ProgramRun
	var createdAt, startedAt, finishedAt, updatedAt string
	err := row.Scan(
		&item.ID, &item.ProgramID, &item.ProgramName, &item.Mode, &item.Status, &item.Workspace, &item.CurrentPackID, &item.CurrentTaskID,
		&item.PercentComplete, &item.TotalPacks, &item.CompletedPacks, &item.TotalTasks, &item.CompletedTasks, &item.LastError,
		&createdAt, &startedAt, &finishedAt, &updatedAt,
	)
	if err != nil {
		return ProgramRun{}, err
	}
	item.CreatedAt = parseTime(createdAt)
	item.StartedAt = parseTime(startedAt)
	item.FinishedAt = parseTime(finishedAt)
	item.UpdatedAt = parseTime(updatedAt)
	return item, nil
}

func scanPackRun(row interface{ Scan(dest ...any) error }) (PackRun, error) {
	var item PackRun
	var createdAt, startedAt, finishedAt, updatedAt string
	err := row.Scan(
		&item.ID, &item.ProgramRunID, &item.PackID, &item.PackName, &item.PackPath, &item.Status, &item.Position, &item.CurrentTaskID,
		&item.PercentComplete, &item.TotalTasks, &item.CompletedTasks, &item.LastError,
		&createdAt, &startedAt, &finishedAt, &updatedAt,
	)
	if err != nil {
		return PackRun{}, err
	}
	item.CreatedAt = parseTime(createdAt)
	item.StartedAt = parseTime(startedAt)
	item.FinishedAt = parseTime(finishedAt)
	item.UpdatedAt = parseTime(updatedAt)
	return item, nil
}

func scanPackRunTask(row interface{ Scan(dest ...any) error }) (PackRunTask, error) {
	var item PackRunTask
	var createdAt, startedAt, finishedAt, updatedAt string
	err := row.Scan(
		&item.ID, &item.PackRunID, &item.TaskID, &item.Title, &item.TaskPath, &item.Branch, &item.Status, &item.Position,
		&item.AgentRunID, &item.SessionID, &item.ExecutionGroup, &item.LastError,
		&item.DependsOnRaw, &item.ValidationRaw, &item.AllowedFilesRaw, &item.ChecklistRaw,
		&createdAt, &startedAt, &finishedAt, &updatedAt,
	)
	if err != nil {
		return PackRunTask{}, err
	}
	item.CreatedAt = parseTime(createdAt)
	item.StartedAt = parseTime(startedAt)
	item.FinishedAt = parseTime(finishedAt)
	item.UpdatedAt = parseTime(updatedAt)
	return item, nil
}

func scanRunEvent(row interface{ Scan(dest ...any) error }) (RunEvent, error) {
	var item RunEvent
	var createdAt string
	err := row.Scan(
		&item.ID, &item.ProgramRunID, &item.PackRunID, &item.PackRunTaskID, &item.AgentRunID, &item.SessionID,
		&item.EventType, &item.Message, &item.Data, &item.ArtifactPath, &createdAt,
	)
	if err != nil {
		return RunEvent{}, err
	}
	item.CreatedAt = parseTime(createdAt)
	return item, nil
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func parseTime(raw string) time.Time {
	if raw == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func marshalStrings(items []string) string {
	data, _ := json.Marshal(items)
	return string(data)
}

func marshalChecklist(items []ChecklistItem) string {
	data, _ := json.Marshal(items)
	return string(data)
}

func unmarshalStrings(raw string) []string {
	if raw == "" {
		return []string{}
	}
	var items []string
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return []string{}
	}
	return items
}

func unmarshalChecklist(raw string) []ChecklistItem {
	if raw == "" {
		return []ChecklistItem{}
	}
	var items []ChecklistItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return []ChecklistItem{}
	}
	return items
}
