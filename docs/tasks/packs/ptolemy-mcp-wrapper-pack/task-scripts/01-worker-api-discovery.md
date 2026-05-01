# Task Script: Worker API Discovery

Confirm the existing worker API before wrapper changes:

1. read `docs/Worker_API.md`
2. inspect `internal/httpapi/router.go`
3. inspect the session and execute handlers
4. note that `/agent/run` is optional and absent unless the worker explicitly adds it later
5. keep discovery notes pack-local unless a repo doc needs a precise wrapper clarification
