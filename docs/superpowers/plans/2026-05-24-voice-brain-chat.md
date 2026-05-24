# Voice → Brain Chat Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When a woken voice phrase isn't a built-in command, send it to the local brain LLM (with a system prompt that corrects noisy speech-to-text) and print the answer, staying awake for follow-ups.

**Architecture:** Reuse the existing `internal/brain.Client` (OpenAI-compatible `/v1/chat/completions`). Add a pure, testable helper `ChatMessagesForVoice` in `internal/brain` that pairs a speech-correcting system prompt with the user transcript. Wire it into `cmd/ptolemy-voice/main.go` at the "unrecognized command" branch; on error, degrade gracefully. Single-turn, no memory.

**Tech Stack:** Go 1.25, stdlib + existing `internal/brain` and `internal/config`. Brain served by Ollama (`qwen3.5:4b`) on `:1089`. Windows build via WSL interop (`build-vosk.bat`).

**Spec:** `docs/superpowers/specs/2026-05-24-voice-brain-chat-design.md`

---

## File Structure

- Create: `internal/brain/voice_prompt.go` — `VoiceSystemPrompt` const + `ChatMessagesForVoice`.
- Create: `internal/brain/voice_prompt_test.go` — tests for the above.
- Modify: `cmd/ptolemy-voice/main.go` — `brainBaseURL`/`brainModel` helpers, brain client, chat fallback + stay-awake + graceful error, `internal/brain` import.
- Modify: `run-vosk.bat` — set `BRAIN_BASE_URL` and `BRAIN_MODEL`.
- Modify: `cmd/ptolemy-voice/README.md` — chat section.

Reused unchanged: `internal/brain/client.go` (`Client`, `Message`, `NewClient`, `Chat`), `internal/config` (`DefaultBrainBaseURL`, `DefaultBrainModel`).

---

## Task 1: Voice chat prompt helper

**Files:**
- Create: `internal/brain/voice_prompt.go`
- Test: `internal/brain/voice_prompt_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/brain/voice_prompt_test.go`:

```go
package brain

import (
	"strings"
	"testing"
)

func TestChatMessagesForVoice(t *testing.T) {
	msgs := ChatMessagesForVoice("whats the capital of france")
	if len(msgs) != 2 {
		t.Fatalf("want 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Errorf("first message role = %q, want system", msgs[0].Role)
	}
	if msgs[0].Content != VoiceSystemPrompt {
		t.Errorf("first message content is not VoiceSystemPrompt")
	}
	if msgs[1].Role != "user" {
		t.Errorf("second message role = %q, want user", msgs[1].Role)
	}
	if msgs[1].Content != "whats the capital of france" {
		t.Errorf("user content altered: %q", msgs[1].Content)
	}
}

func TestVoiceSystemPromptMentionsCorrectionAndConciseness(t *testing.T) {
	p := strings.ToLower(VoiceSystemPrompt)
	for _, kw := range []string{"speech-to-text", "correct", "concise"} {
		if !strings.Contains(p, kw) {
			t.Errorf("VoiceSystemPrompt should mention %q", kw)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/brain/ -run 'ChatMessagesForVoice|VoiceSystemPrompt'`
Expected: FAIL — `undefined: ChatMessagesForVoice` / `undefined: VoiceSystemPrompt` (build failed).

- [ ] **Step 3: Write minimal implementation**

Create `internal/brain/voice_prompt.go`:

```go
package brain

// VoiceSystemPrompt instructs the model that the user's message is raw
// speech-to-text (which may contain recognition errors, missing punctuation, or
// misheard words), to silently correct it to the most likely intended question,
// and to answer concisely for spoken interaction. This is the "refinement layer"
// for the voice chat: correction happens inside this single chat call.
const VoiceSystemPrompt = "You are Ptolemy, a concise voice assistant. " +
	"The user's message is raw speech-to-text and may contain recognition errors, " +
	"missing punctuation, or misheard words. Silently infer and correct the most " +
	"likely intended question, then answer it in 1-3 short sentences. " +
	"Do not mention the correction."

// ChatMessagesForVoice builds the message list for a single-turn voice chat: the
// voice system prompt followed by the user's raw transcript.
func ChatMessagesForVoice(userText string) []Message {
	return []Message{
		{Role: "system", Content: VoiceSystemPrompt},
		{Role: "user", Content: userText},
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/brain/ -run 'ChatMessagesForVoice|VoiceSystemPrompt' -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/brain/voice_prompt.go internal/brain/voice_prompt_test.go
git commit -m "feat(brain): add voice chat system prompt and message builder"
```

---

## Task 2: Wire chat fallback into the voice catcher

**Files:**
- Modify: `cmd/ptolemy-voice/main.go`

This file is `//go:build windows`; it cannot compile on Linux. Verify with the Windows build in Task 3.

- [ ] **Step 1: Add the `internal/brain` import**

In `cmd/ptolemy-voice/main.go` the import block ends with:

```go
	"github.com/luannn010/ptolemy/internal/config"
	"github.com/luannn010/ptolemy/internal/voice"
)
```

Add the brain import (keep alphabetical order):

```go
	"github.com/luannn010/ptolemy/internal/brain"
	"github.com/luannn010/ptolemy/internal/config"
	"github.com/luannn010/ptolemy/internal/voice"
)
```

- [ ] **Step 2: Add `brainBaseURL` and `brainModel` helpers**

Immediately after the existing `workerBaseURL` function:

```go
func workerBaseURL() string {
	if v := strings.TrimSpace(os.Getenv("WORKER_BASE_URL")); v != "" {
		return v
	}
	return config.DefaultWorkerBaseURL
}
```

add:

```go
func brainBaseURL() string {
	if v := strings.TrimSpace(os.Getenv("BRAIN_BASE_URL")); v != "" {
		return v
	}
	return config.DefaultBrainBaseURL
}

func brainModel() string {
	if v := strings.TrimSpace(os.Getenv("BRAIN_MODEL")); v != "" {
		return v
	}
	return config.DefaultBrainModel
}
```

- [ ] **Step 3: Construct the brain client**

The execClient is created here:

```go
	execClient := voice.NewHTTPExecutorClient(workerBaseURL())
	fmt.Printf("Voice catcher started. Executor: %s\n", workerBaseURL())
```

Add the brain client and a startup line right after the executor line:

```go
	execClient := voice.NewHTTPExecutorClient(workerBaseURL())
	brainClient := brain.NewClient(brainBaseURL(), brainModel())
	fmt.Printf("Voice catcher started. Executor: %s\n", workerBaseURL())
	fmt.Printf("Brain: %s (model %s)\n", brainBaseURL(), brainModel())
```

- [ ] **Step 4: Replace the "I didn't catch that" branch with a chat call**

The current branch reads:

```go
			cmd, ok := voice.ParseCommand(normalized, time.Now())
			if !ok {
				fmt.Println("I didn't catch that.")
				emitEvent(*realtimeJSON, runtimeEvent{
					Time:    time.Now().Format(time.RFC3339Nano),
					Type:    "command_unrecognized",
					Heard:   phrase,
					Active:  true,
					Message: "not parsed as supported command",
				})
				continue
			}
```

Replace it with:

```go
			cmd, ok := voice.ParseCommand(normalized, time.Now())
			if !ok {
				// Not a built-in command: ask the brain. The voice system prompt
				// tells the model the input is raw speech-to-text and to correct
				// it before answering, so the prompt is refined inside this call.
				answer, chatErr := brainClient.Chat(ctx, brain.ChatMessagesForVoice(phrase))
				eventMsg := strings.TrimSpace(answer)
				if chatErr != nil {
					fmt.Printf("Ptolemy (brain unavailable): %v\n", chatErr)
					eventMsg = "brain unavailable: " + chatErr.Error()
				} else {
					fmt.Printf("Ptolemy: %s\n", strings.TrimSpace(answer))
				}
				emitEvent(*realtimeJSON, runtimeEvent{
					Time:    time.Now().Format(time.RFC3339Nano),
					Type:    "chat_response",
					Heard:   phrase,
					Active:  true,
					Message: eventMsg,
				})
				// Stay awake so follow-up questions don't need the wake phrase.
				activeUntil = time.Now().Add(commandWindow)
				continue
			}
```

- [ ] **Step 5: Type-check the Windows build (frontend only)**

Run: `GOOS=windows CGO_ENABLED=1 go build -n -tags vosk ./cmd/ptolemy-voice 2>&1 | grep -iE 'error|\.go:[0-9]|undefined|cannot use' | head`
Expected: no output (no type errors). The `-mthreads` gcc error, if any, appears only at the C stage and is unrelated to Go type-checking. Confirm the package reaches codegen:
Run: `GOOS=windows CGO_ENABLED=1 go build -n -tags vosk ./cmd/ptolemy-voice 2>&1 | grep -c "ptolemy-voice"`
Expected: a number > 0.

- [ ] **Step 6: gofmt**

Run: `gofmt -l cmd/ptolemy-voice/main.go`
Expected: no output (file already formatted).

- [ ] **Step 7: Commit**

```bash
git add cmd/ptolemy-voice/main.go
git commit -m "feat(voice): chat fallback to brain on unrecognized phrases"
```

---

## Task 3: Config wiring, docs, and verification

**Files:**
- Modify: `run-vosk.bat`
- Modify: `cmd/ptolemy-voice/README.md`

- [ ] **Step 1: Point run-vosk.bat at the brain**

The current `run-vosk.bat` is:

```bat
@echo off
REM Run the Vosk voice catcher. Puts libvosk.dll (+ support DLLs) and
REM libportaudio.dll on PATH, and points VOSK_MODEL_PATH at the local model.
REM Pass extra args through, e.g.  run-vosk.bat -listen-only
cd /d D:\Ptolemy
set PATH=D:\Ptolemy\vosk-lib;C:\msys64\mingw64\bin;%PATH%
set VOSK_MODEL_PATH=D:\Ptolemy\.state\vosk-model
if "%WORKER_BASE_URL%"=="" set WORKER_BASE_URL=http://127.0.0.1:8080
bin\ptolemy-voice-vosk.exe %*
```

Add the two brain env lines before the `bin\...` line:

```bat
@echo off
REM Run the Vosk voice catcher. Puts libvosk.dll (+ support DLLs) and
REM libportaudio.dll on PATH, and points VOSK_MODEL_PATH at the local model.
REM Pass extra args through, e.g.  run-vosk.bat -listen-only
cd /d D:\Ptolemy
set PATH=D:\Ptolemy\vosk-lib;C:\msys64\mingw64\bin;%PATH%
set VOSK_MODEL_PATH=D:\Ptolemy\.state\vosk-model
if "%WORKER_BASE_URL%"=="" set WORKER_BASE_URL=http://127.0.0.1:8080
if "%BRAIN_BASE_URL%"=="" set BRAIN_BASE_URL=http://127.0.0.1:1089
if "%BRAIN_MODEL%"=="" set BRAIN_MODEL=qwen3.5:4b
bin\ptolemy-voice-vosk.exe %*
```

- [ ] **Step 2: Document chat in the README**

In `cmd/ptolemy-voice/README.md`, after the `## Wake-phrase enrollment` section (it ends with the line about `WAKE_PROFILE_PATH`) and before `## Flags`, add:

````markdown
## Brain chat (ask anything)

After the wake phrase, anything that isn't a built-in command is sent to the
local brain LLM and answered out loud in the terminal:

```
hey ptolemy
what's the capital of France
Ptolemy: Paris.
```

It stays awake after each answer, so you can keep asking without repeating the
wake phrase (each question is independent — there is no memory yet). Because the
input is raw speech-to-text, the chat's system prompt instructs the model to
silently correct recognition errors before answering.

Configure the brain with `BRAIN_BASE_URL` (default `http://127.0.0.1:8088`) and
`BRAIN_MODEL` (default `gemma-4-e2b`). `run-vosk.bat` sets these to
`http://127.0.0.1:1089` and `qwen3.5:4b` (Ollama). If the brain is unreachable
you'll see `Ptolemy (brain unavailable): ...` and the catcher keeps running.
````

- [ ] **Step 3: Run the full Go test suite**

Run: `go test -p 1 ./...`
Expected: all packages `ok` / `no test files` (exit 0). In particular `internal/brain` passes.

- [ ] **Step 4: Build on Windows via interop**

Run: `cmd.exe /c "D:\Ptolemy\build-vosk.bat"`
Expected: `BUILD_EXIT=0` and `bin\ptolemy-voice-vosk.exe` present.

- [ ] **Step 5: Smoke-test startup (no mic interaction)**

Run: `timeout 15 cmd.exe /c "D:\Ptolemy\run-vosk.bat -listen-only > D:\Ptolemy\vosk-run.log 2>&1"` then reap any orphan: `cmd.exe /c "taskkill /F /IM ptolemy-voice-vosk.exe /T"` and read `vosk-run.log`.
Expected (in log): `Voice catcher started. Executor: http://127.0.0.1:8080` and `Brain: http://127.0.0.1:1089 (model qwen3.5:4b)`.

- [ ] **Step 6: Clean up scratch + commit**

```bash
rm -f vosk-run.log
git add run-vosk.bat cmd/ptolemy-voice/README.md
git commit -m "docs(voice): wire brain chat config and document chat"
```

---

## Manual acceptance (user, on Windows, Ollama serving qwen3.5:4b on :1089)

These need a mic and a live brain:

1. `run-vosk.bat` → "hey ptolemy" → ask a non-command question → `Ptolemy: <answer>` appears; catcher stays awake.
2. Ask a follow-up without re-waking → answered.
3. Stop Ollama → ask again → `Ptolemy (brain unavailable): ...`; catcher keeps running.
