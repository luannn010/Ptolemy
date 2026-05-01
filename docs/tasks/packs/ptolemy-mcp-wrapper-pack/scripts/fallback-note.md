# Fallback Note

Full `go run ./cmd/ptolemy-task-runner run --pack ...` execution was not used for this pack by default.

Reason:

- the current pack runtime prepares branches, converges them, pushes, and drafts a PR
- repo instructions require explicit approval before any push
- the repository already had unrelated local modifications that should not be swept into automated branch work

Safe fallback used for this pack:

1. run `go run ./cmd/ptolemy-task-runner plan --pack docs/tasks/packs/ptolemy-mcp-wrapper-pack`
2. run the listed wrapper smoke tests manually
3. execute the inbox tasks through the existing local Ptolemy workflow only after explicit approval for any push-capable automation
