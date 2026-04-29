# Task Branch and Safe Merge Workflow

## Rules

- Never start from a dirty working branch
- Never overwrite user changes
- Never use `git add .`
- One branch per task
- Commit only tested changes
- Merge only if working branch is clean
- If conflict occurs → STOP and report
- Do not auto-resolve runtime code conflicts

## Workflow

### 1. Check working state

```bash
git branch --show-current
git status --short
```

If not clean → STOP

### 2. Create task branch

```bash
git checkout <working-branch>
git pull --ff-only
git checkout -b ptolemy/<task-slug>
```

### 3. Implement task

Use Ptolemy for execution

### 4. Run tests

```bash
go test ./...
```

### 5. Stage explicitly

```bash
git status --short
git add <file1> <file2>
git diff --cached --name-only
```

### 6. Commit

```bash
git commit -m "<message>"
```

### 7. Merge back

```bash
git checkout <working-branch>
git status --short
git pull --ff-only
git merge --no-ff ptolemy/<task-slug>
```

### 8. Conflict handling

```bash
git status --short
git diff --name-only --diff-filter=U
```

STOP and report:
- conflicted files
- branch names
- test status

## Conflict policy

- Docs → can suggest fix
- Config → suggest + validate
- Runtime code → DO NOT auto-resolve
- User files → DO NOT touch
