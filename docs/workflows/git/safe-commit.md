# Git Safe Commit Workflow

Use this workflow whenever an agent is ready to commit task changes.

## Rules

- Never use `git add .`
- Never use `git commit -am`
- Stage only files named by the task or explicitly created by the task.
- Check status before staging.
- Check staged files before committing.
- Run required tests before committing.

## Commands

```bash
git status --short
git add <explicit-file-1> <explicit-file-2>
git diff --cached --name-only
git diff --cached --stat
git commit -m "<message>"
```

## Required report

After committing, report:

* tests run
* files staged
* commit hash
* any files left unstaged
