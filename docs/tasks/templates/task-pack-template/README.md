# Task Pack Report: <Pack Name>

Use this README as the human-facing report for a task pack. It should explain what the pack is for, where it lives, what it will run, and where Ptolemy writes runtime progress.

## Pack Summary

Pack ID: `<pack-id>`

Date folder: `<dd-mm-yyyy>`

Pack path:

```text
.ptolemy/tasks/packs/<dd-mm-yyyy>/<pack-id>/
```

Process state path:

```text
.ptolemy/tasks/process/<pack-id>/
```

Goal:

> Describe the outcome this pack should produce.

Success criteria:

- [ ] behavior or documentation change is complete
- [ ] task checklist items are complete
- [ ] validation commands pass
- [ ] task results are visible in Pack Studio Runs
- [ ] any required documentation is updated

## File Structure

Each task pack must use this date-based structure:

```text
.ptolemy/tasks/packs/
`-- <dd-mm-yyyy>/
    `-- <pack-id>/
        |-- PACK_MANIFEST.yaml
        |-- README.md
        |-- TASK_PLAN.md
        |-- inbox/
        |   |-- 01-discover-context.md
        |   |-- 02-implement-core.md
        |   |-- 03-add-validation.md
        |   `-- 99-finalize-pack.md
        |-- snippets/
        |-- task-scripts/
        `-- scripts/
```

## Pack Files

`PACK_MANIFEST.yaml`

- Runtime manifest used by Pack Studio and task-runner.
- Must match the schema supported by `internal/tasks/pack.go`.
- Defines `pack_id`, `name`, `entrypoint`, folders, validation, and rules.

`README.md`

- This human-facing pack report.
- Should summarize purpose, scope, progress, validation, and known risks.

`TASK_PLAN.md`

- Execution plan for the pack.
- Defines task order, validation strategy, branch expectations, and completion rules.

`inbox/`

- Ordered task files that Pack Studio executes.
- Tasks should stay narrow and sequential.

`snippets/`

- Reusable context snippets referenced by tasks.

`task-scripts/`

- Task-specific helper scripts.
- Scripts require explicit permission before execution.

`scripts/`

- Pack-level helper scripts for setup, validation, or maintenance.

## Manifest Fields

The runtime manifest should define:

- `pack_id`
- `name`
- `entrypoint`
- `folders`
- `execution_mode`
- `requires`
- `validation`
- `rules`

The `folders` values should point to:

- `inbox/`
- `snippets/`
- `task-scripts/`
- `scripts/`

## Task Report

Pack Studio reports progress from each inbox task.

Each task file should include:

- YAML frontmatter with stable metadata
- explicit `allowed_files`
- validation commands when behavior changes
- a `## Checklist` section with Markdown checkboxes

Required task metadata:

- `task_id`
- `priority`
- `parent_task`
- `owner`
- `status`
- `branch`
- `allowed_files`
- `created_by`

Pack Studio may also write:

- `execution_group`
- `depends_on`
- `validation`
- `scripts`
- `snippets`

## Execution Report

Pack execution is sequential in v1:

```text
program run
-> pack
-> inbox task
-> optional child task split
-> fresh agent context
-> validation
-> compact result summary
-> next task
```

Large tasks may be split into child tasks. Child tasks should stay in one phase only:

- `inspect`
- `plan`
- `edit`
- `test`
- `validate`
- `docs`
- `finalize`

Keep child tasks small enough for the local model context:

- comfortable: about 1200-2000 chars
- risky: above 4000 chars

## Runtime Report Files

During execution, Ptolemy writes process state under:

```text
.ptolemy/tasks/process/<pack-id>/
|-- manifest.json
|-- todo.md
`-- state/
    |-- 001-...-result.md
    |-- 002-...-result.md
    `-- ...
```

`manifest.json` reports:

- child-task status
- dependencies
- changed files
- commands run
- validation results
- failure reasons

`todo.md` reports:

- the human-readable checklist view
- current progress
- remaining work

`state/*-result.md` reports:

- compact child-task summaries
- files changed by that child task
- commands run by that child task
- observations needed by the next child task

## Monitoring

Use the embedded UI under `/ui`:

- `Studio` creates packs and programs
- `Overview` shows catalog and run history
- `Runs` shows the program, pack, task tree, checklist progress, recent actions, and live terminal output

When debugging a failed run, inspect the process state files before rerunning. Prefer a fresh rerun after fixing the blocker.

## Validation

Default validation:

```bash
go test ./...
```

Use narrower validation when possible:

```bash
go test ./internal/tasks/...
go test ./internal/packstudio/...
go test ./cmd/ptolemy-task-runner/...
```

Record the final validation command and result here:

```text
Command: <validation command>
Result: <pass/fail/not run>
Notes: <short summary>
```

## Known Risks

- <risk or follow-up>

## Final Status

Status: `<draft | ready | running | done | failed>`

Resolve location for completed packs:

```text
/.ptolemy/resolve/
```

When all inbox tasks are done and validation passes, move the completed pack to `/.ptolemy/resolve` so future runs can confirm it is already resolved.

Completed tasks:

- [ ] `inbox/01-discover-context.md`
- [ ] `inbox/02-implement-core.md`
- [ ] `inbox/03-add-validation.md`
- [ ] `inbox/99-finalize-pack.md`

Final notes:

> Summarize the completed result, validation evidence, documentation updates, and any follow-up work.
