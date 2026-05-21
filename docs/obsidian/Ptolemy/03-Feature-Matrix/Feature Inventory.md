---
project: Ptolemy
category: feature-matrix
status: Partial
last_verified: 2026-05-11
source: codebase
confidence: high
tags:
  - ptolemy
  - feature-matrix
---

# Feature Inventory

| Feature | Category | Status | Evidence | Entrypoint / API | Tests | Ready to use? | Gap | Next improvement |
|---|---|---|---|---|---|---|---|---|
| workerd daemon | Core Runtime | Ready | `cmd/workerd/main.go` | `go run ./cmd/workerd` | `cmd/workerd/main_test.go` | Yes | No fresh runtime pass in this audit | Add CI smoke test |
| health endpoint | Core Runtime | Ready | `internal/httpapi/router.go` | `GET /health` | `internal/httpapi/router_test.go` | Yes | No live call in this audit | Add documented example response |
| HTTP command execution API | Core Runtime | Ready | `internal/httpapi/execute.go`, `internal/executor/executor.go` | `POST /execute` | `internal/httpapi/router_test.go`, `internal/httpapi/commands_test.go` | Yes | Go tests not run here | Add integration smoke test |
| session creation/list/get/close | Core Runtime | Ready | `internal/httpapi/session.go`, `internal/session/store.go` | `/sessions` | `internal/session/store_test.go`, `internal/httpapi/router_test.go` | Yes | No live validation in this audit | Add API examples |
| session command logs | Observability | Ready | `internal/command/store.go` | `GET /sessions/{id}/commands` | `internal/command/store_test.go` | Yes | No retention docs | Add retention policy |
| SQLite persistence | Core Runtime | Ready | `internal/store/store.go`, `internal/store/migrations.go` | `state/ptolemy.db` | `internal/store/store_test.go` | Yes | No migration versioning docs | Add schema note |
| schema migrations | Core Runtime | Ready | `internal/store/migrations.go` | Startup migration path | `internal/store/store_test.go` | Yes | No migration changelog note | Add migration history doc |
| tmux runner | Tools | Partial | `internal/terminal/tmux_runner.go` | `internal/terminal.TmuxRunner` | `internal/terminal/tmux_runner_test.go` | Likely | Environment assumptions | Add live smoke test |
| generic runner fallback | Tools | Partial | `internal/terminal/runner.go` | `internal/terminal.Runner` | `internal/terminal/runner_test.go` | Likely | Not inspected live here | Add platform notes |
| file read/write/list/search/apply | Tools | Ready | `internal/httpapi/fileops.go`, `internal/fileops/fileops.go` | `/file/*` | `internal/fileops/fileops_test.go` | Yes | No example payload docs | Add API examples |
| MCP server | Connectors | Ready | `cmd/ptolemy-mcp/main.go`, `internal/mcp/server.go` | stdio MCP | `cmd/ptolemy-mcp/main_test.go`, `internal/mcp/server_test.go` | Yes | No live MCP handshake in audit | Add smoke test |
| MCP file/session/git/worktree/navigator tools | Tools | Ready | `internal/mcp/*tools/tools.go` | `tools/list`, `tools/call` | `internal/mcp/*tools/*_test.go` | Yes | Tool catalog docs were missing | Generate catalog docs |
| KB build/read/update | Knowledge Base | Ready | `internal/navigator/kb.go`, `internal/httpapi/router.go` | `/kb/build`, `/kb/read`, `/kb/update` | `internal/navigator/*_test.go` | Yes | No live refresh recorded | Add end-to-end KB test |
| compatibility navigator paths | Knowledge Base | Partial | `internal/httpapi/router.go` | `/navigator/*` | `internal/httpapi/router_test.go` | Yes | Legacy/canonical overlap | Clarify deprecation path |
| workspace memory loader | Knowledge Base | Partial | `internal/memory/loader.go` | internal package | `internal/memory/loader_test.go` | Likely | No runtime usage report | Add docs and metrics |
| command policy | Security | Partial | `internal/policy/policy.go` | internal package | `internal/policy/policy_test.go` | Yes | Simple substring policy | Move to structured parsing |
| approval records | Security | Partial | `internal/approval/store.go` | SQLite approvals | Store-level tests indirect | Partial | No explicit approval API found | Add operator endpoints |
| agent-runs API | ChatCLI | Partial | `internal/httpapi/agent_runs.go` | `/agent-runs` | `internal/httpapi/router_test.go` | Likely | Live path not exercised | Add e2e test |
| controller-owned agent loop | ChatCLI | Partial | `internal/agentloop/service.go`, `internal/agentloop/tool_executor.go` | internal service | `internal/agentloop/*_test.go` | Likely | Coexists with prototype CLI | Declare canonical path |
| local `ptolemy-agent` CLI | ChatCLI | Prototype | `cmd/ptolemy-agent/main.go` | `go run ./cmd/ptolemy-agent` | `cmd/ptolemy-agent/*_test.go` | With caution | Prototype status | Clarify support level |
| reasoning profiles | Model / Inference | Partial | `internal/agentloop/reasoning_profile.go` | internal service | `internal/agentloop/reasoning_profile_test.go` | Likely | Narrow profile set | Add profile matrix |
| local brain client | Model / Inference | Partial | `internal/brain/client.go` | `BRAIN_BASE_URL` | `internal/brain/client_test.go` | Likely | Single-provider feel | Add health and fallback strategy |
| Git helpers | Tools | Ready | `internal/httpapi/git.go`, `internal/gitops/gitops.go` | `/git/*` | `internal/gitops/gitops_test.go` | Yes | No push-safety layer docs | Add examples and safeguards |
| worktree management | Tools | Partial | `internal/httpapi/worktree.go`, `internal/worktree/worktree.go` | `/worktree/*` | `internal/worktree/worktree_test.go` | Likely | Local-only isolation | Add lifecycle docs |
| task file scanner/validator | Workflow Engine | Ready | `internal/tasks/scanner.go`, `internal/tasks/validator.go` | internal package | `internal/tasks/scanner_test.go`, `internal/tasks/validator_test.go` | Yes | No live run result | Add examples |
| task runner CLI plan/run/bootstrap | Workflow Engine | Partial | `cmd/ptolemy-task-runner/main.go` | CLI commands | `cmd/ptolemy-task-runner/*_test.go` | Likely | Go toolchain unavailable here | Add CI and docs |
| task pack parser | Workflow Engine | Ready | `internal/tasks/pack.go` | internal package | `internal/tasks/pack_test.go` | Yes | No manifest generator doc | Add pack authoring guide |
| large-task process manifests | Workflow Engine | Partial | `internal/tasks/process.go`, `internal/tasks/pack_runtime.go` | `.ptolemy/tasks/process/*` | `internal/tasks/process_test.go` | Likely | Not exercised live here | Add walkthrough |
| Pack Studio web UI | UI | Partial | `internal/httpapi/packstudio.go`, `internal/httpapi/ui/*` | `/ui` | `internal/httpapi/packstudio_test.go` | Likely | No browser validation | Add browser smoke tests |
| program/pack run tracking | Observability | Partial | `internal/packstudio/service.go`, `internal/store/migrations.go` | `/ui/api/program-runs*` | `internal/httpapi/packstudio_test.go` | Likely | No live long-run evidence | Add run replay test |
| audit logs and run events | Observability | Partial | `internal/logging/store.go`, `internal/store/migrations.go` | SQLite tables | `internal/logging/logging_test.go` | Likely | No retention policy | Add retention cleanup |
| deployment service unit | Deployment | Partial | `deploy/workerd.service` | systemd | None found | Partial | Single deployment artifact | Add more deployment modes |
| distributed workers / nodes | Distributed Workers | Unknown | No verified subsystem found | N/A | N/A | No | Implementation absent or unverified | Decide scope explicitly |
| skills subsystem | Skills | Planned | `AGENTS.md` design note only | N/A | N/A | No | No `internal/skills` implementation | Decide whether to build it |

