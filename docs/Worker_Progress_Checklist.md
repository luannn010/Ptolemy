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
- [ ] read_file
- [ ] write_file
- [ ] list_directory
- [ ] search_codebase (ripgrep)
- [ ] apply_patch
- [ ] Restrict file paths

## Phase 5 — Git operations
- [ ] git_status
- [ ] git_diff
- [ ] git_checkout
- [ ] git_commit (conventional)
- [ ] git_push (approval required)

## Phase 6 — Worktree isolation
- [ ] Setup bare repo
- [ ] create_worktree
- [ ] remove_worktree
- [ ] Bind session → worktree
- [ ] Test parallel sessions

## Phase 7 — SQLite storage
- [ ] Sessions table
- [ ] Actions table
- [ ] Logs table
- [ ] Approvals table
- [ ] Add migrations

## Phase 8 — Policy engine
- [ ] Define allow / ask / deny rules
- [ ] Restrict network/download
- [ ] Restrict secrets access
- [ ] Add approval flow

## Phase 9 — Summarization
- [ ] Summarize command output
- [ ] Include exit code + duration
- [ ] Include changed files
- [ ] Keep summary short

## Phase 10 — Codex bridge
- [ ] Create codex-bridge service
- [ ] Define action schema
- [ ] Map actions → worker
- [ ] Return summaries
- [ ] Test full loop

## Phase 11 — MVP completion
- [ ] Open session
- [ ] Attach repo
- [ ] Edit file
- [ ] Run test
- [ ] Commit
- [ ] Push (with approval)

## Phase 12 — MCP adapter
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
