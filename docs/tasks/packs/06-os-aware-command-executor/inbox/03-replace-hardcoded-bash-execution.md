# 03 - Replace Hardcoded Bash Execution

## Goal
Replace hardcoded shell command creation with the new OS-aware helper.

## Instructions
1. Replace usages like:

```go
exec.CommandContext(ctx, "bash", "-lc", command)
```

with:

```go
shellCommand(ctx, command)
```

2. Ensure all executor paths that run user/project commands use the helper.
3. Do not replace unrelated command executions that intentionally call specific binaries like `git`, `go`, or test tools directly unless they are part of the generic shell execution path.

## Validation
Run:

```bash
go test ./...
```

Then build:

```bash
go build -o ./bin/workerd ./cmd/workerd
```

On Windows:

```powershell
go build -o .\bin\workerd.exe .\cmd\workerd
```

## Acceptance Criteria
- Generic shell execution is OS-aware.
- Linux/macOS still use Bash.
- Windows uses PowerShell.
- Build passes.
