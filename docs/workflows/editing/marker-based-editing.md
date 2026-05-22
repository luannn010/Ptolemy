# Marker-Based Editing Workflow

Improve reliability of edits by using stable anchors.

```text
Developer or agent inserts a marker
  -> Agent locates marker
  -> Agent uses insert_after or ptolemy.insert_after
  -> Ptolemy writes the targeted edit
  -> Tests run immediately
```

Example marker:

```go
// PTOLEMY: INSERT ROUTES HERE
```

Supported entrypoints:

- `ptolemy-agent` action: `insert_after`
- HTTP: `POST /file/insert_after`
- MCP: `ptolemy_insert_after`

For replacing existing code, prefer the first exact-match targeted replacement before whole-file writes:

- `ptolemy-agent` action: `replace_block`
- HTTP: `POST /file/replace_block`
- MCP: `ptolemy_replace_block`

Status: supported by `ptolemy-agent`, worker HTTP file tools, and MCP file tools.
