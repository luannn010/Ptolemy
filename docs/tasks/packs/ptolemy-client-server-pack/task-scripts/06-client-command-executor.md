# Task Script: 06-client-command-executor.md

## Operation
create/update

## Target file
`internal/client/exec/*, cmd/ptolemy-client/*`

## Instructions
Implement local command execution.

Minimum behavior:
- Run commands in workspace root.
- Use configured shell, default `/bin/bash`.
- Capture stdout/stderr and exit code.
- Enforce timeout.
- Add allow/deny policy hook for future command safety.


Preserve unrelated code. Add tests where behavior is new.
