# ptolemy-voice (MVP)

Windows-only voice catcher. You speak; it converts speech to text (speech-to-text),
then either performs a local action or sends a shell command to the Ptolemy executor.

> Note: this is **speech-to-text**, not text-to-speech. You do **not** need to train or
> clone your own voice — recognizing your speech uses a generic speech model. Voice
> cloning would only be needed to make the computer *speak in your voice*, which this
> MVP does not do.

Wake phrase: **`hey ptolemy`**

## Commands

After the wake phrase, within a 30-second window you can say:

- `sleep pc` — put the PC to sleep (local action)
- `set alarm 7:30 am` / `set alarm at 14:00` — schedule a local alarm
- `set reminder buy milk in 10 minutes` / `set reminder standup at 9:00 am` — local reminder
- `run <command>` (or `run command <command>`) — send a shell command to the executor

### Running shell commands (with confirmation)

Shell commands are **never executed on the first utterance**, so a misheard command can't
run by accident:

1. Say `hey ptolemy`, then `run go test ./...`
2. It prints `Would run: go test ./...` and waits up to 30 seconds.
3. Say `confirm` (or `yes do it` / `execute`) to run it; say anything else to cancel.
4. It opens a worker session, runs the command via the executor, and prints
   `exit <code>: <summary>`.

The session is opened lazily on the first confirmed command and reused afterward.

## Executor connection

The voice catcher calls the local `workerd` HTTP server's `/execute` endpoint.

- Start `workerd` first: `make run` (or `go run ./cmd/workerd`).
- Override the address with `WORKER_BASE_URL` (default `http://127.0.0.1:8080`).

## Build and Run

Two listeners are available:

- **Native (recommended first run):** uses the built-in Windows `System.Speech`
  recognizer — no extra dependencies, no model download.

  ```powershell
  make voice-native
  # or:
  go run ./cmd/ptolemy-voice
  ```

- **Vosk (offline neural recognizer, recommended for accuracy):** built with the `vosk`
  tag. The native Windows `System.Speech` engine uses free-form dictation and will not
  reliably recognize the wake word "ptolemy" (it tends to transcribe random English
  phrases). Vosk fixes this and also handles arbitrary `run <command>` dictation.

  Vosk uses **cgo**, so it links a C library at build *and* run time. One-time setup on
  Windows:

  1. **C compiler** (for cgo): install MSYS2 + MinGW gcc.
     ```powershell
     winget install -e --id MSYS2.MSYS2
     # then in "MSYS2 MINGW64": pacman -S mingw-w64-x86_64-gcc
     # add C:\msys64\mingw64\bin to PATH
     ```
  2. **Vosk C library** (`libvosk.dll` + `libvosk.lib` + `vosk_api.h`): download
     `vosk-win64-*.zip` from https://github.com/alphacep/vosk-api/releases and unzip it to
     **`vosk-lib\` at the repo root** (i.e. `D:\Ptolemy\vosk-lib\`). The build finds it
     there automatically via `third_party/vosk-go` — see that folder's `vendor-notes.md`.
  3. **Language model**: download `vosk-model-small-en-us-0.15` (~40MB) from
     https://alphacephei.com/vosk/models and unzip to `.state\vosk-model` (the folder must
     directly contain `am/`, `conf/`, ...), or set `VOSK_MODEL_PATH` to its location.

  Build (PowerShell, at the repo root):
  ```powershell
  $env:CGO_ENABLED = "1"
  go build -tags vosk -o bin\ptolemy-voice-vosk.exe .\cmd\ptolemy-voice
  ```

  > **Do NOT set `CGO_CPPFLAGS` / `CGO_LDFLAGS`.** The Vosk header/lib paths are supplied by
  > `third_party/vosk-go` (a local copy of the wrapper with package-scoped `#cgo` directives).
  > Setting those env vars leaks the include path into Go's own `runtime/cgo`, which builds
  > with `-Werror`, and the build dies with `runtime/cgo: ... exit status 2` before it ever
  > reaches Vosk. If you previously set them, clear them first:
  > `Remove-Item Env:CGO_CFLAGS,Env:CGO_CPPFLAGS,Env:CGO_LDFLAGS -ErrorAction SilentlyContinue`

  Run (`libvosk.dll` must be on PATH at runtime):
  ```powershell
  $env:Path += ";D:\Ptolemy\vosk-lib"
  $env:WORKER_BASE_URL = "http://127.0.0.1:8080"
  .\bin\ptolemy-voice-vosk.exe
  ```

## Flags

- `-realtime-json` — emit runtime events as JSON lines (heard, wake_detected,
  command_recognized, command_pending, command_confirmed, command_executed,
  command_cancelled, command_window_timeout).
- `-listen-only` — only stream recognized phrases; no wake/command handling.
- `-no-actions` — parse and acknowledge commands without executing. For `run <command>`
  it stops at the dry-run print and never calls the executor.
