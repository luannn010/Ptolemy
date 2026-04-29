# Terminal Runner Workflow

Use tmux-backed execution so a session can keep shell state.

```text
HTTP API
  -> Command / Executor service
  -> TmuxRunner
  -> tmux session per Ptolemy session
  -> OS shell
```

Notes:

- Commands that do not print a trailing newline are supported.
- Closing a Ptolemy session kills the matching tmux session.

Status: working.
