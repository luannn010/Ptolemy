# Task Script: 08-client-codebase-scan.md

## Operation
create/update

## Target file
`internal/client/scan/*, cmd/ptolemy-client/*`

## Instructions
Implement codebase scanning to local memory.

Minimum behavior:
- Scan project files excluding `.git`, `.ptolemy`, `node_modules`, `vendor`, `dist`, `build`, binary files.
- Generate `.ptolemy/memory/codebase-map.md`.
- Generate `.ptolemy/memory/dependency-map.md` when package manifests exist.
- Generate `.ptolemy/memory/recent-changes.md` from git status/log if available.


Preserve unrelated code. Add tests where behavior is new.
