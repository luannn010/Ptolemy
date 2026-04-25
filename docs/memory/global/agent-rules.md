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