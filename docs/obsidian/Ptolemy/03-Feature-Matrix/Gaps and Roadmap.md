---
project: Ptolemy
category: feature-matrix
status: Partial
last_verified: 2026-05-11
source: codebase
confidence: high
tags:
  - ptolemy
  - roadmap
---

# Gaps and Roadmap

## Main gaps from the current code audit

- Repo-wide tests could not be run here because `go` was unavailable.
- Security policy is still simple and string-based.
- There are overlapping agent execution paths between `ptolemy-agent` and `internal/agentloop`.
- Pack Studio and task-pack orchestration look substantial, but live execution confidence is still lower than the code surface suggests.
- Distributed workers and a formal skills subsystem are not implemented as first-class runtime modules.

## Practical next roadmap items

1. Make test execution reliable in developer and CI environments.
2. Decide the canonical agent execution path.
3. Harden approval and policy enforcement.
4. Add Pack Studio browser and program-run smoke tests.
5. Clarify compatibility vs canonical KB/navigator APIs.
6. Add deployment automation beyond the systemd unit.
7. Define whether distributed execution is actually in scope.

Related notes:

- [[Feature Inventory]]
- [[../07-Test-Reports/Current Test Status]]

