# AGENTS.md — Ptolemy Worker Development Guide

## Purpose

This file tells Codex/agents how to work inside this repository.

Ptolemy is a local worker/agent system written in Go. It exposes a worker daemon (`workerd`), an MCP adapter (`ptolemy-mcp`), a local LLM-driven agent (`ptolemy-agent`), and a task runner prototype (`ptolemy-task-runner`).

The current development goal is to continue building a safe autonomous coding workflow:

```text
docs/tasks/inbox/*.md
→ classify task size
→ split large tasks
→ execute task with ptolemy-agent
→ validate with tests
→ commit task-related changes
→ move task to docs/tasks/done or docs/tasks/failed
```

## Current Known Architecture

### Main commands

- `cmd/workerd`
  - HTTP worker daemon.
  - Runs sessions, commands, file operations, Git endpoints, worktree endpoints.
  - Uses SQLite state in `state/ptolemy.db`.

- `cmd/ptolemy-mcp`
  - MCP adapter for exposing worker tools to external clients.

- `cmd/ptolemy-agent`
  - Local LLM-driven agent.
  - Calls llama.cpp Gemma server at `http://127.0.0.1:8088`.
  - Calls worker at `http://127.0.0.1:8080`.
  - Supports multi-step loop.
  - Supports actions:
    - `run_command`
    - `read_file`
    - `write_file`
    - `replace_block`
    - `insert_after`
    - `explain`
    - `ask_approval`

- `cmd/ptolemy-task-runner`
  - New task-runner prototype.
  - Should eventually scan `docs/tasks/inbox`, classify tasks, run tasks, commit, and move to done/failed.

### Important folders

- `internal/worker`
  - Go client for calling `workerd`.

- `internal/brain`
  - Go client for calling local llama.cpp OpenAI-compatible endpoint.

- `internal/policy`
  - Allow / ask / deny command policy.

- `internal/memory`
  - Markdown memory loader.

- `internal/inspect`
  - Workspace/project inspector.

- `internal/terminal`
  - tmux-backed command runner.

- `docs/memory`
  - Agent-readable long-term project memory.

- `docs/tasks`
  - Task specifications for Ptolemy.
  - Current desired structure:
    - `docs/tasks/inbox`
    - `docs/tasks/active`
    - `docs/tasks/split`
    - `docs/tasks/done`
    - `docs/tasks/failed`
    - `docs/tasks/scripts`

- `.state/agent-artifacts`
  - Temporary artifact/log storage.
  - Do not commit this folder.

## Critical Rules for Agents

## Workflow Loading Rules

Before executing a task:

1. Read `WORKFLOWS.md`.
2. Select only the workflow file relevant to the task.
3. Use Ptolemy for command execution when available.
4. Do not load every workflow file unless the task explicitly requires a full workflow audit.
5. Follow `docs/workflows/git/safe-commit.md` before committing.
6. If Ptolemy returns EOF, timeout, or no response, follow `docs/workflows/recovery/eof-worker-drop.md`.

## Task Flags, Isolation, and PR Rules

Before starting a task:

1. Read `WORKFLOWS.md`.
2. Load `docs/workflows/agent/task-flags-and-isolation.md`.
3. Confirm the task file has valid metadata.
4. Work on only one `task_id` at a time.
5. Use the task metadata `branch` field for branch creation.
6. Do not edit outside `allowed_files`.

When splitting tasks:

1. Child tasks inherit `priority`, `parent_task`, and `allowed_files`.
2. Child tasks must receive unique `task_id` values.
3. Child branches must use each child task ID.

After a task is tested and committed:

1. Follow `docs/workflows/git/pull-request.md` if PR creation is requested.
2. If GitHub CLI is unavailable or unauthenticated, write fallback PR instructions under `.state/pr/`.
3. Do not auto-merge PRs unless a task explicitly requests it.

## Task Branch Workflow

Before starting a task:

1. Read `WORKFLOWS.md`.
2. Ensure working branch is clean.
3. Create branch: `ptolemy/<task-slug>`.

After task:

1. Run tests.
2. Stage explicit files only.
3. Commit on task branch.
4. Merge only if working branch is clean.
5. If conflict occurs → STOP and report.

Never:
- use `git add .`
- auto-resolve runtime conflicts
- overwrite user files

### General safety

- Never push without explicit user approval.
- Never run destructive commands unless explicitly requested.
- Never delete files unless the task explicitly says cleanup/remove/delete.
- Do not run scripts unless `--allow-scripts` is passed or the user explicitly approves.
- Prefer small, reversible changes.
- If a command fails, inspect the artifact/log before retrying.
- Do not repeat the same failing command without changing something.

## EOF / Worker Drop Recovery

If `ptolemy-agent` or the worker connection drops with EOF, timeout, or no response, do not restart blindly and do not assume failure. First check git status and latest commit. If no commit exists but the expected task files are modified, continue using the deterministic fallback workflow in `WORKFLOWS.md`. Never stage files outside the task scope.

### JSON action rules for `ptolemy-agent`

When interacting with the local agent, Gemma must return exactly one JSON object per response.

Do not return multiple JSON objects.

Do not return top-level JSON arrays.

Do not chain multiple tool actions in one response.

If multiple steps are needed, return one `create_task_batch` object and let Ptolemy queue the child tasks separately.

Correct:

```json
{
  "action": "read_file",
  "path": "cmd/ptolemy-agent/main.go",
  "reason": "inspect current implementation"
}
```

Incorrect:

```json
{ "action": "read_file", "...": "..." }
{ "action": "replace_block", "...": "..." }
```

Queued multi-step form:

```json
{
  "action": "create_task_batch",
  "tasks": [
    {
      "type": "read_file",
      "path": "cmd/ptolemy-agent/main.go"
    },
    {
      "type": "run_command",
      "command": "go test ./..."
    }
  ]
}
```

### File editing rules

Prefer tools in this order:

1. `insert_after`
2. `replace_block`
3. `write_file`

Use `write_file` only for new small files or explicitly full-file replacement.

Do not rewrite large source files.

For source-code edits, prefer:
- `insert_after` when adding a helper/function/rule after a stable marker.
- `replace_block` only when the old text is exact and small.

### Terminal output rules

The agent terminal should show only:
- step number
- action
- reason
- status summary
- artifact path

Large content, raw JSON, command output, and error detail should go to `.state/agent-artifacts`.

## Current Phase Status

### Completed / mostly completed

- Phase 8: SQLite execution memory
- Phase 8.5: Markdown memory structure
- Phase 9: Local brain + basic policy + agent loop
- Phase 9.5 core tools:
  - `read_file`
  - `write_file`
  - `run_command`
  - `replace_block`
  - `insert_after`

### In progress

- Phase 9F: task runner pipeline
- Phase 9F currently being built in small steps:
  - 9F-4A: task-runner skeleton
  - 9F-4B: scan inbox
  - 9F-4C/4D: classify task size
  - 9F-4E: execute one task
  - later: commit and move to done/failed

## Current Immediate Problem

Codex reported:

```text
Post "http://localhost:8080/execute": EOF
```

This means `workerd` on port `8080` accepted the connection but closed it before returning a valid response.

Most likely causes:
- stale `workerd` process
- `workerd` crashed or panicked
- old binary/process still running
- `/execute` handler error
- port 8080 held by wrong process

## How to Verify Local Services

### Check worker health

```bash
curl -s http://localhost:8080/health | jq
```

Expected response:

```json
{
  "status": "ok",
  "service": "workerd"
}
```

### Check port 8080

```bash
sudo ss -ltnp '( sport = :8080 )'
```

### Restart worker

If using systemd:

```bash
sudo systemctl restart workerd
sudo systemctl status workerd
journalctl -u workerd -n 100 --no-pager
```

If running manually:

```bash
pkill workerd || true
go run ./cmd/workerd
```

### Smoke test `/execute`

```bash
SESSION_ID=$(curl -s -X POST http://localhost:8080/sessions/ \
  -H 'Content-Type: application/json' \
  -d '{"name":"local-test","workspace":"'"$PWD"'"}' | jq -r .id)

curl -s -X POST http://localhost:8080/execute \
  -H 'Content-Type: application/json' \
  -d '{
    "session_id":"'"$SESSION_ID"'",
    "command":"echo hello from ptolemy",
    "cwd":"'"$PWD"'",
    "reason":"smoke test",
    "timeout":30
  }' | jq
```

Expected output should include:

```text
hello from ptolemy
```

## Local LLM Startup

The local brain uses llama.cpp server on port `8088`.

Start it from:

```bash
cd ~/projects/llama.cpp
```

Correct model file is the main model:

```bash
./build/bin/llama-server \
  -m ~/.cache/huggingface/hub/models--ggml-org--gemma-4-E2B-it-GGUF/snapshots/*/gemma-4-E2B-it-Q8_0.gguf \
  -c 4096 \
  --port 8088
```

Do not use `mmproj-gemma-4-E2B-it-Q8_0.gguf` as the main model. That is the multimodal projector only.

Test:

```bash
curl -s http://127.0.0.1:8088/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemma-4-e2b",
    "messages": [{"role": "user", "content": "say ready"}]
  }' | jq
```

## Development Workflow for Codex

Before editing:

```bash
git status --short
go test ./...
```

For changes:
- make the smallest safe change
- run `gofmt`
- run `go test ./...`
- inspect `git diff`
- commit grouped changes with meaningful messages
- never push unless approved

Suggested commit message style:
- `feat(agent): ...`
- `fix(agent): ...`
- `feat(task-runner): ...`
- `docs(agent): ...`
- `chore: ...`

## Current Next Work Items

### 1. Stabilize `cmd/ptolemy-agent/main.go`

Check that it builds:

```bash
go test ./cmd/ptolemy-agent
go test ./...
```

If build fails, inspect the reported lines and fix only the broken block.

Known recent issue:
- parse recovery block may have accidentally inserted code in the wrong place
- possible error:
  - `undefined: artifactPath`

If seen, inspect around:

```bash
nl -ba cmd/ptolemy-agent/main.go | sed -n '140,180p'
```

The parse error handling should look like:

```go
action, err := parseBrainAction(reply)
if err != nil {
    summary := summarizeError(err, reply)
    artifactPath := saveArtifact(step, "brain-parse-error", reply)

    fmt.Printf(
        "%s\nartifact: %s\n",
        summary,
        artifactPath,
    )

    observations = append(observations, fmt.Sprintf(
        "Previous brain response was invalid JSON. Summary: %s. Artifact: %s. Return exactly ONE JSON object only. Do not return multiple JSON objects. Do not chain actions. Do one action now and continue later.",
        summary,
        artifactPath,
    ))

    continue
}

fmt.Printf("brain action: %s\n", action.Action)
```

### 2. Continue `cmd/ptolemy-task-runner`

Target behavior:

```text
go run ./cmd/ptolemy-task-runner
```

Should:
1. ensure task folders exist
2. scan `docs/tasks/inbox/*.md`
3. select first task
4. classify:
   - small = max steps 4
   - medium = max steps 8
   - large = split first
5. later execute task via `go run ./cmd/ptolemy-agent --task-file <file> --max-steps <n>`
6. on success:
   - commit related changes
   - move task to `docs/tasks/done`
7. on failure:
   - move task to `docs/tasks/failed`

Do this in small commits.

### 3. Do not over-broaden task files

Gemma E2B struggles when a task asks for many file edits at once.

Prefer:
- one task = one change
- deterministic scripts only when bootstrap is needed
- if script is needed, require explicit `--allow-scripts`

## Useful Commands

### Run tests

```bash
go test ./...
```

### Run agent

```bash
go run ./cmd/ptolemy-agent --task-file docs/tasks/<task>.md --max-steps 8
```

### Run agent with script permission

```bash
go run ./cmd/ptolemy-agent --allow-scripts --task-file docs/tasks/<task>.md --max-steps 3
```

### Run task runner

```bash
go run ./cmd/ptolemy-task-runner
```

### Check artifacts

```bash
tree .state/agent-artifacts
cat .state/agent-artifacts/<file>
```

### Clean artifacts

Artifacts older than 3 days should eventually be garbage-collected.

Manual cleanup:

```bash
find .state/agent-artifacts -type f -mtime +3 -delete
```

## Git Hygiene

Before committing:

```bash
git status --short
git diff --stat
git diff --name-only
go test ./...
```

Avoid committing:
- `.state/`
- `state/*.db`
- temp files like `tmp-*.txt`
- accidental task scratch files unless intended

## Important Design Decision

Use Go-native skills for permanent capabilities.

Use Python scripts only as temporary bootstrap/migration helpers.

Preferred long-term skill architecture:

```text
internal/skills/
├── edit/
│   ├── insert_after.go
│   ├── replace_block.go
│   ├── apply_patch.go
│   └── read_range.go
├── git/
├── taskrunner/
└── policy/
```

## Definition of Done for Current Milestone

This milestone is done when:

- `go test ./...` passes
- `workerd` health endpoint works
- `/execute` smoke test works
- `ptolemy-agent` can run a simple task file
- `ptolemy-task-runner` can scan inbox tasks
- no accidental temp files are staged
- commits are grouped by task
