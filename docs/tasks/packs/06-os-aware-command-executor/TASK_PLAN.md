# Task Plan: OS-Aware Command Executor

## Pack ID
06-os-aware-command-executor

## Goal
Make Ptolemy's command executor OS-aware so `workerd` can run commands correctly on both Linux/macOS and Windows.

## Problem
The current command executor appears to assume a Unix shell:

```go
exec.CommandContext(ctx, "bash", "-lc", command)
```

This fails or behaves incorrectly on Windows because `bash` is usually unavailable unless Git Bash, WSL, or MSYS2 is installed.

## Desired Outcome
Ptolemy should choose the correct shell based on the operating system:

- Windows: `powershell.exe -NoProfile -ExecutionPolicy Bypass -Command <command>`
- Linux/macOS: `bash -lc <command>`

## Scope
Implement a small OS-aware shell command helper and replace direct hardcoded shell calls.

## Non-goals
- Do not redesign the executor system.
- Do not add a full shell abstraction framework.
- Do not change MCP behavior.
- Do not change task-pack execution behavior.
- Do not change security/approval logic unless tests require minor compatibility updates.

## Expected Files to Inspect
Start by searching for hardcoded command execution:

```bash
grep -R "exec.CommandContext" -n .
grep -R "bash.*-lc" -n .
grep -R "powershell" -n .
```

Likely areas:
- `internal/executor`
- `internal/command`
- `internal/action`
- `cmd/workerd`

## Implementation Strategy
1. Find the command execution helper or package.
2. Add an OS-aware shell helper.
3. Replace hardcoded `bash -lc` usages with the helper.
4. Add tests for command construction.
5. Rebuild `workerd`.
6. Smoke test command execution on the current OS.

## Proposed Helper

```go
func shellCommand(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(
			ctx,
			"powershell.exe",
			"-NoProfile",
			"-ExecutionPolicy", "Bypass",
			"-Command", command,
		)
	}

	return exec.CommandContext(ctx, "bash", "-lc", command)
}
```

Required imports:

```go
import (
	"context"
	"os/exec"
	"runtime"
)
```

Only include imports that are needed in the final file.

## Validation Commands

```bash
go test ./...
go build -o ./bin/workerd ./cmd/workerd
```

Windows build command:

```powershell
go build -o .\bin\workerd.exe .\cmd\workerd
```

## Runtime Smoke Test

Start worker:

```powershell
.\bin\workerd.exe
```

Check health:

```powershell
curl http://localhost:8080/health
```

Create or reuse a session, then test Windows command execution:

```powershell
curl -X POST http://localhost:8080/execute `
  -H "Content-Type: application/json" `
  -d '{
    "session_id": "YOUR_SESSION_ID",
    "command": "Get-Location"
  }'
```

Linux smoke test:

```bash
curl -s -X POST http://localhost:8080/execute \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "YOUR_SESSION_ID",
    "command": "pwd"
  }'
```

## Acceptance Criteria
- `workerd` builds on Linux/macOS.
- `workerd.exe` builds on Windows.
- Existing tests pass, or unrelated failures are clearly documented.
- Hardcoded `bash -lc` usage is removed from the command executor path.
- Windows command execution works with PowerShell.
- Linux/macOS command execution still works with Bash.
- No unrelated files are modified.
