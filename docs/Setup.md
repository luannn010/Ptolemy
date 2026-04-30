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
go run ./cmd/ptolemy-agent --task-file docs/tasks/<task>.md --max-steps 8
go run ./cmd/ptolemy-task-runner
```
