---
priority: normal
task_id: 03-add-client-config-and-init
parent_task: ptolemy-client-server-pack
owner: unassigned
status: inbox
branch: ptolemy/03-add-client-config-and-init
execution_group: sequential
depends_on: ['02-add-server-bootstrap-endpoint']
allowed_files:
  - cmd/ptolemy-client/
  - internal/client/config/
  - internal/client/init/
validation:
  - go test ./...
  - go build ./cmd/ptolemy-client
scripts:
  - task-scripts/03-client-config-init.md
snippets:
  - snippets/client-init-tree.txt
  - snippets/client-yaml.example
  - snippets/gitignore-entry.txt
created_by: chatgpt
---

# Task: Add client config and init command

## Goal
Implement this step of the Ptolemy client-server architecture without Docker dependency.

## Scope
Only modify files listed in `allowed_files`.

## Inputs
Use these pack files:

- `task-scripts/03-client-config-init.md`
- `snippets/client-init-tree.txt`
- `snippets/client-yaml.example`
- `snippets/gitignore-entry.txt`

## Required behavior
Follow the linked task script exactly. Keep changes small, testable and compatible with the existing Ptolemy server codebase.

## File operations
1. Create or update only the allowed files.
2. Preserve unrelated code.
3. Add tests for new behavior where possible.

## Validation

```bash
go test ./... && go build ./cmd/ptolemy-client
```

## Done when

- [ ] File changes are complete
- [ ] Validation passes
- [ ] Only allowed files changed
- [ ] Task can be moved from `inbox/` to `done/`
