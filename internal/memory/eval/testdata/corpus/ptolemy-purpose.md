---
published_at: 2024-01-01T00:00:00Z
---
# Ptolemy purpose

Ptolemy is a Go-based agent runtime being rebuilt clean-room as v2.
The policy harness gates every side-effecting operation (shellcmd,
fileops, gitops, worktrees) behind hybrid approvals — in-band tokens
for low-risk commands, out-of-band approval for high-risk ones.
