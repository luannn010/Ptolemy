---
published_at: 2024-02-01T00:00:00Z
---
# deny-policy-write rule

The policy engine has two self-protection rules that must never be
loosened: deny-policy-write blocks writes to .ptolemy/policy.json, and
deny-secret-* blocks reads of secrets. New rules are always allow or ask;
never weaken a deny.
