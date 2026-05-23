# Voice → Brain Chat (single-turn) — Design

**Date:** 2026-05-24
**Branch:** `ptolemy/wake-enrollment` (built on current branch)
**Status:** Approved (pending written-spec review)

## Problem

The voice catcher recognizes a fixed set of commands (sleep/alarm/reminder/run).
Anything else prints "I didn't catch that." We want spoken phrases that aren't
commands to be answered by the local LLM ("brain"), turning the catcher into a
voice chat. Because the input is raw speech-to-text and often imperfect, the
prompt sent to the brain must first be refined for correctness.

## Brain liveness (verified 2026-05-24)

A `brain.Client` already exists (`internal/brain/client.go`) speaking the
OpenAI-compatible `POST /v1/chat/completions` API, configured by `BRAIN_BASE_URL`
(default `http://127.0.0.1:8088`) and `BRAIN_MODEL` (default `gemma-4-e2b`).

The brain is **not currently serving**: ports 8088 and 1089 are held by Windows
`svchost.exe`, and all HTTP requests reset (classic Windows reserved-port-range
behavior). The user's model server is intended to run on **:1089**. This design
therefore (a) makes the base URL configurable to point at :1089 and (b) degrades
gracefully while the brain is down.

## Decisions

- **Trigger:** fallback to chat. Existing commands match first; an unrecognized
  phrase goes to the brain instead of "I didn't catch that."
- **Conversation:** stay awake after each answer (reset the 30s window) for
  follow-ups. No memory — each turn is standalone (no history, no session save).
- **Output:** print text only (`Ptolemy: <answer>`). No TTS.
- **Refinement layer:** system-prompt only (one call). The voice system prompt
  instructs the model that the user message is raw speech-to-text that may
  contain recognition errors, and to silently correct it to the most likely
  intended question before answering concisely. No separate refinement call.
- **Config:** read `BRAIN_BASE_URL` / `BRAIN_MODEL` from the environment;
  `run-vosk.bat` sets `BRAIN_BASE_URL=http://127.0.0.1:1089`.

## Non-Goals (YAGNI)

- No conversation memory or multi-turn context.
- No session persistence.
- No text-to-speech.
- No streaming responses.
- No separate LLM refinement pass (refinement is via the system prompt).

## Components

### 1. `internal/brain/voice_prompt.go` (new, no build tags → testable on Linux)
- `const VoiceSystemPrompt` — the refinement + conciseness instruction, e.g.:

  > "You are Ptolemy, a concise voice assistant. The user's message is raw
  > speech-to-text and may contain recognition errors, missing punctuation, or
  > misheard words. Silently infer and correct the most likely intended question,
  > then answer it in 1–3 short sentences. Do not mention the correction."

- `func ChatMessagesForVoice(userText string) []Message` → returns
  `[{role:"system", content:VoiceSystemPrompt}, {role:"user", content:userText}]`.

This lives in `internal/brain` (next to the client) so the voice package depends
only on `brain`, and the helper is unit-testable without the Windows/cgo build.

### 2. `cmd/ptolemy-voice/main.go` (Windows-tagged glue)
- At startup, construct a brain client using small env helpers that mirror the
  existing `workerBaseURL()` in this file:
  `brainBaseURL()` → `BRAIN_BASE_URL` or `config.DefaultBrainBaseURL`;
  `brainModel()` → `BRAIN_MODEL` or `config.DefaultBrainModel`; then
  `brainClient := brain.NewClient(brainBaseURL(), brainModel())`.
- In the command-window branch where `ParseCommand` currently fails (prints
  "I didn't catch that."), instead call the brain:
  ```
  answer, err := brainClient.Chat(ctx, brain.ChatMessagesForVoice(phrase))
  if err != nil {
      fmt.Printf("Ptolemy (brain unavailable): %v\n", err)
  } else {
      fmt.Printf("Ptolemy: %s\n", strings.TrimSpace(answer))
  }
  activeUntil = time.Now().Add(commandWindow) // stay awake for follow-ups
  continue
  ```
- The existing `-no-actions` flag suppresses side-effecting commands; chat is a
  read-only query, so it still runs under `-no-actions` (a chat has no system
  side effects). (Documented; no special-casing.)

### 3. `run-vosk.bat` + README
- `run-vosk.bat` sets `BRAIN_BASE_URL=http://127.0.0.1:1089` (and leaves
  `BRAIN_MODEL` to the env/default) so the catcher targets the user's server.
- README documents the chat behavior, the system-prompt refinement, the port,
  and the graceful-degradation message.

## Data Flow

```
mic → Vosk → transcript → (awake) ParseCommand
  match    → run command (unchanged)
  no match → brainClient.Chat(ctx, ChatMessagesForVoice(transcript))
               → print "Ptolemy: <answer>"  (or brain-unavailable notice)
               → reset 30s window (stay awake)
```

The brain itself performs the refinement (interpreting/correcting the noisy
transcript) under the guidance of `VoiceSystemPrompt`.

## Error Handling

| Situation | Behavior |
|-----------|----------|
| Brain unreachable / reset / timeout | Print `Ptolemy (brain unavailable): <err>`, stay awake, never crash |
| Empty/whitespace answer | Print the trimmed (possibly empty) answer; still stay awake |
| `BRAIN_BASE_URL`/`BRAIN_MODEL` unset | Use `config.DefaultBrainBaseURL` / `config.DefaultBrainModel` |

`brain.Client.Chat` already maps timeouts to a helpful message; we surface it.

## Testing

- `ChatMessagesForVoice` (Linux unit tests): returns exactly two messages;
  first is `system` with `VoiceSystemPrompt`; second is `user` with the input
  text unchanged; system prompt mentions speech-to-text/correction and
  conciseness.
- Existing `brain.Client` HTTP tests already cover the request/response path.
- Windows glue verified via the interop build; live behavior smoke-tested once
  the brain server is actually serving on :1089.

## Implementation Phases

1. `internal/brain/voice_prompt.go` + tests (`VoiceSystemPrompt`,
   `ChatMessagesForVoice`).
2. `cmd/ptolemy-voice/main.go` wiring (brain client from config; chat fallback;
   stay-awake; graceful error).
3. `run-vosk.bat` sets `BRAIN_BASE_URL`; README chat section; Windows build +
   `go test ./...` verification.

Each phase is a focused commit on `ptolemy/wake-enrollment`.

## Manual acceptance (user, on Windows, brain serving on :1089)

1. `run-vosk.bat` → "hey ptolemy" → ask a question that isn't a command →
   `Ptolemy: <answer>` appears and it stays awake.
2. Ask a follow-up without re-waking → answered.
3. With the brain down → `Ptolemy (brain unavailable): ...` and it keeps running.
