# Worker Bot Progress Checklist

## Phase 0 — Project setup
- [x] Create repo for the worker platform
- [x] Create folders: cmd, internal, configs, state, deploy
- [x] Initialize Go module
- [x] Add config loader (.env or YAML)
- [x] Add logging
- [x] Add Makefile / task runner

## Phase 1 — Core worker daemon
- [x] Create workerd entrypoint
- [x] Add health check endpoint
- [x] Add config parsing
- [x] Add graceful shutdown
- [x] Setup systemd service

## Phase 2 — Session management
- [x] Define session model
- [x] Implement open_session
- [x] Implement close_session
- [x] Persist session in SQLite
- [x] Add session recovery

## Phase 3 — Terminal execution
- [x] Setup tmux
- [x] Create session per task
- [x] Implement run_command
- [x] Capture stdout/stderr cleanly
- [x] Add timeout handling
- [x] Store command logs in SQLite
- [x] Live tmux session per worker session
- [x] Automated tests

## Phase 4 — File operations
- [x] read_file
- [x] write_file
- [x] list_directory
- [x] search_codebase (ripgrep)
- [x] apply_patch (basic)
- [x] Restrict file paths

## Phase 5 — Codex Adapter
- [x] Create `cmd/ptolemy-mcp`
- [x] Add adapter config: worker URL
- [x] Implement JSON-RPC stdio loop
- [x] Add `tools/list`
- [x] Add `tools/call`
- [x] Map tools to existing HTTP endpoints
- [x] Test with manual JSON-RPC input
- [x] Add Codex MCP config

## Phase 6 — Git operations
- [x] Create internal/gitops module
- [x] Implement git_status
- [x] Implement git_diff
- [x] Implement git_log
- [x] Implement git_checkout
- [x] Implement git_create_branch
- [x] Implement git_commit (conventional)
- [x] Implement git_push (approval required)
- [x] Expose Git endpoints (HTTP)
- [x] Expose Git MCP tools

## Phase 7 — Worktree isolation
- [x] Setup bare repo
- [x] create_worktree
- [x] remove_worktree
- [x] Auto branch per session
- [x] Prevent session collision
- [x] Bind session → worktree
- [x] Test parallel sessions

## Phase 8 — Agent Memory Layer

SQLite execution memory
- [x] Keep sessions table
- [x] Keep command_logs table
- [x] Add actions table
- [x] Add logs table
- [x] Add approvals table
- [x] Add indexes
- [x] Add metadata JSON fields

Migrations
- [x] Create basic migration runner
- [x] Add schema_migrations table
- [x] Auto-run migrations on startup

Execution integration
- [x] Before command runs → create action pending
- [x] After command runs → update action success/failed
- [x] Important events → write logs
- [ ] Dangerous actions → create approval record

## Phase 8.5 — Markdown Knowledge Memory

Structure
- [x] Create memory folder structure
- [x] Add global agent rules
- [x] Add project-level memory

Content
- [x] Architecture notes
- [x] Conventions
- [x] Important decisions
- [x] Known issues

Integration
- [x] Add file loader utility
- [x] Add "load memory before execution"

Markdown knowledge memory
- [x] Add knowledge/global/
- [x] Add knowledge/projects/
- [x] Add project architecture notes
- [x] Add important decisions notes
- [x] Add agent-readable conventions



## Phase 9 — Policy Engine + Local Brain

Foundation
- [ ] Create workspace inspector
- [ ] Detect OS / WSL / CPU / RAM / disk / GPU
- [ ] Detect project type: Go, Node, Python, Java, Docker
- [ ] Save workspace snapshot into SQLite or Markdown
- [ ] Use snapshot in agent prompt

Local brain
- [ ] Use llama.cpp server
- [ ] Add Gemma 4 E2B config
- [ ] Add simple client to call local model

Policy
- [ ] Define allow / ask / deny rules
- [ ] Block dangerous commands
- [ ] Create approval record

Agent loop
- [ ] observe → think → act → observe
- [ ] max step limit
- [ ] failure recovery
- [ ] save execution traces

## Phase 10 — Summarization
- [ ] Summarize command output
- [ ] Include exit code + duration
- [ ] Include changed files
- [ ] Keep summary short

## Phase 11 — Codex bridge
- [ ] Create codex-bridge service
- [ ] Define action schema
- [ ] Map actions → worker
- [ ] Return summaries
- [ ] Test full loop

## Phase 12 — MVP completion
- [ ] Open session
- [ ] Attach repo
- [ ] Edit file
- [ ] Run test
- [ ] Commit
- [ ] Push (with approval)

## Phase 13 — MCP adapter
- [ ] Expose tools via MCP
- [ ] Support stdio
- [ ] Add HTTP later

---

## First Milestone (Must Achieve)
- [ ] Open session
- [ ] Run command
- [ ] Capture output
- [ ] Read + edit file
- [ ] Run git status
- [ ] Save logs

---

## Weekly Checkpoints

### Week 1 — Executor
- [ ] Worker runs
- [ ] Commands execute
- [ ] Output captured

### Week 2 — Repo + File Ops
- [ ] Git works
- [ ] File edit works

### Week 3 — Isolation + Safety
- [ ] Worktrees working
- [ ] Policy enforced

### Week 4 — Codex Loop
- [ ] Full loop working
- [ ] Tasks executed end-to-end
