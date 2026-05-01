# 02 - Add OS-Aware Shell Command Helper

## Goal
Add a small OS-aware command helper so command execution works on both Windows and Unix-like systems.

## Required Behavior
When `runtime.GOOS == "windows"`, commands should run through:

```go
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command <command>
```

Otherwise commands should run through:

```go
bash -lc <command>
```

## Suggested Implementation

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

## Instructions
1. Add the helper near the existing executor command creation logic.
2. Add imports only where needed:
   - `runtime`
   - `os/exec`
   - `context` if not already imported
3. Keep the helper unexported unless the package already uses exported helper patterns.
4. Do not rewrite executor architecture.

## Acceptance Criteria
- Helper compiles.
- Helper is placed in the executor/command package where it is naturally used.
- No unrelated behavior changes.
