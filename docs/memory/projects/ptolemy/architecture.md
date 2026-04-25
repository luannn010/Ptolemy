# Ptolemy Architecture

Ptolemy is a local worker / MCP execution server.

## Core Components

- HTTP API (chi router)
- TmuxRunner (isolated execution)
- SQLite (execution memory)
- MCP layer (external control)

## Memory Design

### SQLite (execution memory)

Stores:
- sessions
- command_logs
- actions
- logs
- approvals

Used for:
- debugging
- replay
- audit
- agent reasoning history

### Markdown (knowledge memory)

Stores:
- architecture
- conventions
- decisions
- lessons learned

Used for:
- guiding agent behavior
- cross-project reuse
