---
project: Ptolemy
category: chatcli
status: Prototype
last_verified: 2026-05-11
source: codebase
confidence: medium
tags:
  - ptolemy
  - implementation
---

# ChatCLI

## What this aspect means

This aspect covers command-line conversation or task execution loops where an LLM interacts with Ptolemy step by step from a terminal interface.

## Current implementation status

Status: `Prototype`

`cmd/ptolemy-agent` is a local LLM-driven executor prototype that reads a task or task file, loads KB context, talks to the local brain, and performs one JSON action at a time through the worker client. It is usable as a prototype but is not clearly the final canonical conversation layer.

## Evidence from codebase

| Evidence | Path | Notes |
|---|---|---|
| Local agent prototype | `cmd/ptolemy-agent/main.go` | Main CLI loop |
| Recovery tests | `cmd/ptolemy-agent/recovery_test.go` | Invalid JSON and recovery scenarios |
| Prompt/tool constraints | `cmd/ptolemy-agent/main.go` | Defines action contract |
| Worker-facing client | `internal/worker/client.go` | Prototype depends on worker API |

## Ready-to-use capabilities

- Run a task file through a local brain-and-worker loop
- Load KB prompt context before action generation
- Enforce one JSON object per step

## Partial or unfinished capabilities

- Not clearly aligned with the newer `internal/agentloop` controller loop
- Environment setup for the local brain is manual

## APIs / interfaces

| Interface | Path / Endpoint / Command | Purpose | Status |
|---|---|---|---|
| CLI | `cmd/ptolemy-agent` | Local task execution loop | Prototype |
| Brain API | `BRAIN_BASE_URL/v1/chat/completions` | Model responses | Partial |
| Worker API | `WORKER_BASE_URL` | Executes tool requests | Partial |

## Important commands

```bash
go run ./cmd/ptolemy-agent --task-file docs/tasks/inbox/<task>.md --max-steps 8
go run ./cmd/ptolemy-agent --allow-scripts --task-file docs/tasks/inbox/<task>.md --max-steps 3
```

## Tests and validation

| Test / command | What it validates | Current result |
|---|---|---|
| `cmd/ptolemy-agent/recovery_test.go` | Multi-object recovery behavior | Test file present |
| `cmd/ptolemy-agent/prompt_test.go` | Prompt-related expectations | Test file present |
| `cmd/ptolemy-agent/artifact_naming_test.go` | Artifact path behavior | Test file present |

## Risks

- Two agent architectures now coexist.
- The prototype depends on a manually started local model server.

## Gaps

- No full conversational shell UX beyond task execution loop behavior.

## Next recommended improvements

- Decide whether `ptolemy-agent` remains a prototype or becomes a supported CLI.
- Document the relationship between this prototype and `/agent-runs`.

