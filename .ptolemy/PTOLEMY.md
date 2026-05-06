Ptolemy is a codebase navigator, not a whole-codebase reader.

When available, use the `ptolemy-workflows` Codex skill to select the right workflow from
`WORKFLOWS.md`. Codex should route the workflow; Ptolemy should execute deterministic steps.

Golden rule:

```text
Search first.
Read small.
Edit targeted.
Test immediately.
Summarise changes.
Update memory only after confirmed change.
```

Default workflow:

1. Read this file.
2. Read `.ptolemy/kb/PROJECT_MAP.md`.
3. Use `.ptolemy/kb/FILE_INDEX.json` and `.ptolemy/kb/SYMBOL_INDEX.json` to choose likely files.
4. Search by keyword or symbol only after KB triage.
5. Read only top relevant files.
6. Make small changes.
7. Run targeted tests.
8. Save session notes.
9. Update the KB only after confirmed useful changes.
