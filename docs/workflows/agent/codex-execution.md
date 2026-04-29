# Codex Execution Workflow

Ptolemy acts as the execution backend while Codex plans, edits, and validates.

```text
Codex
  -> Reads AGENTS.md and .ptolemy context
  -> Uses navigator/search/read tools
  -> Makes targeted edits
  -> Runs commands/tests through Ptolemy
  -> Summarizes results
```

Status: working as a supported development pattern.
