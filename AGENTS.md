# AGENTS.md - Ptolemy Worker Development Guide

## Purpose

This file defines mandatory operating rules for Codex/agents working in this repository.

## Ptolemy-First Rule (Always On)

- For every user prompt in this repository, the agent must use Ptolemy workflows and Ptolemy-native capabilities first.
- The agent must treat Ptolemy as the default execution path for planning, editing, task execution, validation, and delivery.
- If a request is ambiguous, the agent should still anchor execution in Ptolemy workflow files before taking action.

## Superpowers Plugin Rule (Always On)

- The agent must use `[@superpowers](plugin://superpowers@openai-curated)` as the default augmentation layer for planning, implementation, validation, and delivery whenever capabilities overlap with base behavior.
- Superpowers usage does not replace Ptolemy-first execution; it must be applied within Ptolemy workflow boundaries.
- If Superpowers capabilities are unavailable in-session, the agent must continue with Ptolemy-native execution and clearly note the fallback.

## GitHub Plugin Rule (Always On for Git Interactions)

- For repository and GitHub interactions, the agent must use the GitHub plugin/app capabilities first.
- Default GitHub-plugin-first scope includes: pull request creation/updates, PR comments/review handling, issue operations, and GitHub metadata retrieval.
- If the GitHub plugin path is unavailable or lacks a required capability, the agent may fall back to git/CLI commands and must briefly state the reason for fallback.
- Branching/commit safety rules in this file still apply when using either plugin or CLI fallback.

### TDD Convention (Required with Superpowers)

- Follow strict TDD for all non-trivial code changes: `Red -> Green -> Refactor`.
- Start by adding or updating a failing test that captures expected behavior.
- Implement the smallest code change needed to pass the new/updated test.
- Refactor only after tests are green; keep refactors behavior-preserving.
- Do not submit implementation changes without corresponding tests unless the task is docs/config-only; if tests are not practical, explicitly document why.

### Superpowers Skill Discipline

- Prefer Superpowers skill workflows before ad-hoc execution when a matching skill exists.
- Load only the minimal required skill(s) for the active task and keep edits task-scoped.
- Preserve repository safety rules, branch/commit policy, and PR template requirements while using Superpowers.

Mandatory execution checklist:
- `docs/workflows/agent/agents-compliance-checklist.md`
- The checklist must be followed as a hard gate before coding, before each commit phase, before push, and before PR creation.

## Required Startup Flow

Before any implementation task:
1. Read `WORKFLOWS.md`.
2. Load only the workflow file(s) needed for the current task.
3. Confirm task scope and allowed files.
4. Keep changes small, reversible, and task-scoped.
5. Complete Section 1 (Startup Gate) in `docs/workflows/agent/agents-compliance-checklist.md`.

## Branching and Commit Policy

### Feature implementation

When asked to implement a feature, the agent must ask:
- "Do you want me to create a new branch for this feature?"

Then apply one of these paths:

1. If user says yes:
- create a new branch from the current branch using `ptolemy/<task-slug>` unless task metadata provides a branch name.
- implement in phases.
- stage and commit per completed phase (not one large commit containing all edits).
- open/create a PR back to the original branch when implementation is complete.

2. If user says no:
- implement on current branch.
- still stage and commit by completed phase.
- if change is very small and single-phase, one focused commit is allowed.

### Commit hygiene

Always:
- stage explicit files only.
- keep commits focused to one completed phase/sub-phase.
- run relevant tests before each phase commit when practical.
- use clear commit messages (`feat(...)`, `fix(...)`, `chore(...)`, `docs(...)`).

Never:
- use `git add .`
- create "all-in-one" commits for multi-phase work
- stage unrelated files

## Taskpack Policy (Ptolemy Skill)

When creating or executing a taskpack, the agent must ask and validate branch topology first.

Required confirmation:
- "Should I create a new parent branch from the current branch for this taskpack?"

If yes, enforce this workflow:
1. Create parent branch from current branch.
2. For each task file, create a dedicated sub-branch from the parent branch.
3. Implement each task file on its own sub-branch with multiple focused commits (not one large commit).
4. Merge each completed sub-branch back into the parent branch.
5. Run validations/tests on parent branch after merges.
6. Create one PR from parent branch to the original branch.

If no parent branch is approved:
- stop and ask user how to proceed before implementing the taskpack.

## Task Isolation Rules

- Work on one `task_id` at a time unless taskpack workflow explicitly defines parallel child tasks.
- Do not edit outside `allowed_files`.
- Child tasks must use unique `task_id` values.
- Child tasks inherit `priority`, `parent_task`, and `allowed_files` unless explicitly overridden.

## Safety Rules

- Never push without explicit user approval.
- Never run destructive commands unless explicitly requested.
- Never delete files unless task instructions explicitly require deletion.
- Do not auto-resolve merge conflicts; stop and report.
- If a command fails repeatedly, inspect logs/artifacts before retrying.

## Script Execution Rules

- Do not run scripts unless user explicitly approves or `--allow-scripts` is provided.
- Prefer Go-native implementation for permanent capabilities.
- Use scripts only for narrow bootstrap/migration steps.

## PR Rules

- Create PR only when requested by workflow or user, or when operating on a user-approved feature/task branch.
- All PRs MUST use the template at `.github/pull_request_template.md`.
- Agent must fill all required template sections before opening the PR.
- If PR tooling is unavailable, write fallback PR instructions under `.state/pr/`.
- Do not auto-merge PRs unless explicitly requested.

## Minimal Done Criteria

A task is done only when:
- implementation is complete for approved scope,
- relevant tests pass (or failure is reported clearly),
- commits are clean and phase-grouped,
- branch/PR behavior follows the rules above,
- TDD evidence is present for code changes (failing test introduced first, then passing result) or an explicit exception is documented.
