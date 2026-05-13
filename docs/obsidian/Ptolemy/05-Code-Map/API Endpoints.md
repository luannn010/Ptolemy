---
project: Ptolemy
category: code-map
status: Ready
last_verified: 2026-05-11
source: codebase
confidence: high
tags:
  - ptolemy
  - api
---

# API Endpoints

| Method | Path | Handler file | Purpose | Request shape | Response shape | Status |
|---|---|---|---|---|---|---|
| `GET` | `/health` | `internal/httpapi/router.go` | Health check | none | `{status, service, timestamp}` | Ready |
| `POST` | `/execute` | `internal/httpapi/execute.go` | Session command execution | `{session_id, command, cwd?, timeout?, title?, purpose?, reasoning_step?, target?}` | `{session_id, command, exit_code, output, summary, duration_ms, success}` | Ready |
| `POST` | `/sessions` | `internal/httpapi/session.go` | Create session | `{name, workspace?, description?}` | session object | Ready |
| `GET` | `/sessions` | `internal/httpapi/session.go` | List sessions | none | `[]session` | Ready |
| `GET` | `/sessions/{id}` | `internal/httpapi/session.go` | Get session | path id | session object | Ready |
| `POST` | `/sessions/{id}/close` | `internal/httpapi/session.go` | Close session | path id | session object | Ready |
| `POST` | `/sessions/{id}/commands` | `internal/httpapi/commands.go` | Run command in session | `{command, cwd?, timeout?, title?, purpose?, reasoning_step?, target?}` | command log or approval/policy error | Ready |
| `GET` | `/sessions/{id}/commands` | `internal/httpapi/commands.go` | List command logs | path id | `[]command_log` | Ready |
| `POST` | `/agent-runs` | `internal/httpapi/agent_runs.go` | Create agent run | `{session_id?, task_id?, task_file?, workspace?, branch?, worktree_path?, finalization_mode?, max_steps?, current_phase?, final_report_path?, last_error?, auto_start?}` | agent run | Partial |
| `GET` | `/agent-runs/{id}` | `internal/httpapi/agent_runs.go` | Get agent run | path id | agent run | Partial |
| `GET` | `/agent-runs/{id}/actions` | `internal/httpapi/agent_runs.go` | List run actions | path id | `[]action` | Partial |
| `GET` | `/agent-runs/{id}/observations` | `internal/httpapi/agent_runs.go` | List observations | path id | `[]observation` | Partial |
| `POST` | `/agent-runs/{id}/resume` | `internal/httpapi/agent_runs.go` | Resume or start run | path id | updated agent run | Partial |
| `POST` | `/agent-runs/{id}/cancel` | `internal/httpapi/agent_runs.go` | Cancel run | path id | updated agent run | Partial |
| `POST` | `/file/read` | `internal/httpapi/fileops.go` | Read file | `{session_id, task_session_id?, path}` | `{path, content}` | Ready |
| `POST` | `/file/write` | `internal/httpapi/fileops.go` | Write file | `{session_id, path, content}` | `{path, written}` | Ready |
| `POST` | `/file/list` | `internal/httpapi/fileops.go` | List directory | `{session_id, path}` | `{path, entries}` | Ready |
| `POST` | `/file/search` | `internal/httpapi/fileops.go` | Search workspace | `{session_id, query}` | `{query, result}` | Ready |
| `POST` | `/file/apply` | `internal/httpapi/fileops.go` | Apply patch/file update | `{session_id, path, content}` | `{path, applied}` | Ready |
| `POST` | `/navigator/index` | `internal/httpapi/router.go` | Compatibility workspace index | JSON body handled by navigator | navigator result | Partial |
| `POST` | `/navigator/context` | `internal/httpapi/router.go` | Compatibility context read | JSON body handled by navigator | context files | Partial |
| `POST` | `/navigator/session/start` | `internal/httpapi/router.go` | Start task session | JSON body handled by navigator | task session info | Partial |
| `POST` | `/navigator/session/note` | `internal/httpapi/router.go` | Append task session note | JSON body handled by navigator | note result | Partial |
| `POST` | `/kb/build` | `internal/httpapi/router.go` | Canonical KB build | JSON body handled by navigator | KB build result | Ready |
| `POST` | `/kb/read` | `internal/httpapi/router.go` | Canonical KB read | JSON body handled by navigator | KB context | Ready |
| `POST` | `/kb/update` | `internal/httpapi/router.go` | Canonical KB update | JSON body handled by navigator | KB update result | Ready |
| `POST` | `/git/status` | `internal/httpapi/git.go` | Git status | `{session_id}` | git status output | Ready |
| `POST` | `/git/diff` | `internal/httpapi/git.go` | Git diff | `{session_id}` | git diff output | Ready |
| `POST` | `/git/log` | `internal/httpapi/git.go` | Git log | `{session_id}` | git log output | Ready |
| `POST` | `/git/checkout` | `internal/httpapi/git.go` | Checkout branch | `{session_id, branch}` | git result | Ready |
| `POST` | `/git/branch` | `internal/httpapi/git.go` | Create branch | `{session_id, branch}` | git result | Ready |
| `POST` | `/git/commit` | `internal/httpapi/git.go` | Commit changes | `{session_id, message}` | git result | Ready |
| `POST` | `/git/push` | `internal/httpapi/git.go` | Push branch | `{session_id, remote, branch}` | git result | Ready |
| `POST` | `/worktree/create` | `internal/httpapi/worktree.go` | Create worktree | `{session_id, name, branch}` | worktree result | Partial |
| `POST` | `/worktree/list` | `internal/httpapi/worktree.go` | List worktrees | `{session_id}` | worktree list | Partial |
| `POST` | `/worktree/remove` | `internal/httpapi/worktree.go` | Remove worktree | `{session_id, name}` | worktree result | Partial |
| `POST` | `/tasks/run-inbox` | `internal/httpapi/tasks.go` | Batch-run inbox metadata flow | `{dir?, max_batch?}` | `{ok, completed, failed, blocked, error?}` | Prototype |
| `GET` | `/ui` | `internal/httpapi/packstudio.go` | Pack Studio shell | none | HTML shell | Partial |
| `GET` | `/ui/studio` | `internal/httpapi/packstudio.go` | Studio shell | none | HTML shell | Partial |
| `GET` | `/ui/overview` | `internal/httpapi/packstudio.go` | Overview shell | none | HTML shell | Partial |
| `GET` | `/ui/runs/{id}` | `internal/httpapi/packstudio.go` | Run shell | path id | HTML shell | Partial |
| `GET` | `/ui/api/overview` | `internal/httpapi/packstudio.go` | UI overview data | none | packs/programs/runs summary | Partial |
| `GET` | `/ui/api/packs` | `internal/httpapi/packstudio.go` | List packs | none | pack list | Partial |
| `GET` | `/ui/api/packs/{id}` | `internal/httpapi/packstudio.go` | Get pack | path id | pack detail | Partial |
| `GET` | `/ui/api/packs/{id}/plan` | `internal/httpapi/packstudio.go` | Pack task order | path id | `{pack_id, tasks}` | Partial |
| `POST` | `/ui/api/packs` | `internal/httpapi/packstudio.go` | Create pack | pack creation JSON | pack detail | Partial |
| `GET` | `/ui/api/programs` | `internal/httpapi/packstudio.go` | List programs | none | program list | Partial |
| `GET` | `/ui/api/programs/{id}` | `internal/httpapi/packstudio.go` | Get program | path id | program + validation errors | Partial |
| `POST` | `/ui/api/programs` | `internal/httpapi/packstudio.go` | Create program | program creation JSON | program definition | Partial |
| `GET` | `/ui/api/program-runs` | `internal/httpapi/packstudio.go` | List program runs | none | run list | Partial |
| `POST` | `/ui/api/program-runs` | `internal/httpapi/packstudio.go` | Create program run | `{program_id, pack_id, workspace}` | program run | Partial |
| `GET` | `/ui/api/program-runs/{id}` | `internal/httpapi/packstudio.go` | Get program run detail | path id | run detail + operator state | Partial |
| `GET` | `/ui/api/program-runs/{id}/tree` | `internal/httpapi/packstudio.go` | Alias for run detail/tree | path id | run detail | Partial |
| `GET` | `/ui/api/program-runs/{id}/events` | `internal/httpapi/packstudio.go` | List run events | path id | event list | Partial |
| `GET` | `/ui/api/program-runs/{id}/terminal` | `internal/httpapi/packstudio.go` | Terminal snapshot | path id | terminal snapshot | Partial |
| `POST` | `/ui/api/program-runs/{id}/cancel` | `internal/httpapi/packstudio.go` | Cancel run | path id | status result | Partial |
| `GET` | `/ui/api/program-runs/{id}/events/stream` | `internal/httpapi/packstudio.go` | Event stream | path id | SSE stream | Partial |
| `GET` | `/ui/api/program-runs/{id}/terminal/stream` | `internal/httpapi/packstudio.go` | Terminal stream | path id | SSE/stream output | Partial |
| `GET` | `/ui/api/artifact` | `internal/httpapi/packstudio.go` | Read artifact file | query args | artifact response | Partial |

