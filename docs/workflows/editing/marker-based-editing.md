# Marker-Based Editing Workflow

Improve reliability of edits by using stable anchors.

```text
Developer or agent inserts a marker
  -> Agent locates marker
  -> Agent uses insert_after
  -> Ptolemy writes the targeted edit
  -> Tests run immediately
```

Example marker:

```go
// PTOLEMY: INSERT ROUTES HERE
```

Status: supported by `ptolemy-agent` insert-after behavior.
