# Task Script: 04-client-workspace-guard.md

## Operation
create/update

## Target file
`internal/client/workspace/*`

## Instructions
Implement workspace path guard.

Minimum behavior:
 - Reject `../` traversal and symlink escapes where practical.

 - Reject symlink escapes where practical.

- Resolve workspace root to absolute path.
- Clean and resolve requested paths.
- Reject `../` traversal and symlink escapes where practical.
- Unit test valid paths, traversal paths and absolute paths outside workspace.


Preserve unrelated code. Add tests where behavior is new.
