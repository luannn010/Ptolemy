# Agent Recovery Workflow: EOF / Worker Connection Drop

Recover safely when `ptolemy-agent` or the worker connection drops with `EOF`, timeout, or no response during a task.

Rules:

1. Do not assume the task failed just because the worker returned `EOF`.
2. Check whether a commit was created:

```bash
git log -1 --oneline
git status --short
```

3. If a commit exists, inspect it:

```bash
git show --stat --oneline HEAD
```

4. If no commit exists, verify the working tree only contains task-file paths.
5. Run deterministic task instructions manually through the same Ptolemy session.
6. Run required tests before committing.
7. Stage only task-file paths. Never use `git add .`.
8. Verify the index:

```bash
git diff --cached --name-only
```

9. Commit only if tests pass and the index contains only expected files.
10. Report:

- `EOF` occurred.
- Whether tests passed.
- Whether a commit was created.
- Whether fallback was used.
- Whether scope stayed clean.

Status: required fallback when the agent or worker drops before a task result is returned.
