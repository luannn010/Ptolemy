# Decisions

## KB Storage
- Canonical repo memory lives under `.ptolemy/kb/`.
- `PROJECT_MAP.md`, `FILE_INDEX.json`, `SYMBOL_INDEX.json`, and `CHANGELOG.md` are generated or machine-updated.
- `WORKFLOWS.md` and `DECISIONS.md` are curated and should not be auto-rewritten from repo scans.

## Update Strategy
- Agents read the KB before broad repo inspection.
- Successful task packs update KB entries from the integration worktree diff.
- Failed task packs do not append KB changelog entries.
