# Task Plan: Ptolemy Execute Metadata

## Goal

Make the Codex-triggered `ptolemy.execute -> /execute` path accept and persist descriptive metadata so live executions can store `title`, `purpose`, `reasoning_step`, and `target` without breaking older callers.

## Validation

```bash
/usr/local/go/bin/go test ./internal/mcp/executortools ./internal/executor ./internal/httpapi
/usr/local/go/bin/go test ./...
/usr/local/go/bin/go build ./cmd/workerd
```
