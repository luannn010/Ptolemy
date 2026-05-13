# Ptolemy Distributed Worker Orchestration Plan

## 1. Objective

Build a system where:

* Codex = planner (reasoning, task generation)
* Ptolemy = execution layer
* Multiple worker nodes = parallel execution across devices

Goal:

* Reduce token usage
* Increase execution throughput
* Enable scalable, parallel code operations

---

## 2. High-Level Architecture

Codex (Planner)
↓
Ptolemy Orchestrator (Control Layer)
↓
Worker Nodes (Execution Layer)

Worker Devices:

* RTX3050 PC (GPU + LLM)
* Debian Server (stable backend execution)
* MacBook M1 (CPU inference + light tasks)
* WSL Machine (frontend / Windows tasks)

---

## 3. Core Components

### 3.1 Orchestrator (NEW)

Responsibilities:

* Job queue management
* Worker registry
* Capability-based routing
* Concurrency control
* Approval system
* Result aggregation

### 3.2 Worker Node (EXISTING Ptolemy)

Responsibilities:

* Execute commands
* Read/write files
* Run tests/builds
* Return structured results

### 3.3 Memory Layer

* SQLite (actions, logs, approvals)
* Markdown memory:

  * /docs/memory/global
  * /docs/memory/projects/{project}

---

## 4. Execution Model

### Step Flow:

1. Codex generates task plan
2. Orchestrator receives task
3. Orchestrator assigns job to worker
4. Worker executes in isolated workspace
5. Worker returns structured result
6. Orchestrator summarizes result
7. Codex evaluates and continues

---

## 5. Worker Registry (Example)

```json
{
  "worker_id": "rtx3050-pc",
  "host": "http://100.x.x.x:8080",
  "capabilities": ["gpu", "llm", "docker", "node", "go"],
  "status": "idle",
  "max_parallel_jobs": 2
}
```

---

## 6. Job Definition

```json
{
  "job_id": "job-001",
  "type": "test",
  "required_capabilities": ["go"],
  "workspace": "/projects/worktrees/job-001",
  "commands": ["go test ./..."],
  "risk_level": "safe"
}
```

---

## 7. Concurrency Strategy

Allowed:

* Parallel read operations
* Parallel test/build jobs
* Parallel execution across workers

Restricted:

* No shared workspace writes
* No concurrent git push

Rule:
→ One worktree per job

Example:

* /worktrees/job-001
* /worktrees/job-002

---

## 8. Task Routing Strategy

| Task Type      | Assigned Worker      |
| -------------- | -------------------- |
| LLM inference  | RTX3050 PC           |
| Backend tests  | Debian server        |
| Frontend build | WSL / Windows        |
| File analysis  | Any available worker |

---

## 9. Safety & Approval System

Safe:

* read files
* run tests
* git status/diff

Needs Approval:

* git commit
* git push
* file deletion
* package install

Blocked:

* deleting system directories
* exposing secrets
* unknown scripts

---

## 10. Future Enhancements

* Result caching
* Semantic context retrieval
* Distributed job queue (Redis / NATS)
* Worker auto-scaling
* Failure recovery (retry / fallback worker)
* DAG-based execution planning

---

## 11. Development Roadmap

Stage 1: Stable single worker execution
Stage 2: Add structured execution API
Stage 3: Add worker registry
Stage 4: Add job queue
Stage 5: Add worktree isolation
Stage 6: Add parallel execution
Stage 7: Add orchestration intelligence

---

## 12. Key Design Principle

Codex MUST NOT:

* execute commands
* read entire codebase
* handle raw logs

Codex SHOULD:

* plan tasks
* request execution
* evaluate summarized results

Ptolemy MUST:

* execute safely
* manage state
* return structured outputs

---

## 13. Success Criteria

* 10x–50x reduction in token usage
* Parallel execution across devices
* Stable execution without conflicts
* Clear separation: planning vs execution

---
