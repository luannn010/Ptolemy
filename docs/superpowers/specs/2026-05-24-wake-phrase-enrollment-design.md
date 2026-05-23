# Personalized Wake-Phrase Enrollment — Design

**Date:** 2026-05-24
**Feature branch:** `ptolemy/wake-enrollment`
**Status:** Approved (pending written-spec review)

## Problem

The Vosk voice catcher detects its wake phrase by exact text match:
`wakePhraseRe = \bhey ptolemy\b` applied to Vosk's transcript. Because Vosk
transcribes the *same spoken phrase* differently depending on the speaker's
pronunciation/accent (e.g. `"hey tolemy"`, `"hey ptolomy"`, `"hey toll me"`),
a user whose "hey ptolemy" does not transcribe to the literal string never
wakes the assistant — exactly the failure observed in testing.

We want a "Hey Siri"-style enrollment: the user says the wake phrase a few
times during a short setup, and the system adapts to how *their* speech is
recognized.

## Approach

**Text-variant enrollment with fuzzy matching** (chosen over true acoustic
fingerprinting, which is a large DSP/ML effort and duplicates what Vosk already
does). Enrollment captures **what Vosk transcribes** when the user says the
wake phrase, and stores those transcripts as personal wake variants. At runtime
the wake check fuzzy-matches the live transcript against the stored variants.

This fits the existing text pipeline (Vosk → text → match) with no audio
fingerprinting, and is buildable as a small, well-tested unit.

## Non-Goals (YAGNI)

- No acoustic/MFCC matching or per-user model training.
- No multiple named voice profiles or multi-user support.
- No GUI; setup is terminal-driven like the rest of the voice catcher.
- No live re-training during normal use.

## Components

All new pure logic lives in `internal/voice/` with **no build tags**, so it is
unit-testable on any platform (the existing Vosk listener is the only
Windows+cgo piece).

### 1. `internal/voice/wake_profile.go`
- `type WakeProfile struct { Variants []string }`
- `DefaultWakeProfile() *WakeProfile` → `{Variants: ["hey ptolemy"]}`. The
  default is always present so the catcher works even with no/!corrupt profile.
- `WakeProfilePath() string` → `.state/wake-profile.json`, overridable via the
  `WAKE_PROFILE_PATH` environment variable (mirrors `VOSK_MODEL_PATH`).
- `LoadWakeProfile(path string) *WakeProfile` → reads + unmarshals JSON. On
  missing file or unmarshal error, returns `DefaultWakeProfile()` and logs a
  warning to stderr. Never returns a fatal error (wake must always work).
- `(p *WakeProfile) Save(path string) error` → creates the parent dir if
  needed, writes pretty JSON.
- `(p *WakeProfile) Matches(transcript string) bool` → the runtime wake check
  (see Matching).

### 2. `internal/voice/fuzzy.go`
- `levenshtein(a, b string) int` → standard edit distance, pure function.

### 3. `BuildProfileFromSamples` (in `wake_profile.go`)
- `BuildProfileFromSamples(samples []string) (*WakeProfile, []string)` returns
  the profile and a list of human-readable warnings.
- Logic: normalize each sample (lowercase, collapse whitespace, trim); drop
  empties; dedupe; **always include the default `"hey ptolemy"`**.
- If fewer than `minUsableSamples` (=2) non-empty samples were captured, return
  a default-only profile plus a warning (do not fail).

### 4. Enrollment glue — `cmd/ptolemy-voice/main.go` (Windows-tagged)
Thin orchestration only; all decision logic is in the testable functions above.

```
func runEnrollment(ctx, phrases <-chan string) []string
  for n in 1..enrollmentSamples (=4):
    print: Say "hey ptolemy"  (n/4)
    wait for next non-empty transcript from phrases (per-sample 10s timeout)
      on timeout: print retry hint, re-prompt (cap 3 attempts/sample)
    echo: "  heard: <transcript>"; append to samples
  return samples
```

### 5. Wiring in `main()`
- Add flag: `enroll := flag.Bool("enroll", false, "(re)run wake-phrase enrollment, then exit")`.
- Determine `profilePath := voice.WakeProfilePath()`.
- After the listener starts:
  - `needEnroll := *enroll || !fileExists(profilePath)`
  - if `needEnroll`: `samples := runEnrollment(...)`;
    `profile, warns := voice.BuildProfileFromSamples(samples)`;
    `profile.Save(profilePath)`; print variants + warnings;
    if `*enroll` → exit 0; else fall through to the listen loop.
  - `profile := voice.LoadWakeProfile(profilePath)`
- In the main loop, replace `voice.IsWakePhrase(normalized)` with
  `profile.Matches(normalized)`. (`IsWakePhrase` is retained for the default
  exact behavior and existing tests.)

## Matching (`WakeProfile.Matches`)

```
norm = normalize(transcript)           // lowercase, collapse spaces, trim
for each variant v in p.Variants:
    nv = normalize(v)
    if nv == "" { continue }
    if strings.Contains(norm, nv) { return true }   // current substring behavior
    if levenshtein(norm, nv) <= threshold(nv) { return true }
return false

threshold(v) = max(2, len(v)/4)        // "hey ptolemy" (11) -> 2; tolerant but distinctive
```

Rationale: substring keeps today's behavior (wake phrase embedded in a longer
utterance). Edit-distance adds tolerance for Vosk's run-to-run variation. The
phrase is long/distinctive enough that `len/4` keeps false wakes low.

## Data Flow

**Enrollment:** mic → Vosk → transcript channel → `runEnrollment` collects N
samples → `BuildProfileFromSamples` → `WakeProfile` → `Save` → JSON on disk.

**Runtime:** mic → Vosk → transcript → `profile.Matches` → wake/no-wake (rest
of the existing state machine unchanged).

## Persistence Format

`.state/wake-profile.json`:
```json
{
  "variants": ["hey ptolemy", "hey tolemy", "hey ptolomy"]
}
```

## Error Handling

| Situation | Behavior |
|-----------|----------|
| Silence during a sample | Re-prompt, up to 3 attempts per sample |
| < 2 usable samples total | Save default-only profile, print warning |
| Profile file missing | Use `DefaultWakeProfile()` (or trigger auto-enroll) |
| Profile file corrupt | Warn to stderr, use `DefaultWakeProfile()` |
| `.state/` missing on save | Create it |

Enrollment never aborts the program; worst case the user keeps the default
`"hey ptolemy"`.

## Testing (TDD)

Pure/IO units, all runnable via `go test ./internal/voice/` on Linux:

- `levenshtein`: known pairs (insert/delete/substitute, empty, identical).
- `Matches`: exact hit, fuzzy hit (`"hey toll me"`), fuzzy miss
  (`"what time is it"`), substring (`"ok hey ptolemy now"`), empty transcript,
  empty/whitespace variant skipped.
- `BuildProfileFromSamples`: dedupe, drop-empty, always-includes-default,
  `<2` usable → default-only + warning.
- `Load`/`Save`: round-trip via a temp file; load of missing file → default;
  load of corrupt JSON → default.

The Windows-tagged `runEnrollment` glue is not unit-tested (consistent with the
existing untested listener glue); it is verified manually on Windows via WSL
interop in the same way the build was verified.

## Implementation Phases

1. `fuzzy.go` + `levenshtein` tests.
2. `wake_profile.go`: type, Default, Load/Save, Matches + tests.
3. `BuildProfileFromSamples` + tests.
4. `cmd/ptolemy-voice` wiring: `-enroll` flag, `runEnrollment`, auto-enroll,
   swap to `profile.Matches`.
5. Docs (README enrollment section) + `run-vosk.bat` note; Windows build/run
   verification.

Each phase is a focused commit on `ptolemy/wake-enrollment`.
