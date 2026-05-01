# Task Script: 05-client-file-operations.md

## Operation
create/update

## Target file
`internal/client/fileops/*, cmd/ptolemy-client/*`

## Instructions
Implement local client file operations.

Minimum behavior:
- read file
- write file
- insert after marker
- replace block between markers
- delete file only when config allows delete or CLI flag confirms it
- All operations must pass through workspace guard.


Preserve unrelated code. Add tests where behavior is new.
