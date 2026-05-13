# Task Plan: <Pack Name>

## Goal

Describe the final end state this pack should reach.

Include:

- what will change
- why it matters
- what success looks like
- how the result can be validated

Example:

> Ptolemy should support OS-aware command execution. Linux/macOS should use `bash -lc`, while Windows should use PowerShell. The pack is complete when command execution works on both operating systems and tests pass.

## Execution Strategy

- Keep tasks small and deterministic.
- Prefer one task per behavior slice or file group.
- Validate as early as practical.
- Read only the files needed for the current task.
- Avoid broad refactors unless the task explicitly requires them.
- Reserve the final task for pack-wide validation and documentation sync.

## Monitoring Expectations

The Pack Studio run monitor expects:

- each task to have stable `task_id` metadata
- explicit `allowed_files`
- `validation` commands when behavior changes
- a `## Checklist` section with Markdown checkboxes

While the pack is running, the UI will show:

- the pack and task tree
- current progress and checklist state
- recent actions and observations
- the live tmux terminal for the current task session

## Execution Order

1. `01-discover-context.md`
2. `02-implement-core.md`
3. `03-add-validation.md`
4. `99-finalize-pack.md`

Optional tasks can be inserted in the middle, but the plan should stay sequential and easy to follow.

## Documentation Rule

When a completed task changes documented behavior, commands, setup, workflow, API behavior, or user-facing expectations, add or run a documentation task before the pack is considered complete.
- `.ptolemy/context/conventions.md`

---

## Global Validation

Use the narrowest command that proves the pack is complete.

Default:

```bash
go test ./...
```

Replace this with a narrower command when possible.

Examples:

```bash
go test ./internal/executor/...
go test ./internal/httpapi/...
go test ./cmd/workerd/...
```

---

## Final Pull Request

After all tasks are merged into the pack branch, raise one Pull Request.

PR source branch:

```text
feature/<ddmmyy>-<pack-name>
```

PR target branch:

```text
main
```

The PR description should include:

- pack goal
- task list completed
- files changed
- validation commands
- documentation updates
- known risks or follow-up work
