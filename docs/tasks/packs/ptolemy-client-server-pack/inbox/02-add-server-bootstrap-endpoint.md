---
priority: normal
task_id: 02-add-server-bootstrap-endpoint
parent_task: ptolemy-client-server-pack
owner: unassigned
status: inbox
branch: ptolemy/02-add-server-bootstrap-endpoint
execution_group: sequential
depends_on: ['01-add-server-skill-registry']
allowed_files:
  - internal/bootstrap/
  - internal/httpapi/
  - cmd/workerd/
validation:
  - go test ./...
scripts:
  - task-scripts/02-server-bootstrap-endpoint.md
snippets:
  - snippets/client-init-tree.txt
  - snippets/client-yaml.example
created_by: chatgpt
---

# Task: Add server bootstrap endpoint

## Goal
Implement this step of the Ptolemy client-server architecture without Docker dependency.

## Scope
Only modify files listed in `allowed_files`.

## Inputs
Use these pack files:

- `task-scripts/02-server-bootstrap-endpoint.md`
- `snippets/client-init-tree.txt`
- `snippets/client-yaml.example`

## Required behavior
Follow the linked task script exactly. Keep changes small, testable and compatible with the existing Ptolemy server codebase.

## File operations
1. Create or update only the allowed files.
2. Preserve unrelated code.
3. Add tests for new behavior where possible.

## Validation

```bash
go test ./...
```

## Done when

- [ ] File changes are complete
- [ ] Validation passes
- [ ] Only allowed files changed
- [ ] Task can be moved from `inbox/` to `done/`
