# Known Issues

- /execute route and /sessions/{id}/commands route are separate execution paths.
- Phase 8 action logging is currently hooked into command route first.
- Approval table exists, but dangerous command enforcement belongs to Phase 9.
- Markdown memory is loaded before command execution, but not yet injected into an LLM prompt.
