---
project: Ptolemy
category: tools
status: Ready
last_verified: 2026-05-11
source: codebase
confidence: high
tags:
  - ptolemy
  - implementation
---

# Tools

## What this aspect means

Tools are the callable operations Ptolemy gives to agents and external MCP clients, such as reading files, running commands, updating KB state, or managing worktrees.

## Current implementation status

Status: `Ready`

The codebase contains two clear tool surfaces: worker-owned agent loop actions in `internal/agentloop/tool_executor.go` and MCP tool groups in `internal/mcp/*tools/tools.go`.

## Evidence from codebase

| Evidence | Path | Notes |
|---|---|---|
| Agent tool registration | `internal/agentloop/tool_executor.go` | Registers `read_file`, `write_file`, `replace_block`, `insert_after`, `run_command`, `ask_approval`, `explain` |
| Agent tool registry | `internal/agentloop/tool_registry.go` | Dispatch mechanism |
| MCP tool declarations | `internal/mcp/filetools/tools.go`, `internal/mcp/sessiontools/tools.go`, `internal/mcp/navigatortools/tools.go`, `internal/mcp/gittools/tools.go`, `internal/mcp/worktreetools/tools.go`, `internal/mcp/executortools/tools.go` | External tool catalog |

## Ready-to-use capabilities

- Agent loop file editing and command tools
- MCP session, execute, file, navigator, KB, Git, and worktree tools

## Partial or unfinished capabilities

- The local `ptolemy-agent` prototype and the controller loop expose overlapping capabilities

## APIs / interfaces

| Interface | Path / Endpoint / Command | Purpose | Status |
|---|---|---|---|
| Agent tool registry | `internal/agentloop/tool_registry.go` | Worker-owned action dispatch | Ready |
| MCP tool list | `tools/list` via `internal/mcp/server.go` | Lists exposed MCP tools | Ready |
| MCP tool call | `tools/call` via `internal/mcp/server.go` | Executes tool handlers | Ready |

## Important commands

```bash
go run ./cmd/ptolemy-mcp
```

## Tests and validation

| Test / command | What it validates | Current result |
|---|---|---|
| `internal/agentloop/tool_executor_test.go` | Agent tool behavior | Test file present |
| `internal/mcp/*tools/*_test.go` | MCP tool registration/handling | Test files present |
| `go test ./...` | Repo-wide validation | Could not run here because `go` was unavailable |

## Risks

- Overlap between tool surfaces could confuse future callers.

## Gaps

- No single user-facing matrix of all tools existed before this audit.

## Next recommended improvements

- Publish one canonical tool reference generated from code.
- Decide which tool surfaces are stable vs compatibility-era.

