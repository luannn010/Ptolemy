# Task Script: 07-client-skill-sync.md

## Operation
create/update

## Target file
`internal/client/skillsync/*, cmd/ptolemy-client/*`

## Instructions
Implement skill sync from server.

Minimum behavior:
- Read `server_url` from `.ptolemy/client.yaml`.
- Fetch skill list from server.
- Download selected skills into `.ptolemy/cache/skills`.
- Support `ptolemy-client sync-skills` and `ptolemy-client skills list`.
- Fail gracefully if server is offline.


Preserve unrelated code. Add tests where behavior is new.
