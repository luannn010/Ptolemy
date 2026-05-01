# Client-Server Local Workflow (No Docker)

This workflow describes the no-Docker MVP where `workerd` runs locally and `ptolemy-client` runs inside a target project workspace.

## Roles

- Server (`workerd`): provides worker APIs and skill endpoints.
- Client (`ptolemy-client`): initializes `.ptolemy/`, syncs skills, scans local codebase, and runs local workspace commands.

## Commands

Initialize client state in a target project:

```bash
ptolemy-client init --workspace .
```

List available skills from server:

```bash
ptolemy-client skills list --workspace .
```

Sync all skills:

```bash
ptolemy-client sync-skills --workspace .
```

Sync selected skills:

```bash
ptolemy-client sync-skills --workspace . --skills "go/fmt,go/test"
```

Run codebase scan:

```bash
ptolemy-client scan --workspace .
```

Run a workspace command with timeout:

```bash
ptolemy-client exec --workspace . --command "go test ./..."
```

## Generated Files

The client creates and maintains workspace-local state under `.ptolemy/`, including:

- `.ptolemy/client.yaml`
- `.ptolemy/cache/skills/*`
- `.ptolemy/memory/codebase-map.md`
- `.ptolemy/memory/dependency-map.md`
- `.ptolemy/memory/recent-changes.md`

`.ptolemy/` should be gitignored in target projects.

## Troubleshooting

Server offline:

- Symptom: `skills list` / `sync-skills` returns a server unavailable error.
- Action: verify server URL in `.ptolemy/client.yaml` and ensure `workerd` is reachable.

Command execution failures:

- Symptom: non-zero `exit_code` or `timed_out: true` from `ptolemy-client exec`.
- Action: re-run command manually in workspace and adjust timeout or command scope.
