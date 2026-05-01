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
2. Read `.ptolemy/context/project-map.md`.
3. Search by keyword or symbol.
4. Read only top relevant files.
5. Make small changes.
6. Run targeted tests.
7. Save session notes.