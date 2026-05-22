# ptolemy-voice (MVP)

Windows-only MVP voice catcher for:
- `sleep pc`
- `set alarm ...`
- `set reminder ...`

Wake phrase:
- `hey ptolemy`

## Build and Run

This MVP uses a Vosk microphone listener behind the `vosk` build tag.

1. Install dependencies:
   - Vosk model directory and set `VOSK_MODEL_PATH` (or use `.state/vosk-model`)
   - PortAudio runtime
2. Add Go packages:
   - `go get github.com/alphacep/vosk-api/go github.com/gordonklaus/portaudio`
3. Run:

```powershell
go run -tags vosk ./cmd/ptolemy-voice
```

Without `-tags vosk`, the command exits with a clear dependency error.
