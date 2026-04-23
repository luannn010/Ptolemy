# MVP Design

## Core Features
- run_command
- read/write file
- search code
- git status/diff/commit
- session management

## Session Model
- one session = one tmux + one worktree

## Tools
- open_session
- run_command
- read_file
- apply_patch
- git_commit

## Policy
Allow:
- read/edit files
- run tests

Ask:
- git push
- install packages

Deny:
- secrets access
- system-level changes

