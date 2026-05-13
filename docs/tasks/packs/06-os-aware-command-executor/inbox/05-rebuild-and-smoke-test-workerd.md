# 05 - Rebuild and Smoke Test Workerd

## Goal
Rebuild `workerd` and verify command execution works on the current operating system.

## Instructions
1. Run tests:

```bash
go test ./...
```

2. Build Linux/macOS binary:

```bash
go build -o ./bin/workerd ./cmd/workerd
```

3. Build Windows binary if on Windows or cross-compiling:

```powershell
go build -o .\bin\workerd.exe .\cmd\workerd
```

4. Restart worker.

Windows:

```powershell
.\bin\workerd.exe
```

Linux:

```bash
./bin/workerd
```

5. Check health:

```bash
curl http://localhost:8080/health
```

6. Create a session if needed.

Linux/macOS:

```bash
curl -s -X POST http://localhost:8080/sessions/ \
  -H "Content-Type: application/json" \
  -d '{
    "name": "executor-smoke-test",
    "workspace": "'"$PWD"'"
  }'
```

Windows PowerShell:

```powershell
curl -X POST http://localhost:8080/sessions/ `
  -H "Content-Type: application/json" `
  -d '{
    "name": "executor-smoke-test",
    "workspace": "C:\\path\\to\\project"
  }'
```

7. Test command execution.

Windows:

```powershell
curl -X POST http://localhost:8080/execute `
  -H "Content-Type: application/json" `
  -d '{
    "session_id": "YOUR_SESSION_ID",
    "command": "Get-Location"
  }'
```

Linux/macOS:

```bash
curl -s -X POST http://localhost:8080/execute \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "YOUR_SESSION_ID",
    "command": "pwd"
  }'
```

## Acceptance Criteria
- `workerd` starts successfully.
- `/health` returns OK.
- `/execute` runs an OS-native command successfully.
- Final notes include exact commands run and results.
