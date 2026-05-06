# 01 - Discover Current Executor Shell Path

## Goal
Find where Ptolemy currently creates shell commands and identify all hardcoded Unix shell assumptions.

## Instructions
1. Inspect the repository for command execution code.
2. Search for:
   - `exec.CommandContext`
   - `bash`
   - `-lc`
   - command runner/executor packages
3. Identify the smallest file/package where an OS-aware helper should live.

## Suggested Commands

```bash
grep -R "exec.CommandContext" -n .
grep -R '"bash"' -n .
grep -R '\-lc' -n .
find internal -maxdepth 3 -type f | sort
```

## Output Required
Create a short note in the task result describing:
- the file(s) that create shell commands
- which usages need patching
- whether there is already a helper function to reuse

## Acceptance Criteria
- Exact executor file path is identified.
- No code changes unless needed for discovery.
