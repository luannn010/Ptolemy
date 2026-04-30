# Multi-Task Execution

## 1. Current MVP: dependency-aware sequential multi-task runner

The current scheduler scans inbox tasks, checks dependencies, filters conflicts, and executes selected tasks sequentially.

## 2. Why true parallel execution needs worktrees

Parallel execution needs branch and filesystem isolation so tasks do not overwrite each other in one checkout.

## 3. Worktree layout example

Each runnable task branch gets its own worktree directory:

```bash
git worktree add ../ptolemy-worktrees/add-git-status ptolemy/add-git-status
git worktree add ../ptolemy-worktrees/add-queue-store ptolemy/add-queue-store
```

## 4. File conflict rules using allowed_files

Tasks can run together only when `allowed_files` sets do not overlap exactly.

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
