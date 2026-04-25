# Ptolemy Architecture

## Overview

Codex = planner
Gemma = executor
Ptolemy = runtime

## Flow

Codex → Task
→ Gemma (local loop)
→ MCP tools
→ Ptolemy worker
→ tmux / fileops / git
→ result (JSON summary)
→ Codex validates

## Components

- workerd (Go)
- tmux sessions
- fileops
- gitops (future)
- MCP adapter
- SQLite storage

## Future

- parallel sessions
- job orchestration
- policy enforcement
