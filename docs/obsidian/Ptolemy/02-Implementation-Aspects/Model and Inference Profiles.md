---
project: Ptolemy
category: model and inference profiles
status: Partial
last_verified: 2026-05-11
source: codebase
confidence: medium
tags:
  - ptolemy
  - implementation
---

# Model and Inference Profiles

## What this aspect means

This covers how Ptolemy chooses model endpoints, model names, timeouts, and execution-phase reasoning profiles.

## Current implementation status

Status: `Partial`

The codebase supports a configurable local brain endpoint and model name, plus controller-owned reasoning profiles for agent-run phases. It does not yet look like a broad inference-profile catalog with multiple providers or advanced routing.

## Evidence from codebase

| Evidence | Path | Notes |
|---|---|---|
| Config defaults | `internal/config/config.go` | `BRAIN_BASE_URL`, `BRAIN_MODEL` defaults |
| Brain client | `internal/brain/client.go` | OpenAI-compatible chat API client |
| Reasoning profile policy | `internal/agentloop/reasoning_profile.go` | `low`, `normal`, `high` mapped by phase |
| Reasoning profile tests | `internal/agentloop/reasoning_profile_test.go` | Profile policy is tested at file level |

## Ready-to-use capabilities

- Local model endpoint configuration
- Worker-owned reasoning profile resolution for phases

## Partial or unfinished capabilities

- Only one main model path is obvious in runtime config
- No provider selection layer was found

## APIs / interfaces

| Interface | Path / Endpoint / Command | Purpose | Status |
|---|---|---|---|
| Config | `BRAIN_BASE_URL`, `BRAIN_MODEL` | Brain target selection | Ready |
| Brain client | `internal/brain.Client` | Chat completion calls | Partial |
| Reasoning policy | `internal/agentloop.PolicyGuard` | Prevents model override | Partial |

## Important commands

```bash
curl -s http://127.0.0.1:8088/v1/chat/completions
```

## Tests and validation

| Test / command | What it validates | Current result |
|---|---|---|
| `internal/brain/client_test.go` | Brain client behavior | Test file present |
| `internal/agentloop/reasoning_profile_test.go` | Reasoning-profile mapping | Test file present |

## Risks

- Model configuration is sensitive to local environment drift.

## Gaps

- No broader model profile inventory or fallback strategy.

## Next recommended improvements

- Add a documented matrix of supported model endpoints and intended use phases.
- Add startup health checks for the configured brain endpoint.

