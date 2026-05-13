# Task Pack Template

Use this folder when one implementation should be broken into several deterministic tasks that Ptolemy can create, run, and monitor from the embedded Pack Studio UI.

## Runtime Shape

`PACK_MANIFEST.yaml` must match the schema supported by `internal/tasks/pack.go`.
The important fields are:

- `pack_id`
- `name`
- `entrypoint`
- `folders`
- `execution_mode`
- `requires`
- `validation`
- `rules`

The checked-in template now matches the runtime manifest used by Pack Studio.

## Suggested Structure

```text
<pack-id>/
├── PACK_MANIFEST.yaml
├── README.md
├── TASK_PLAN.md
├── inbox/
│   ├── 01-discover-context.md
│   ├── 02-implement-core.md
│   ├── 03-add-validation.md
│   └── 99-finalize-pack.md
├── snippets/
├── task-scripts/
└── scripts/
```

## Task Authoring Rules

Each task file should have YAML frontmatter with at least:

- `task_id`
- `priority`
- `parent_task`
- `owner`
- `status`
- `branch`
- `allowed_files`
- `created_by`

Pack Studio also writes:

- `execution_group`
- `depends_on`
- `validation`
- `scripts`
- `snippets`

Each task should include a `## Checklist` section with Markdown checkboxes so the run monitor can render explicit progress. If a legacy task does not include a checklist, the UI falls back to status-derived checklist items.

## Execution Model

Pack Studio runs packs sequentially through `agent-runs`.

- One program run is active at a time in v1.
- Packs inside a program run execute sequentially.
- Large parent tasks are split into narrow child tasks before execution.
- Each child task runs as a fresh brain call with compact manifest state instead of full prior chat history.
- Tasks inside a pack execute sequentially.
- The live terminal is streamed from the tmux-backed worker session.

## Large Task Guidance

Keep each child task small enough for the 24K local model context window.

- Comfortable child task body: about 1200-2000 chars.
- Risky child task body: above 4000 chars.
- Prefer one narrow goal, one phase, one small `allowed_files` scope, and at most 3-5 explicit steps.
- Split inspect/research, plan, implementation, tests, validation, and final summary into separate child tasks when possible.

## Process State

During execution, Ptolemy writes process state under:

```text
.ptolemy/tasks/process/<pack-id>/
├── manifest.json
├── todo.md
└── state/
    ├── 001-...-result.md
    ├── 002-...-result.md
    └── ...
```

`manifest.json` tracks child-task status, dependencies, changed files, commands run, and failure reasons.
`todo.md` is the human-readable checklist view.
Each child result summary is compact and becomes the memory carried into the next child task.

## Monitoring

After a pack or program is created, use the embedded UI under `/ui`:

- `Studio` creates packs and programs.
- `Overview` shows catalog and run history.
- `Runs` shows the tree, checklist progress, recent actions, and live terminal output.
