---
published_at: 2024-02-01T00:00:00Z
---
# Approval policy (v1)

For shell commands the agent uses in-band tokens for low-risk calls
and out-of-band approval for high-risk ones. The token TTL is 5 minutes.
The agent never self-approves; the worker console is the approval surface.
