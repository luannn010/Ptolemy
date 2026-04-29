# Pull Request Workflow

Use this workflow after a task branch is committed and a pull request should be opened instead of merging directly into the working branch.

## Purpose

The PR workflow keeps task branches reviewable and gives the task author a clear handoff path:

```text
task file -> task branch -> implementation -> tests -> commit -> push branch -> open PR -> review -> merge
```

## Rules

- Do not create a PR from a dirty branch.
- Do not push unrelated changes.
- Do not use `git add .`.
- Only use task branches created for the current task.
- Include the priority and task ID in the PR title.
- Include the task file, scope, validation, and risk notes in the PR body.

## Verify the branch

```bash
git status --short
git branch --show-current
git log -1 --oneline
```

If the branch is dirty, stop and clean it before creating a PR.

## Build the PR body

Create a body file before invoking GitHub tooling.

Suggested path:

```text
.state/pr/<task_id>-pr-body.md
```

Include:

- task file path
- task ID
- parent task, if any
- allowed files changed
- validation commands run
- test result
- known risks

Example body outline:

```md
# PR Summary

- Task file: `docs/tasks/inbox/<task-file>.md`
- Task ID: `<task-id>`
- Parent task: `<parent-task-or-null>`
- Allowed files changed: `<paths>`

## Validation

- `<command>`
- `<command>`

## Result

- `<pass/fail and short note>`

## Risks

- `<known risk or none>`
```

## Push the task branch

```bash
git push -u origin <task-branch>
```

Only push after tests pass and the branch contains the expected task files.

## Create the PR with GitHub CLI

If `gh` is installed and authenticated:

```bash
gh auth status
gh pr create --base <working-branch> --head <task-branch> --title "<priority>: <task_id>" --body-file .state/pr/<task_id>-pr-body.md
```

## Fallback when GitHub CLI is unavailable

If `gh` is missing or auth fails, write an instruction file instead of failing the task.

Suggested path:

```text
.state/pr/<task_id>-pr-instructions.md
```

Include:

- branch name
- target branch
- commit hash
- PR title
- PR body
- commands attempted
- reason the PR was not created automatically

## Required report

After the PR step, report:

- branch name
- commit hash
- PR title
- PR URL if created
- fallback file path if used
- any remaining review risk
