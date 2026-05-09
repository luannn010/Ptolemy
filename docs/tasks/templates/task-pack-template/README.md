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
- Tasks inside a pack execute sequentially.
- The live terminal is streamed from the tmux-backed worker session.

## Monitoring

After a pack or program is created, use the embedded UI under `/ui`:

- `Studio` creates packs and programs.
- `Overview` shows catalog and run history.
- `Runs` shows the tree, checklist progress, recent actions, and live terminal output.
