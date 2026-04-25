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
