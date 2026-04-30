# Multi-Task Execution

## 1. Current MVP: deterministic sequential task scheduler

The current scheduler can:

- scan inbox tasks from Markdown frontmatter
- load task packs and scan pack `inbox/` tasks in place
- validate task metadata before execution
- build a deterministic plan from dependencies, priority, and execution group
- check `allowed_files` conflicts
- run validation commands sequentially
- update task status to `running`, `completed`, or `failed`
- stop at the first failed task

## 2. Why true parallel execution needs worktrees

Parallel execution needs branch and filesystem isolation so tasks do not overwrite each other in one checkout.

## 3. Worktree layout example

Each runnable task branch gets its own worktree directory:

```bash
git worktree add ../ptolemy-worktrees/add-git-status ptolemy/add-git-status
git worktree add ../ptolemy-worktrees/add-queue-store ptolemy/add-queue-store
```

## 4. File conflict rules using allowed_files

Tasks can run together only when cleaned `allowed_files` paths do not overlap.

Current helpers:

- `FindAllowedFileConflicts(tasks []Task) []FileConflict`
- `CanRunTogether(tasks []Task) bool`

## 5. Safe merge sequence

Merge completed task branches one at a time in dependency order. Re-run validation after each merge.

## 6. Conflict handling strategy

If merge conflicts happen, stop automation, log conflicting files, and require manual resolution before continuing.

## 7. When to require human approval

Require approval when:

- dependencies are unclear
- `allowed_files` overlap unexpectedly
- branch/worktree setup fails
- merges produce conflicts

True parallel execution should only run tasks together when dependencies are completed, `allowed_files` do not overlap, each task has its own branch, and each task has its own worktree.

## 8. Current CLI entrypoints

Preview plan:

```bash
go run ./cmd/ptolemy-task-runner plan --inbox docs/tasks/inbox
go run ./cmd/ptolemy-task-runner plan --pack <pack-dir>
```

Run sequential scheduler:

```bash
go run ./cmd/ptolemy-task-runner run --inbox docs/tasks/inbox --workspace .
go run ./cmd/ptolemy-task-runner run --pack <pack-dir> --workspace .
```

Pack behavior in v1:

- the runner reads `PACK_MANIFEST.yaml` and `TASK_PLAN.md`
- the runner executes the pack's `inbox/` tasks directly from the pack directory
- referenced `task-scripts/` and `snippets/` files are validated for existence
- pack `scripts/` hooks are not auto-run

These commands operate on task metadata and validation commands only. They do not create branches, worktrees, or true parallel execution yet.
