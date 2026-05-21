# Setup

## Requirements

- Go 1.25 or newer, matching `go.mod`
- Make
- tmux
- ripgrep (`rg`)
- Git
- Optional: `jq` for smoke-test output formatting
- Optional: llama.cpp server for `ptolemy-agent` local brain mode

## Initial Setup

```bash
cp .env.example .env
go mod tidy
```

Default environment values:

```env
APP_ENV=development
HTTP_PORT=8080
LOG_LEVEL=debug
STATE_DIR=./state
DB_PATH=./state/ptolemy.db
WORKER_BASE_URL=http://127.0.0.1:1088
BRAIN_BASE_URL=http://127.0.0.1:1090
BRAIN_MODEL=gemma-4-e2b
PTOLEMY_AGENT_BIN=
```

## Common Commands

```bash
make run
make build
make build-mcp
make test
make test-integration
make fmt
make tidy
```

You can also run the main binaries directly:

```bash
go run ./cmd/workerd
go run ./cmd/ptolemy-mcp
go run ./cmd/ptolemy-agent --workspace . --task-file docs/tasks/<task>.md --max-steps 8
go run ./cmd/ptolemy-task-runner
go run ./cmd/ptolemy-task-runner bootstrap --workspace /path/to/target-repo
```

When you want to point Ptolemy at another repository:

- start `workerd`
- set `WORKER_BASE_URL`, `BRAIN_BASE_URL`, and `BRAIN_MODEL` if you are not using the defaults
- run `ptolemy-agent --workspace /path/to/repo ...` to target that repository explicitly
- set `PTOLEMY_AGENT_BIN` for `ptolemy-task-runner` when the agent binary is not already on `PATH`
