# Ptolemy Execute Metadata

This task pack closes the remaining metadata gap on the Codex-facing `ptolemy.execute` path.

It extends the MCP executor tool and `/execute` so live Ptolemy calls can store:

- `title`
- `purpose`
- `reasoning_step`
- `target`

The pack is complete when a real `ptolemy.execute`-style call writes those keys into the latest `command.exec` action row in `state/ptolemy.db`.
