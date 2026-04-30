# Ptolemy Client-Server Implementation Pack

This pack implements a non-Docker client/server model for Ptolemy.

Use it as an execution contract pack:

- `PACK_MANIFEST.yaml` defines the machine-readable pack settings
- `TASK_PLAN.md` defines pack-level ordering and stop rules
- `inbox/*.md` files define task-level execution contracts
- `task-scripts/` and `snippets/` are exact referenced inputs for bounded tasks

## Target design

- Current Ptolemy repo remains the **server**.
- Each target codebase gets a lightweight **client** through `.ptolemy/` local state.
- `.ptolemy/` should be ignored by git in each target codebase.
- The client runs commands inside the target project's real Linux/Unix dev environment.
- The client can fetch skills/templates/workflows from the server and cache them locally.

## Run strategy

Execute tasks in `inbox/` order using sequential execution and stop on the first failed validation.
