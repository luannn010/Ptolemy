# Task Plan: <Pack Name>

## Goal

Describe the final end state this pack should reach.

Include:

- what will change
- why it matters
- what success looks like
- how the result can be validated

Example:

> Ptolemy should support OS-aware command execution. Linux/macOS should use `bash -lc`, while Windows should use PowerShell. The pack is complete when command execution works on both operating systems and tests pass.

---

## Execution Strategy

- Keep tasks small and deterministic.
- Prefer one task per behavior slice or file group.
- Validate as early as practical.
- Read only the files needed for the current task.
- Avoid broad refactors unless the task explicitly requires them.
- Reserve the final task for pack-wide validation, documentation sync, and PR readiness.

---

## Branching Strategy

### Pack Branch

Create the pack branch first:

```bash
git checkout -b feature/<ddmmyy>-<pack-name>
```

Example:

```bash
git checkout -b feature/010526-os-aware-executor
```

### Task Branches

Each task should be completed on its own task branch:

```bash
git checkout -b feature/<ddmmyy>-<pack-name>/task-01
```

Example:

```bash
git checkout -b feature/010526-os-aware-executor/task-01
```

### Merge Rule

After each task is complete and validated:

```bash
git checkout feature/<ddmmyy>-<pack-name>
git merge --no-ff feature/<ddmmyy>-<pack-name>/task-01
```

Then continue with the next task branch.

---

## If a Task Is Too Big

If any task becomes too large, split it into smaller task branches.

Example:

```text
feature/<ddmmyy>-<pack-name>/task-02a
feature/<ddmmyy>-<pack-name>/task-02b
feature/<ddmmyy>-<pack-name>/task-02c
```

Each split branch must still be validated before merging into the pack branch.

---

## Execution Order

1. `01-discover-context.md`
2. `02-implement-core.md`
3. `03-add-validation.md`
4. `04-refactor-cleanup.md`
5. `05-update-docs.md`
6. `06-add-follow-up-tests.md`
7. `99-finalize-pack.md`

Optional tasks may be skipped if they are not needed, but `99-finalize-pack.md` must always run.

---

## Task Definitions

### 1. `01-discover-context.md`

Purpose:

- inspect the repo
- identify the relevant files
- confirm the current behavior
- define the smallest safe implementation path

Expected output:

- files inspected
- current behavior summary
- proposed implementation path
- risks or unknowns

No functional code changes should be made unless explicitly required.

---

### 2. `02-implement-core.md`

Purpose:

- implement the core change
- keep edits narrow
- avoid unrelated cleanup

Expected output:

- files changed
- reason for each change
- any assumptions made

---

### 3. `03-add-validation.md`

Purpose:

- add or update tests
- run the narrowest useful validation command
- confirm the core behavior works

Expected output:

- tests added or updated
- validation command
- validation result

---

### 4. `04-refactor-cleanup.md`

Purpose:

- clean up implementation details only after the core change is working
- remove duplication if needed
- improve readability without changing behavior

Expected output:

- files changed
- cleanup summary
- validation command
- validation result

This task is optional.

---

### 5. `05-update-docs.md`

Purpose:

- update documentation if this pack changes commands, setup, behavior, APIs, workflows, or user-facing expectations

Expected output:

- documentation files updated
- summary of documentation changes
- any docs intentionally left unchanged and why

This task is required when documented behavior changes.

---

### 6. `06-add-follow-up-tests.md`

Purpose:

- add extra tests only if validation from task 03 found missing edge cases

Expected output:

- tests added
- edge cases covered
- validation command
- validation result

This task is optional.

---

### 7. `99-finalize-pack.md`

Purpose:

- run final validation
- update documentation if still needed
- check `.ptolemy/context` if needed
- confirm all task branches are merged
- prepare the pack branch for Pull Request

Expected output:

- final test results
- documentation updated
- task branches merged
- PR summary draft
- known risks or follow-up work

---

## Documentation Rule

When a completed task changes documented behavior, commands, setup, workflow, API behavior, or user-facing expectations, add or run a documentation task before the pack is considered complete.

Documentation targets may include:

- `README.md`
- `WORKFLOWS.md`
- service-specific README files
- `.ptolemy/context/architecture.md`
- `.ptolemy/context/commands.md`
- `.ptolemy/context/conventions.md`

---

## Global Validation

Use the narrowest command that proves the pack is complete.

Default:

```bash
go test ./...
```

Replace this with a narrower command when possible.

Examples:

```bash
go test ./internal/executor/...
go test ./internal/httpapi/...
go test ./cmd/workerd/...
```

---

## Final Pull Request

After all tasks are merged into the pack branch, raise one Pull Request.

PR source branch:

```text
feature/<ddmmyy>-<pack-name>
```

PR target branch:

```text
main
```

The PR description should include:

- pack goal
- task list completed
- files changed
- validation commands
- documentation updates
- known risks or follow-up work
