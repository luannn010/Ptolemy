# Patch Spec Workflow

Structured patch specs are the intended future replacement for fragile text edits.

Example:

```yaml
type: insert_after
file: cmd/ptolemy-agent/main.go
anchor: "// PTOLEMY: INSERT ACTION CASES HERE"
content: |
  case "insert_after":
```

Status: planned. Full patch-spec validation is not implemented yet.

Available targeted edit primitives today:

- `replace_block`: replace the first exact old-text match in a file.
- `insert_after`: insert content after the first exact marker match in a file.

These are exposed through `ptolemy-agent`, worker HTTP file endpoints, and MCP tools. Use them before falling back to whole-file `write_file` or `apply_patch` replacement.
