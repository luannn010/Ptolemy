# Task Plan: Ptolemy Client-Server Implementation

## Goal
Implement a non-Docker Ptolemy client/server architecture where the current repo remains the server and a lightweight client can run inside other codebases.

## Execution Strategy
Sequential-first. Do not run final integration until tasks `01` through `09` are complete.

## Global Constraints

- Do not require Docker for any task in this pack.
- Keep edits within each task's `allowed_files`.
- Stop on the first failed validation.
- Treat `task-scripts/` and `snippets/` as exact referenced inputs.

## Execution Order

### Phase 1 - Server foundations
1. `01-add-server-skill-registry.md`
2. `02-add-server-bootstrap-endpoint.md`

### Phase 2 - Client foundations
3. `03-add-client-config-and-init.md`
4. `04-add-client-workspace-guard.md`
5. `05-add-client-file-operations.md`
6. `06-add-client-command-executor.md`

### Phase 3 - Client/server integration
7. `07-add-client-skill-sync.md`
8. `08-add-client-codebase-scan.md`

### Phase 4 - Documentation and validation
9. `09-add-client-server-workflow-docs.md`
10. `99-final-integration.md`

## Global Validation

```bash
go test ./...
go build ./cmd/workerd
go build ./cmd/ptolemy-client
```

## Completion Policy

This pack is complete only when all required tasks are completed and the global validation commands pass.

## Failure Rule

If any task fails validation, stop immediately and mark the task as failed. Do not continue to the next task until the issue is fixed.
