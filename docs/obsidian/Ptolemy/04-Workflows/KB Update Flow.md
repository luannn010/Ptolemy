---
project: Ptolemy
category: workflows
status: Partial
last_verified: 2026-05-11
source: codebase
confidence: high
tags:
  - ptolemy
  - workflows
---

# KB Update Flow

The current KB update flow is:

1. Normalize changed file paths.
2. Rebuild or refresh file index and symbol index entries.
3. Rewrite `.ptolemy/kb/PROJECT_MAP.md`, `FILE_INDEX.json`, and `SYMBOL_INDEX.json`.
4. Sync compatibility artifacts such as `.ptolemy/index/file-tree.json`.
5. Optionally append a changelog entry for a completed pack.

Evidence:

- `internal/navigator/kb.go`
- `.ptolemy/kb/*`
- `internal/httpapi/router.go`

Related notes:

- [[../02-Implementation-Aspects/Knowledge Base and Memory]]

