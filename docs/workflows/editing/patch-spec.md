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

Status: planned. Basic content replacement exists through file tools, but full patch-spec validation is not implemented yet.
