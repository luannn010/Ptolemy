# Task Plan: Ptolemy Agent KB Hooks

## Goal

Read `.ptolemy/kb/` before agent workspace inspection and update the KB once per successful task pack.

## Validation

```bash
/usr/local/go/bin/go test ./cmd/ptolemy-agent ./internal/tasks
```
