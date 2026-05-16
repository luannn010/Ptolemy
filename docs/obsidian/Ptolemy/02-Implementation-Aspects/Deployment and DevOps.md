---
project: Ptolemy
category: deployment and devops
status: Partial
last_verified: 2026-05-11
source: codebase
confidence: medium
tags:
  - ptolemy
  - implementation
---

# Deployment and DevOps

## What this aspect means

This covers runtime configuration, service startup, environment variables, deployment packaging, and operational automation.

## Current implementation status

Status: `Partial`

Ptolemy has environment-based configuration and a systemd unit for `workerd`. I did not find broader deployment automation such as container builds, Kubernetes manifests, or visible CI/CD definitions in the requested inspection areas.

## Evidence from codebase

| Evidence | Path | Notes |
|---|---|---|
| Environment config loader | `internal/config/config.go` | Runtime configuration surface |
| Example env files | `.env.example`, `.env` | Local configuration hints |
| systemd unit | `deploy/workerd.service` | Service deployment artifact |
| Worker startup contract | `cmd/workerd/main.go` | Reads config and starts service |

## Ready-to-use capabilities

- Local env-driven configuration
- Systemd deployment path for `workerd`

## Partial or unfinished capabilities

- No verified container or CI/CD layer
- No deployment docs under `deploy/` beyond the service file

## APIs / interfaces

| Interface | Path / Endpoint / Command | Purpose | Status |
|---|---|---|---|
| Config loader | `internal/config.Load()` | Read env config | Ready |
| Service unit | `deploy/workerd.service` | Run `workerd` under systemd | Partial |

## Important commands

```bash
sudo systemctl restart workerd
sudo systemctl status workerd
```

## Tests and validation

| Test / command | What it validates | Current result |
|---|---|---|
| `internal/config/config_test.go` | Config loading behavior | Test file present |
| Live service commands | systemd management | Not executed in this audit |

## Risks

- Deployment knowledge appears localized and environment-specific.

## Gaps

- No CI/CD config verified.
- No portable deployment bundle verified.

## Next recommended improvements

- Add deployment documentation for local, service, and future container modes.
- Add a deployment smoke-test checklist.

