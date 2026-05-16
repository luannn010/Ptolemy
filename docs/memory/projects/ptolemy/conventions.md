# Ptolemy Conventions

## API

- JSON input/output only
- All handlers must validate input
- Return structured errors

## Execution

- All commands must go through runner
- No direct os/exec in handlers

## Logging

- Use zerolog
- Include session_id and action_id

## Agent Execution Budgets

| Task type | Recommended max steps |
|---|---|
| Fixed command / bootstrap script | 3 |
| Safe test file creation/update | 4 |
| Single-file code edit | 5 |
| Standard coding task | 8 |
| Multi-file change / debugging | 10 |

- Prefer smaller step budgets first.
- Increase step budget only when the task genuinely needs more tool cycles.
- Repeated read_file without progress means the agent lacks enough context or the task needs a more specific instruction.
