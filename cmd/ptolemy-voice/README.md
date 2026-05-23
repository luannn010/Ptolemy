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

- **Vosk (offline, higher quality):** built with the `vosk` tag. Requires the PortAudio
  runtime and a Vosk model directory (`VOSK_MODEL_PATH`, default `.state/vosk-model`).

  ```powershell
  go get github.com/alphacep/vosk-api/go github.com/gordonklaus/portaudio
  make voice
  # or:
  go run -tags vosk ./cmd/ptolemy-voice
  ```

## Flags

- `-realtime-json` — emit runtime events as JSON lines (heard, wake_detected,
  command_recognized, command_pending, command_confirmed, command_executed,
  command_cancelled, command_window_timeout).
- `-listen-only` — only stream recognized phrases; no wake/command handling.
- `-no-actions` — parse and acknowledge commands without executing. For `run <command>`
  it stops at the dry-run print and never calls the executor.
