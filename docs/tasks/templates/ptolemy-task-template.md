# Ptolemy Task Template

Use this template whenever Codex is asked to build a task for DateDrop and execute it with Ptolemy. The task must be specific enough that a worker can follow it without inventing the implementation plan.

## Required Task Header

```md
# Task: <short action-oriented title>

Parent branch: <current branch before work starts>
Task branch: codex/<short-kebab-case-task-name>
Ptolemy session: <session id or task session id>
Owner: Codex using Ptolemy-first workflow
Status: Draft | In progress | Ready for review | Merged
```

## Non-Negotiable Rules

1. Prefer Ptolemy tool calls for task setup, notes, file tracking, knowledge-base updates, and task status before using direct Codex-only actions.
2. Read the task fully before changing files.
3. Create a new branch from the current branch before making code changes.
4. Keep the parent branch unchanged until the task branch is complete.
5. Every implementation step must name exact files to create, edit, or delete.
6. Do not write vague implementation steps such as "implement auth" or "update backend logic". Replace them with explicit file-level actions.
7. Make small, meaningful commits after each coherent change.
8. Add or update unit tests for the changed behavior.
9. Run a smoke test that proves the completed feature works at the workflow level.
10. Every generated task file must include a complete "Context" section before the implementation plan.
11. The "Context" section must capture product intent, relevant existing files, current behavior, desired behavior, constraints, assumptions, risks, dependencies, and testing context.
12. Every generated task file must identify relevant README or documentation files in the directories affected by the task.
13. If behavior, setup, API shape, workflow, environment variables, commands, or developer usage changes, update the relevant README files as part of the task.
14. If no README update is needed, the task must explicitly explain why.
15. Update Ptolemy notes, changes, files-read, and test results as work progresses.
16. When all checks pass, merge the task branch back into the parent branch.
17. Update the Ptolemy knowledge base after the merge with changed files, commit SHA, completed task ids, and session id.

## Ptolemy-First Execution Flow

Use this checklist as the default execution order.

- [ ] Create or reuse a Ptolemy session for this task.
  - Required tool preference: `ptolemy_create_session`
- [ ] Start a task session with the full task text.
  - Required tool preference: `ptolemy_start_task_session`
- [ ] Build or refresh the project knowledge base before planning changes.
  - Required tool preference: `ptolemy_kb_build`
- [ ] Write the task "Context" section from the user request, Ptolemy KB, and files read so far.
- [ ] Append a note summarizing the files and behavior that need to be changed.
  - Required tool preference: `ptolemy_append_session_note`
- [ ] Create the task branch from the parent branch.
- [ ] Read every file listed in "Files to Read First".
- [ ] Locate and read relevant README/documentation files near the files being changed.
- [ ] Update the task "Context" section if file reading changes the understanding of the work.
- [ ] Add README/documentation updates to the file-level implementation plan, or record why no docs update is needed.
- [ ] Append a Ptolemy note with the confirmed implementation plan.
- [ ] Apply one small change group at a time.
- [ ] After each change group, run the matching focused test.
- [ ] Commit each successful change group with a meaningful commit message.
- [ ] Append a Ptolemy note after each commit with the commit message, files changed, and verification result.
- [ ] Run all unit tests listed in "Unit Tests".
- [ ] Run every smoke test listed in "Smoke Tests".
- [ ] Update the Ptolemy task session test results.
- [ ] Update the Ptolemy knowledge base for changed files.
  - Required tool preference: `ptolemy_kb_update`
- [ ] Merge the task branch back into the parent branch only after tests pass.
- [ ] Update Ptolemy with the merge commit SHA and mark the task complete.
