# Task Script: Smoke Tests

Run and record:

1. `python scripts/mcp/ptolemy_mcp.py --self-test`
2. `curl -s http://127.0.0.1:8080/health`
3. `/usr/local/go/bin/go test ./...`
4. `go run ./cmd/ptolemy-task-runner plan --pack docs/tasks/packs/ptolemy-mcp-wrapper-pack`

If the full pack runner would push or create a PR without explicit approval, stop there and keep the fallback note in `scripts/fallback-note.md`.
