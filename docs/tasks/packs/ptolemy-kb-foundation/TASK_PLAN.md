# Task Plan: Ptolemy KB Foundation

## Goal

Refactor the navigator and memory loader so `.ptolemy/kb/` becomes the canonical repo memory store and `.ptolemy/context` plus `.ptolemy/index/file-tree.json` remain compatibility outputs.

## Execution Strategy

- Use sequential-first execution.
- Keep scope centered on `internal/navigator`, `internal/memory`, and `.ptolemy`.
- Prefer deterministic generated content over prompt-authored context refreshes.
- Preserve existing navigator entrypoints while changing the underlying storage model.

## Validation

```bash
/usr/local/go/bin/go test ./internal/navigator ./internal/memory
```
