---
project: Ptolemy
category: prompts
status: Partial
last_verified: 2026-05-11
source: codebase
confidence: medium
tags:
  - ptolemy
  - prompts
---

# Future Codex Prompts

## Audit refresh prompt

Audit the current Ptolemy codebase again and refresh `docs/obsidian/Ptolemy/` using code evidence only. Re-run `go test ./...` if Go is available and update the test report note with the exact result.

## Runtime validation prompt

Validate the current Ptolemy runtime end to end: start `workerd`, call `/health`, create a session, run a simple command, and update the Obsidian docs with verified API examples. Do not change runtime code unless a failure is reproduced first.

## KB validation prompt

Exercise the KB flow by calling `/kb/build`, `/kb/read`, and `/kb/update`, then update the implementation notes with the exact observed outputs and any drift between code and docs.

## Pack Studio prompt

Run the Pack Studio UI locally, verify `/ui` and `/ui/api/program-runs/*` behavior, and update the UI and Observability notes with live validation findings.

