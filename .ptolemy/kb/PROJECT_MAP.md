# Project Map

## Purpose
Ptolemy is a local worker and agent execution system.

## Main areas
- `cmd/`
- `internal/`
- `docs/`
- `configs/`
- `deploy/`
- `cmd/workerd`: worker daemon entrypoint
- `cmd/ptolemy-agent`: local agent entrypoint
- `cmd/ptolemy-task-runner`: task-pack and inbox runner
- `internal/navigator`: repo memory and KB builder
- `internal/tasks`: task and task-pack execution runtime
