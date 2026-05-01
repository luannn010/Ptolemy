# Task Script: 03-client-config-init.md

## Operation
create/update

## Target file
`cmd/ptolemy-client/*, internal/client/config/*, internal/client/init/*`

## Instructions
Create a `ptolemy-client` CLI with `init` command.

Minimum behavior:
- `ptolemy-client init` creates `.ptolemy/` tree in current workspace.
- Add `.ptolemy/` to `.gitignore` if missing.
- Create `.ptolemy/client.yaml`.
- Do not overwrite existing user context files unless `--force` is passed.


Preserve unrelated code. Add tests where behavior is new.
