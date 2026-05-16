---
project: Ptolemy
category: workflows
status: Partial
last_verified: 2026-05-11
source: codebase
confidence: medium
tags:
  - ptolemy
  - workflows
---

# Codex to Ptolemy Flow

The current intended Codex-to-Ptolemy path is:

1. Read `WORKFLOWS.md`.
2. Load KB or workflow context before broad repo scanning.
3. Use `workerd` or `ptolemy-mcp` as the deterministic execution layer.
4. For structured work, use task files or task packs.
5. Let the worker or task runner own command execution, logging, and state.

Evidence:

- `WORKFLOWS.md`
- `docs/workflows/agent/codex-execution.md`
- `README.md`
- `cmd/ptolemy-mcp/main.go`
- `cmd/workerd/main.go`

