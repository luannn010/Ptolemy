# Agent Rules

## Execution Rules

- Always log every command execution into SQLite (actions + logs).
- Never run destructive commands without approval:
  - rm -rf
  - git reset --hard
  - docker system prune
  - database reset

## Behavior

- Prefer small, safe, reversible commands.
- If a command fails, inspect the error before retrying.
- Avoid repeating failed commands without modification.

## Git Rules

- Always check git status before commit.
- Never push without explicit approval.
- Use descriptive commit messages.

## Memory Model

- SQLite = execution history
- Markdown = knowledge memory

## Step Budget Policy

- Use `--max-steps 3` for deterministic bootstrap tasks, fixed command execution, or simple validation.
- Use `--max-steps 4` to `--max-steps 5` for small single-file edits or simple read → edit → validate flows.
- Use `--max-steps 8` for normal coding tasks that require reading files, editing, formatting, and testing.
- Use `--max-steps 10` only for multi-file edits, policy updates, or debugging tasks.
- Avoid using more than 10 steps unless explicitly approved by the user.
- Stop early when the task is complete.
- If the agent repeats the same action twice without progress, stop and report the loop.
- If the agent hits max steps, summarize what was completed and what remains.