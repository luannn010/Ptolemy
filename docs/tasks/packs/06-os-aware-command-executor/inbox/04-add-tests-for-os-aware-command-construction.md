# 04 - Add Tests for OS-Aware Command Construction

## Goal
Add focused tests so the OS-aware shell behavior is protected against regression.

## Instructions
1. Add tests near the package where `shellCommand` lives.
2. Test command construction as much as possible without needing to fake the whole OS.
3. If direct `runtime.GOOS` testing is difficult, refactor minimally to allow testing with an internal helper like:

```go
func shellCommandForOS(ctx context.Context, goos string, command string) *exec.Cmd
```

Then:

```go
func shellCommand(ctx context.Context, command string) *exec.Cmd {
	return shellCommandForOS(ctx, runtime.GOOS, command)
}
```

4. Test:
   - Windows uses `powershell.exe`
   - Windows args include `-NoProfile`, `-ExecutionPolicy`, `Bypass`, `-Command`
   - Unix-like systems use `bash -lc`

## Acceptance Criteria
- Tests cover both Windows and non-Windows command construction.
- Tests are narrow and do not execute risky shell commands.
- `go test ./...` passes or unrelated failures are documented.
