# Task Pack Template

Use this template when a change needs more structure than a single loose inbox task.

This template is an execution contract, not just planning prose. A generated pack should tell Ptolemy:

- what the pack is trying to complete
- which tasks run in which order
- what each task may edit
- which exact pack assets the task may use
- how success is validated
- when execution must stop or ask for help

## Layout

```text
task-pack-name/
|-- TASK_PLAN.md
|-- PACK_MANIFEST.yaml
|-- README.md
|-- scripts/
|-- task-scripts/
|-- snippets/
`-- inbox/
```

## Contract rules

- `TASK_PLAN.md` is pack-level orchestration only.
- `PACK_MANIFEST.yaml` is the machine-readable pack contract.
- `inbox/*.md` files are task-level execution contracts.
- `task-scripts/` and `snippets/` are exact referenced inputs for a task.
- `scripts/` may hold pack assets, but Ptolemy does not auto-run shell hooks from this folder in v1.
- Each task must include the required Markdown sections from `inbox-task-template.md`.
- Each strict contract pack should set `agent_mode: bounded_markdown_contract`.
