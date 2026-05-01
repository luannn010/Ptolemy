# Edit Action Validation Rules

Use this behavior as the source of truth.

## replace_block

Required fields:

- `target_file`
- `new_block`
- either `old_block` or `anchor`

If incomplete, do not execute. Return:

```text
Your replace_block action is incomplete. Return exactly one JSON object with target_file, old_block, and new_block.
```

If the implementation supports anchor-based replacement, it may instead request `target_file`, `anchor`, and `new_block`, but the prompt must ask for exactly one complete JSON object.

## insert_after

Required fields:

- `target_file`
- `anchor`
- `snippet`

If incomplete, do not execute. Return:

```text
Your insert_after action is incomplete. Return exactly one JSON object with target_file, anchor, and snippet.
```

## create_file

Required fields:

- `target_file`
- `content`

If incomplete, do not execute. Return:

```text
Your create_file action is incomplete. Return exactly one JSON object with target_file and content.
```

## update_file

Required fields:

- `target_file`
- either `content` or `patch`

If incomplete, do not execute. Return:

```text
Your update_file action is incomplete. Return exactly one JSON object with target_file and content or patch.
```

## Safety invariant

Validation must run before any filesystem write or command execution for the action. Incomplete edit actions must not change repository files.
