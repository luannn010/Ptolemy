# Task Pack Template

Use this folder when a change needs multiple related tasks, shared context, and controlled branch/PR workflow.

## Purpose

A task pack helps Ptolemy, Codex, or another agent execute work safely by splitting a larger change into small deterministic tasks.

Each pack should have:

- one clear goal
- one pack branch
- one or more task branches
- validation after each meaningful change
- one final Pull Request

---

## Suggested Structure

```text
<pack-name>/
├── PACK_MANIFEST.yaml
├── README.md
├── TASK_PLAN.md
├── inbox/
│   ├── 01-discover-context.md
│   ├── 02-implement-core.md
│   ├── 03-add-validation.md
│   ├── 04-refactor-cleanup.md
│   ├── 05-update-docs.md
│   ├── 06-add-follow-up-tests.md
│   └── 99-finalize-pack.md
├── snippets/
├── task-scripts/
└── scripts/
```

---

## Branching Workflow

Create one branch for the whole pack:

```bash
git checkout -b feature/<ddmmyy>-<pack-name>
```

Create one branch per task from the pack branch:

```bash
git checkout -b feature/<ddmmyy>-<pack-name>/task-01
```

Merge task branches back into the pack branch:

```bash
git checkout feature/<ddmmyy>-<pack-name>
git merge --no-ff feature/<ddmmyy>-<pack-name>/task-01
```

When the pack is complete, raise one Pull Request from the pack branch into `main`.

---

## If a Task Is Too Big

Split the task into smaller branches:

```text
feature/<ddmmyy>-<pack-name>/task-02a
feature/<ddmmyy>-<pack-name>/task-02b
feature/<ddmmyy>-<pack-name>/task-02c
```

Each split branch should be validated before merging into the pack branch.

---

## Completion Rule

The pack is complete only when:

- all required tasks are done
- task branches are merged into the pack branch
- validation passes
- documentation is updated
- the PR is ready
