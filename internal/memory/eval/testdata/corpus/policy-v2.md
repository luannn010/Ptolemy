---
published_at: 2026-01-01T00:00:00Z
---
# Approval policy (v2)

For shell commands the agent uses in-band tokens for low-risk calls
and out-of-band approval for high-risk ones. The token TTL is now 15
minutes (extended from 5 minutes in v1 after operator feedback). The
agent never self-approves; the worker console is the approval surface.
