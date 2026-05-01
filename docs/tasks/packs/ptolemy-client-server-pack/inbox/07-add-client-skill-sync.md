---
priority: normal
task_id: 07-add-client-skill-sync
parent_task: ptolemy-client-server-pack
owner: unassigned
status: inbox
branch: ptolemy/07-add-client-skill-sync
execution_group: sequential
max_steps: 8
requires_approval: false
stop_on_error: true
depends_on:
  - 06-add-client-command-executor
allowed_files:
  - internal/client/skillsync/
  - cmd/ptolemy-client/
validation:
  - go test ./internal/client/...
scripts:
  - task-scripts/07-client-skill-sync.md
snippets:
  - snippets/client-yaml.example
created_by: chatgpt
---

# Task: Add client skill sync

## Goal
Implement this step of the Ptolemy client-server architecture without Docker dependency.

## Scope
Only modify files listed in `allowed_files`.

## Constraints

- Do not edit files outside `allowed_files`.
- Preserve unrelated code and user changes.
- Read the server URL from `.ptolemy/client.yaml`.
- Fail gracefully if the server is offline or missing.

## Inputs
Use these pack files:

- `task-scripts/07-client-skill-sync.md`
- `snippets/client-yaml.example`

## Execution Steps
1. Read the linked task script and referenced snippets before editing.
2. Implement skill-list retrieval from the server and selected skill downloads into cache.
3. Add `ptolemy-client sync-skills` and `ptolemy-client skills list`.
4. Handle offline or failing server responses gracefully.
5. Add focused tests and run the listed validation commands.

## Acceptance Checks

- `go test ./internal/client/...`
- CLI can list skills and sync them into `.ptolemy/cache/skills`

## Failure / Escalation

- Stop if the change requires editing files outside `allowed_files`.
- Stop if server API changes are required outside current pack scope.
- Stop if validation fails and the issue is not clearly within task scope.

## Done when

- [ ] File changes are complete
- [ ] Validation passes
- [ ] Only allowed files changed
- [ ] Task can be moved from `inbox/` to `done/`
