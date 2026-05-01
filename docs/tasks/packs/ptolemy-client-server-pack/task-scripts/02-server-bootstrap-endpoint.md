# Task Script: 02-server-bootstrap-endpoint.md

## Operation
create/update

## Target file
`internal/httpapi/*, internal/bootstrap/*`

## Instructions
Add a bootstrap endpoint that returns the recommended `.ptolemy/` client structure.

Minimum behavior:
- Endpoint returns default context file names, task folders, memory folders and client config template.
- Include server URL and project name as optional request inputs.
- Do not write files to the target project from the server endpoint; it only returns bootstrap data.


Preserve unrelated code. Add tests where behavior is new.
