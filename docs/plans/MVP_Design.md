# MVP Design

## Goal
Run autonomous coding tasks with:
- Codex (planner)
- Gemma (executor)
- Ptolemy (runtime)

## Core Flow

1. Codex creates task
2. Gemma executes:
   - read files
   - edit code
   - run tests
3. Ptolemy runs commands
4. Output summarized
5. Codex validates

## Output Format

{
  "status": "completed",
  "summary": "Tests fixed",
  "steps": [
    {"tool": "read_file", "summary": "inspected file"},
    {"tool": "apply_patch", "summary": "fixed bug"},
    {"tool": "execute", "summary": "tests passed"}
  ]
}

## Key Principles

- minimal tokens
- structured output
- logs stored locally
- parallel execution ready
