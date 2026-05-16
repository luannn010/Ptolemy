# AGENTS Compliance Checklist

Use this checklist before implementation, before each commit phase, before push, and before PR creation.

## 1) Startup Gate (mandatory before coding)

- [ ] Read `WORKFLOWS.md`.
- [ ] Load only workflow file(s) needed for this task.
- [ ] Confirm task scope and allowed files.
- [ ] Confirm change is small/reversible/task-scoped.

## 2) Branching Gate

### Feature work

- [ ] Asked user: "Do you want me to create a new branch for this feature?"
- [ ] If yes, created `ptolemy/<task-slug>` (unless task metadata says otherwise).
- [ ] If no, confirmed implementation will continue on current branch.

### Taskpack work

- [ ] Asked user: "Should I create a new parent branch from the current branch for this taskpack?"
- [ ] If yes:
  - [ ] Parent branch created.
  - [ ] Dedicated sub-branch per task file created from parent branch.
  - [ ] Each sub-branch merged back into parent branch after completion.
  - [ ] Parent branch validations/tests run after merges.
  - [ ] Single PR planned from parent -> original branch.
- [ ] If no parent branch approved, stop and ask user how to proceed.

## 3) Task Isolation Gate

- [ ] Working on one `task_id` at a time (unless approved taskpack parallelism).
- [ ] No edits outside `allowed_files`.
- [ ] Child tasks have unique `task_id` values.
- [ ] Child tasks inherit `priority`, `parent_task`, `allowed_files` unless explicitly overridden.

## 4) Safety Gate

- [ ] No push without explicit user approval.
- [ ] No destructive commands unless explicitly requested.
- [ ] No file deletion unless explicitly required by task instructions.
- [ ] No auto-resolve merge conflicts.
- [ ] Repeated command failures handled by checking logs/artifacts before retry.

## 5) Script Execution Gate

- [ ] No script execution unless user approved or `--allow-scripts` is provided.
- [ ] Go-native implementation preferred for permanent capabilities.
- [ ] Scripts used only for narrow bootstrap/migration steps.

## 6) Commit Hygiene Gate (run before each commit)

- [ ] Staged explicit files only.
- [ ] Commit scope = one completed phase/sub-phase.
- [ ] Relevant tests run before commit when practical (or limitation reported).
- [ ] Commit message style uses `feat(...)`, `fix(...)`, `chore(...)`, `docs(...)`.
- [ ] Did not use `git add .`.
- [ ] Did not stage unrelated files.
- [ ] Did not create one all-in-one commit for multi-phase work.

## 7) Pre-Push Gate

- [ ] User explicitly approved push.
- [ ] `git status --short` reviewed.
- [ ] Branch target confirmed.
- [ ] Validation/test results are documented.

## 8) PR Gate (mandatory)

- [ ] PR creation requested/allowed by user or workflow.
- [ ] `.github/pull_request_template.md` used.
- [ ] All template sections completed (or clearly marked N/A).
- [ ] If PR tooling unavailable, fallback instructions written under `.state/pr/`.
- [ ] No auto-merge unless explicitly requested.

## 9) Done Gate

Task is done only when all are true:

- [ ] Implementation complete for approved scope.
- [ ] Relevant tests passed (or failure/limits clearly reported).
- [ ] Commits are clean and phase-grouped.
- [ ] Branch/PR behavior follows all AGENTS rules.

