# File Search / Read Workflow

Search before reading full files.

```text
Agent
  -> POST /file/search
  -> Choose top relevant files
  -> POST /file/read with optional task_session_id
  -> Ptolemy records the read in .ptolemy/sessions/<id>/files-read.json
```

Status: working.
