# Harden Edit Action Validation Pack

This task pack patches `ptolemy-agent` so malformed edit actions are handled safely.

## Problem

The model sometimes emits incomplete edit actions such as a `replace_block` without a target file, old block, anchor, or replacement text. Previously, those actions could reach execution and burn an agent step without changing the repository safely.

## Desired Outcome

Incomplete edit actions must be rejected before execution and converted into a corrective prompt that asks the model to return one complete JSON action.

## Pack Contents

```text
05-harden-edit-action-validation/
|-- TASK_PLAN.md
|-- PACK_MANIFEST.yaml
|-- README.md
|-- scripts/
|-- task-scripts/
|   `-- harden-edit-action-validation.md
|-- snippets/
|   `-- edit-action-validation-rules.md
`-- inbox/
    `-- 05-harden-edit-action-validation.md
```

## Suggested Run

Copy this pack into your Ptolemy task packs folder, then run the inbox task through Ptolemy from the repository root.

Example:

```bash
cp -R 05-harden-edit-action-validation docs/tasks/packs/
./bin/ptolemy run docs/tasks/packs/05-harden-edit-action-validation/inbox/05-harden-edit-action-validation.md
```

If your binary path is different, use your existing Ptolemy run command.
