# Client-Server Architecture Contract

## Server responsibilities
- Serve shared skills, task templates and workflow rules.
- Provide bootstrap payload for creating `.ptolemy/` in a target project.
- Avoid direct file access to target projects unless the project explicitly creates a session with that workspace.

## Client responsibilities
- Run inside a target project.
- Create and maintain `.ptolemy/` local state.
- Read/write/edit/delete files only within the workspace root.
- Run shell commands inside the target workspace.
- Fetch and cache skills from the server.
- Generate codebase memory files.

## Safety rules
- Reject path traversal outside workspace.
- Require explicit flags for delete operations.
- Do not commit `.ptolemy/` by default.
- Prefer dry-run support for destructive operations.
