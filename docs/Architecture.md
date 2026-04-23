# Architecture

## Overview
Codex = planner/verifier  
Worker = executor  
MCP = future tool layer  

## Components
- Codex App Server (JSON-RPC)
- Codex Bridge (Go)
- Worker Daemon (Go)
- tmux (sessions)
- Git worktree (repo isolation)
- SQLite (state)

## Flow
1. Codex plans
2. Bridge translates
3. Worker validates
4. Worker executes
5. Worker summarizes
6. Codex verifies

## Topology
Codex -> Bridge -> Worker -> (tmux + git + fs)

