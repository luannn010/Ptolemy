# Worker Bot Progress Checklist

## Phase 0 — Project setup
- [x] Create repo for the worker platform
- [x] Create folders: cmd, internal, configs, state, deploy
- [x] Initialize Go module
- [x] Add config loader (.env or YAML)
- [x] Add logging
- [x] Add Makefile / task runner

## Phase 1 — Core worker daemon
- [ ] Create workerd entrypoint
- [ ] Add health check endpoint
- [ ] Add config parsing
- [ ] Add graceful shutdown
- [ ] Setup systemd service

## Phase 2 — Session management
- [ ] Define session model
- [ ] Implement open_session
- [ ] Implement close_session
- [ ] Persist session in SQLite
- [ ] Add session recovery

## Phase 3 — Terminal execution
- [ ] Setup tmux
- [ ] Create session per task
- [ ] Implement run_command
- [ ] Capture stdout/stderr
- [ ] Add timeout + interrupt
- [ ] Store command logs

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
