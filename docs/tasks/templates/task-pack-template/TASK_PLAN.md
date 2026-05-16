# Task Plan: <Pack Name>

## Pack Location

Create this pack under the date-based task pack root:

```text
.ptolemy/tasks/packs/<dd-mm-yyyy>/<pack-id>/
```

Example:

```text
.ptolemy/tasks/packs/15-05-2026/ptolemy-cli-feature/
```

Use the same `<pack-id>` in:

- the folder name
- `PACK_MANIFEST.yaml` `pack_id`
- process state paths under `.ptolemy/tasks/process/<pack-id>/`
- branch names when the pack or its tasks need git isolation

## Required File Structure

The pack must use this structure:

```text
<dd-mm-yyyy>/
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

## Goal

Describe the final state this pack should reach.

Include:

- what will change
- why it matters
- what success looks like
- how the result can be validated

Example:

> Ptolemy should support OS-aware command execution. Linux/macOS should use `bash -lc`, while Windows should use PowerShell. The pack is complete when command execution works on both operating systems and tests pass.

## Manifest Requirements

`PACK_MANIFEST.yaml` must match the schema supported by `internal/tasks/pack.go`.

Required runtime fields:

- `pack_id`
- `name`
- `entrypoint`
- `folders`
- `execution_mode`
- `requires`
- `validation`
- `rules`

The `folders` section should point to the pack-local folders:

- `inbox/`
- `snippets/`
- `task-scripts/`
- `scripts/`

## Folder Responsibilities

`PACK_MANIFEST.yaml`

- Defines pack metadata, execution mode, folder paths, validation, and rules.
- Should be the single source of truth for Pack Studio runtime setup.

`README.md`

- Explains the pack purpose, scope, setup notes, and human-facing context.
- Should stay concise enough to be useful inside Pack Studio.

`TASK_PLAN.md`

- Defines execution order, validation strategy, branch expectations, and completion rules.
- Should be updated if the task sequence changes.

`inbox/`

- Contains the task files Pack Studio should execute.
- Tasks should be narrow, sequential, and numbered by intended order.

`snippets/`

- Contains reusable prompt/context snippets referenced by tasks.
- Keep snippets small and specific.

`task-scripts/`

- Contains scripts intended to be referenced by individual task files.
- Scripts must not run unless the pack or task explicitly allows scripts.

`scripts/`

- Contains pack-level helper scripts for setup, validation, or maintenance.
- Prefer Go-native implementation for permanent behavior.

## Inbox Task Order

Default task sequence:

1. `inbox/01-discover-context.md`
2. `inbox/02-implement-core.md`
3. `inbox/03-add-validation.md`
4. `inbox/99-finalize-pack.md`

Optional tasks may be inserted between `02-implement-core.md` and `99-finalize-pack.md`.

Recommended phases:

- `01-discover-context.md`: inspect only, identify files, confirm constraints
- `02-implement-core.md`: make the smallest behavior change
- `03-add-validation.md`: add or update tests and validation commands
- `99-finalize-pack.md`: run pack-wide validation, sync docs, summarize outcome

## Task File Requirements

Each inbox task must include YAML frontmatter with at least:

- `task_id`
- `priority`
- `parent_task`
- `owner`
- `status`
- `branch`
- `allowed_files`
- `validation`
- `created_by`

Pack Studio may also write:

- `execution_group`
- `depends_on`
- `validation`
- `scripts`
- `snippets`

Each task must include a `## Checklist` section with Markdown checkboxes.

Keep each task narrow:

- one goal
- one phase
- one small `allowed_files` scope
- at most 3-5 explicit steps
- about 1200-2000 chars when possible

Tasks above 4000 chars should usually be split before execution.

## Execution Model

Pack Studio runs the pack sequentially through `agent-runs`.

Expected runtime flow:

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

Large tasks should be split into child tasks before execution or during the process flow. Child tasks should use compact manifest state and result summaries rather than full prior chat history.

## Process State

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

`manifest.json` tracks task status, dependencies, changed files, commands run, and failure reasons.

`todo.md` is the human-readable checklist view.

Each result file should be compact enough to carry forward as memory for the next task.

## Branching Strategy

Use the task metadata `branch` field as the source of truth for task branches.

When the whole pack needs isolation, use a pack branch:

```text
ptolemy/<pack-id>
```

For task-specific branches, use each task's `branch` value. A typical pattern is:

```text
ptolemy/<task-id>
```

No task should commit directly to the parent branch. Stage explicit files only, and do not use `git add .`.

## Validation

Use the narrowest command that proves the pack is complete.

Default:

```bash
go test ./...
```

Replace this with narrower commands when possible:

```bash
go test ./internal/tasks/...
go test ./internal/packstudio/...
go test ./cmd/ptolemy-task-runner/...
```

Each task should include its own `validation` command in frontmatter so runner validation can fail fast before execution.

## Documentation Rule

When a completed task changes documented behavior, commands, setup, workflow, API behavior, or user-facing expectations, add or run a documentation task before the pack is considered complete.

Relevant docs may include:

- `README.md`
- `WORKFLOWS.md`
- `docs/workflows/agent/task-pack-execution.md`
- files under `.ptolemy/kb/` when the change affects local knowledge

## Monitoring

Use the embedded UI under `/ui`:

- `Studio` creates packs and programs
- `Overview` shows catalog and run history
- `Runs` shows the program, pack, task tree, checklist progress, recent actions, and live terminal output

The run monitor expects:

- stable `task_id` metadata
- explicit `allowed_files`
- validation commands when behavior changes
- a `## Checklist` section in each task

## Final Pull Request

Create one pull request for the completed pack if PR creation is requested.

The PR should include:

- pack goal
- completed task list
- files changed
- validation commands
- documentation updates
- known risks or follow-up work

Follow:

- `.github/pull_request_template.md`
- `docs/workflows/git/pull-request.md`

Do not push or open a PR without explicit user approval.

## Completion Checklist

- [ ] Pack is under `.ptolemy/tasks/packs/<dd-mm-yyyy>/<pack-id>/`
- [ ] `PACK_MANIFEST.yaml` matches the runtime schema
- [ ] `README.md` describes pack scope and usage
- [ ] `TASK_PLAN.md` matches this file structure
- [ ] `inbox/` contains ordered task files
- [ ] every task has valid metadata and `allowed_files`
- [ ] every task has a `## Checklist`
- [ ] validation commands are defined
- [ ] docs are updated when behavior changes
- [ ] process state can be monitored under `.ptolemy/tasks/process/<pack-id>/`
- [ ] when complete, move the pack to `/.ptolemy/resolve/`
