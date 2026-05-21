---
project: Ptolemy
category: code-map
status: Ready
last_verified: 2026-05-11
source: codebase
confidence: high
tags:
  - ptolemy
  - database
---

# Database Tables

| Table | Purpose | Key columns | Source file | Status |
|---|---|---|---|---|
| `sessions` | Worker sessions and workspace binding | `id`, `name`, `status`, `workspace`, `created_at`, `closed_at` | `internal/store/store.go` | Ready |
| `command_logs` | Command execution history per session | `id`, `session_id`, `command`, `cwd`, `exit_code`, `duration_ms` | `internal/store/store.go` | Ready |
| `schema_migrations` | Migration tracking | `version`, `applied_at` | `internal/store/migrations.go` | Ready |
| `actions` | Normalized action records | `id`, `session_id`, `agent_run_id`, `type`, `status`, `metadata` | `internal/store/migrations.go` | Ready |
| `logs` | Structured log records | `id`, `session_id`, `action_id`, `level`, `message` | `internal/store/migrations.go` | Ready |
| `approvals` | Approval requests and decisions | `id`, `session_id`, `action_type`, `status`, `reason` | `internal/store/migrations.go` | Partial |
| `agent_runs` | Controller-owned agent run state | `id`, `session_id`, `task_id`, `task_file`, `status`, `current_step`, `finalization_mode` | `internal/store/migrations.go` | Partial |
| `agent_observations` | Agent run observations and artifacts | `id`, `run_id`, `action_id`, `step`, `source`, `artifact_path` | `internal/store/migrations.go` | Partial |
| `program_runs` | High-level program execution state | `id`, `program_id`, `status`, `workspace`, `current_pack_id`, `percent_complete` | `internal/store/migrations.go` | Partial |
| `pack_runs` | Pack-level execution state within a program | `id`, `program_run_id`, `pack_id`, `status`, `position`, `current_task_id` | `internal/store/migrations.go` | Partial |
| `pack_run_tasks` | Task-level execution state inside a pack run | `id`, `pack_run_id`, `task_id`, `status`, `agent_run_id`, `depends_on_json`, `allowed_files_json` | `internal/store/migrations.go` | Partial |
| `run_events` | Event timeline for program and pack runs | `id`, `program_run_id`, `pack_run_id`, `agent_run_id`, `event_type`, `artifact_path` | `internal/store/migrations.go` | Partial |

