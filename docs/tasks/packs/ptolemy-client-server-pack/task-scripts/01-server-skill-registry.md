# Task Script: 01-server-skill-registry.md

## Intent
Implement a server-side skill registry for the client/server architecture without introducing Docker requirements.

## Allowed targets
`internal/skills/*, internal/httpapi/*, cmd/workerd/*, docs/skills/*`

## Stable anchors
- existing skill-loading code in `internal/skills/`
- existing HTTP route registration in `cmd/workerd/`
- existing HTTP response helpers in `internal/httpapi/`

## Referenced snippets
- `snippets/client-server-architecture.md`

## Expected outputs

- A configurable server-side skill registry exists.
- An HTTP API can list available skills.
- An HTTP API can read one skill by id or name.
- The API returns JSON responses.
- Tests cover registry path validation and not-found behavior.

## Must not do

- Do not require Docker.
- Do not edit files outside the task's `allowed_files`.
- Do not rewrite unrelated server subsystems.

## Instructions
Implement the minimum registry behavior described above using targeted edits and add tests only where the new behavior is introduced.
