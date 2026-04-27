# Ptolemy Workflows

This document defines the core execution and development workflows supported by Ptolemy.
It is designed for **agent-driven development (Codex/LLM)** with deterministic, low-risk operations.

---

## 1. Session Workflow

Create and manage isolated execution environments.

### Flow

```
Client / Agent
  -> POST /sessions
  -> Ptolemy creates session
  -> Session stored in SQLite
  -> Workspace attached
```

### Purpose

* Isolate execution per task/agent
* Bind all commands to a specific workspace

---

## 2. Command Execution Workflow

Execute shell commands safely within a session.

### Flow

```
Client / Agent
  -> POST /sessions/{id}/commands
  -> Validate session
  -> Execute command (cwd, timeout)
  -> Return stdout / stderr / exit code
```

### Purpose

* Controlled command execution
* Used by Codex to build, test, run code

---

## 3. Health Check Workflow

Verify worker availability.

### Flow

```
Client
  -> GET /health
  -> Worker responds OK
```

### Purpose

* Debug worker status
* Detect crashed or unavailable worker

---

## 4. Persistent Session Workflow

Sessions survive restarts.

### Flow

```
Worker starts
  -> Load sessions from SQLite
  -> Sessions reusable
  -> Sessions can be closed
```

### Purpose

* Long-running workflows
* Resume agent tasks

---

## 5. Terminal Runner Workflow

Abstract execution layer.

### Flow

```
HTTP API
  -> Command Service
  -> Terminal Runner
  -> OS Shell
```

### Variants

* Basic runner (stateless)
* Tmux runner (stateful, planned)

### Purpose

* Support both short and long-running processes
* Future: persistent shells

---

## 6. Worktree Workflow

Isolated Git environments per task.

### Flow

```
Client / Agent
  -> Request worktree creation
  -> Validate session
  -> Create git worktree (branch/name)
```

### Purpose

* Prevent modifying main branch
* Enable parallel development

---

## 7. Codex Execution Workflow

Ptolemy as execution backend for LLM.

### Flow

```
Codex
  -> Reads TASK.md / AGENT.md
  -> Sends commands to Ptolemy
  -> Ptolemy executes
  -> Returns result
  -> Codex updates code
  -> Repeat
```

### Purpose

* Human-out-of-the-loop coding
* Deterministic execution layer

---

## 8. Task-File Driven Workflow

Structured instructions instead of free-form prompts.

### Files

* AGENT.md → behavior rules
* TASK.md → specific objective

### Flow

```
Agent
  -> Reads TASK.md
  -> Understands scope
  -> Executes steps via Ptolemy
```

### Purpose

* Reduce token usage
* Increase reliability
* Improve reproducibility

---

## 9. Code Update Workflow (Insert-After Strategy)

Deterministic code modification without rewriting files.

### Strategy

```
1. Locate anchor
2. Identify insertion point
3. Insert new code AFTER anchor
4. Save file
5. Run test/build
```

### Example

```
anchor: "func (h *Handler) Create"
action: insert_after
```

### Supported Anchors

* Function signature
* Specific string
* Comment marker (recommended)

### Example Marker

```go
// PTOLEMY: INSERT ROUTES HERE
```

### Purpose

* Safe code edits
* Avoid breaking structure
* Works with large codebases

---

## 10. Patch Execution Workflow (Planned)

Structured patch system for agents.

### Flow

```
Agent
  -> Generates PATCH spec
  -> Sends to Ptolemy
  -> Ptolemy applies patch
  -> Runs command/test
```

### Patch Example

```yaml
type: insert_after
file: internal/httpapi/handler.go
anchor: "func (h *Handler) Create"
content: |
  log.Info().Msg("created")
```

### Purpose

* Standardize code edits
* Enable automation
* Replace fragile text edits

---

## 11. Execution Failure Workflow (Current Debug State)

Known issue:

```
POST /sessions works
POST /execute fails (EOF)
```

### Interpretation

* HTTP layer OK
* Worker execution layer failing

### Next Step

* Verify worker process
* Check execution handler
* Restart or debug command runner

---

## Design Principles

```
- Deterministic over “smart”
- File-based over prompt-based
- Safe edits over full rewrites
- Local-first execution
- Agent-compatible architecture
```

---

## Future Extensions

* AST-based patch engine
* Multi-file patch support
* Context indexing per project
* Distributed worker nodes
* Skill execution (Python/Go hybrid)

---
## 12. Marker-Based Editing Workflow

Improve reliability of code edits by using stable markers instead of fragile text matching.

### Strategy

1. Developer inserts marker in code  
2. Agent locates marker  
3. Agent uses insert_after  
4. Ptolemy modifies file safely  

### Example

```go



---

## 13. Patch Spec Workflow

Introduce structured patch instructions instead of raw text edits.

### Flow

Agent  
→ Generates patch spec  
→ Ptolemy validates patch  
→ Applies patch via edit tools  
→ Runs test/build  

### Example

```yaml
type: insert_after
file: cmd/ptolemy-agent/main.go
anchor: "// PTOLEMY: INSERT ACTION CASES HERE"
content: |
  case "insert_after":
```


## 14. Planner vs Executor Workflow

Separate reasoning from execution.

### Flow

Gemma (Planner)  
→ Reads TASK.md  
→ Generates plan / patch spec  

Ptolemy (Executor)  
→ Validates plan  
→ Executes actions  
→ Runs commands/tests  
→ Returns results  

### Purpose

- Reduce LLM error rate  
- Make execution deterministic  
- Allow debugging at each layer     