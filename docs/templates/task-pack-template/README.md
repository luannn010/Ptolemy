# Task Pack Template

Use this template when a change needs more structure than a single loose inbox task.

## Layout

```text
task-pack-name/
├── TASK_PLAN.md
├── PACK_MANIFEST.yaml
├── README.md
├── scripts/
├── task-scripts/
├── snippets/
└── inbox/
```

## Notes

- `TASK_PLAN.md` explains the intended execution order and pack goal.
- `PACK_MANIFEST.yaml` provides machine-readable pack metadata.
- `inbox/` contains the actual task files Ptolemy executes.
- `task-scripts/` and `snippets/` are validated references in v1.
- `scripts/` exists for pack assets, but Ptolemy does not auto-run pack shell hooks in v1.
