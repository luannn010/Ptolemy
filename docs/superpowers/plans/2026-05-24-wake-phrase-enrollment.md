# Wake-Phrase Enrollment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a user enroll their own pronunciation of the wake phrase (Hey-Siri-style setup) so the Vosk voice catcher wakes reliably despite accent/transcription variation.

**Architecture:** Enrollment captures what Vosk *transcribes* when the user says the wake phrase a few times and stores those strings as personal wake variants in `.state/wake-profile.json`. At runtime the wake check fuzzy-matches the live transcript (substring OR small Levenshtein distance) against the stored variants. All matching/persistence logic is pure Go in `internal/voice` (unit-tested on any OS); only the interactive prompt loop is Windows-tagged glue in `cmd/ptolemy-voice`.

**Tech Stack:** Go 1.25, standard library only (`encoding/json`, `os`). Reuses the existing `normalize` helper in `internal/voice/command.go`. Windows build via WSL→interop (`build-vosk.bat`).

**Spec:** `docs/superpowers/specs/2026-05-24-wake-phrase-enrollment-design.md`

---

## File Structure

- Create: `internal/voice/fuzzy.go` — `levenshtein` edit-distance helper.
- Create: `internal/voice/fuzzy_test.go` — tests for `levenshtein`.
- Create: `internal/voice/wake_profile.go` — `WakeProfile` type, default, path, Load/Save, `Matches`, `BuildProfileFromSamples`.
- Create: `internal/voice/wake_profile_test.go` — tests for all of the above.
- Modify: `cmd/ptolemy-voice/main.go` — `-enroll` flag, `runEnrollment`, auto-enroll, swap wake check to `profile.Matches`.
- Modify: `cmd/ptolemy-voice/README.md` — enrollment docs.

Reused as-is: `normalize(string) string` in `internal/voice/command.go:102`.

---

## Task 1: Levenshtein helper

**Files:**
- Create: `internal/voice/fuzzy.go`
- Test: `internal/voice/fuzzy_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/voice/fuzzy_test.go`:

```go
package voice

import "testing"

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "abc", 0},
		{"", "abc", 3},
		{"abc", "", 3},
		{"kitten", "sitting", 3},
		{"flaw", "lawn", 2},
		{"hey ptolemy", "hey ptolomy", 1}, // one substitution
		{"hey ptolemy", "hey tolemy", 1},  // one deletion
		{"hey ptolemy", "hey there", 5},
	}
	for _, tc := range tests {
		if got := levenshtein(tc.a, tc.b); got != tc.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/voice/ -run TestLevenshtein`
Expected: FAIL — `undefined: levenshtein` (build failed).

- [ ] **Step 3: Write minimal implementation**

Create `internal/voice/fuzzy.go`:

```go
package voice

// levenshtein returns the edit distance (insertions, deletions, substitutions)
// between a and b, compared rune-by-rune. Used for fuzzy wake-phrase matching.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			curr[j] = min3(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/voice/ -run TestLevenshtein -v`
Expected: PASS (all subtests).

- [ ] **Step 5: Commit**

```bash
git add internal/voice/fuzzy.go internal/voice/fuzzy_test.go
git commit -m "feat(voice): add levenshtein helper for fuzzy wake matching"
```

---

## Task 2: WakeProfile type, default, path, Load/Save, Matches

**Files:**
- Create: `internal/voice/wake_profile.go`
- Test: `internal/voice/wake_profile_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/voice/wake_profile_test.go`:

```go
package voice

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultWakeProfileMatches(t *testing.T) {
	p := DefaultWakeProfile()
	if !p.Matches("hey ptolemy") {
		t.Fatal("default profile should match exact wake phrase")
	}
}

func TestWakeProfileMatches(t *testing.T) {
	// Second variant simulates a pronunciation captured during enrollment.
	p := &WakeProfile{Variants: []string{"hey ptolemy", "hey tolemy"}}
	tests := []struct {
		transcript string
		want       bool
	}{
		{"hey ptolemy", true},        // exact / substring
		{"hey ptolomy", true},        // dist 1 to "hey ptolemy"
		{"hey tolomy", true},         // dist 1 to enrolled "hey tolemy"
		{"ok hey ptolemy now", true}, // substring
		{"HEY PTOLEMY", true},        // case-insensitive
		{"hey there", false},         // dist 4-5, not close
		{"what time is it", false},   // unrelated
		{"", false},                  // empty
	}
	for _, tc := range tests {
		if got := p.Matches(tc.transcript); got != tc.want {
			t.Errorf("Matches(%q) = %v, want %v", tc.transcript, got, tc.want)
		}
	}
}

func TestWakeProfileSkipsEmptyVariant(t *testing.T) {
	p := &WakeProfile{Variants: []string{"", "   "}}
	if p.Matches("anything") {
		t.Fatal("empty/whitespace variants must not match everything")
	}
}

func TestWakeProfileSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "wake-profile.json")
	orig := &WakeProfile{Variants: []string{"hey ptolemy", "hey tolemy"}}
	if err := orig.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got := LoadWakeProfile(path)
	if len(got.Variants) != 2 || got.Variants[0] != "hey ptolemy" || got.Variants[1] != "hey tolemy" {
		t.Fatalf("round-trip mismatch: %+v", got.Variants)
	}
}

func TestLoadWakeProfileMissingReturnsDefault(t *testing.T) {
	got := LoadWakeProfile(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if !got.Matches("hey ptolemy") {
		t.Fatal("missing profile should load default")
	}
}

func TestLoadWakeProfileCorruptReturnsDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wake-profile.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := LoadWakeProfile(path)
	if !got.Matches("hey ptolemy") {
		t.Fatal("corrupt profile should fall back to default")
	}
}

func TestWakeProfilePathEnvOverride(t *testing.T) {
	t.Setenv("WAKE_PROFILE_PATH", "/tmp/custom-wake.json")
	if WakeProfilePath() != "/tmp/custom-wake.json" {
		t.Fatalf("env override not honored: %q", WakeProfilePath())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/voice/ -run 'WakeProfile|LoadWakeProfile'`
Expected: FAIL — `undefined: DefaultWakeProfile` etc. (build failed).

- [ ] **Step 3: Write minimal implementation**

Create `internal/voice/wake_profile.go`:

```go
package voice

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// defaultWakePhrase is always present in a profile so the catcher works even
// with no saved profile.
const defaultWakePhrase = "hey ptolemy"

// WakeProfile holds the set of transcript variants that should trigger the wake
// word. Variants are produced by enrollment (what Vosk transcribed when the user
// spoke the wake phrase) plus the built-in default.
type WakeProfile struct {
	Variants []string `json:"variants"`
}

// DefaultWakeProfile returns a profile containing only the built-in wake phrase.
func DefaultWakeProfile() *WakeProfile {
	return &WakeProfile{Variants: []string{defaultWakePhrase}}
}

// WakeProfilePath returns where the wake profile is stored. Override with the
// WAKE_PROFILE_PATH environment variable (mirrors VOSK_MODEL_PATH).
func WakeProfilePath() string {
	if v := strings.TrimSpace(os.Getenv("WAKE_PROFILE_PATH")); v != "" {
		return v
	}
	return filepath.Join(".state", "wake-profile.json")
}

// LoadWakeProfile reads a profile from path. On any error (missing file, bad
// JSON, no variants) it logs a warning to stderr and returns the default
// profile, so wake detection always works.
func LoadWakeProfile(path string) *WakeProfile {
	data, err := os.ReadFile(path)
	if err != nil {
		return DefaultWakeProfile()
	}
	var p WakeProfile
	if err := json.Unmarshal(data, &p); err != nil {
		fmt.Fprintf(os.Stderr, "voice: wake profile %q is unreadable (%v); using default\n", path, err)
		return DefaultWakeProfile()
	}
	if len(p.Variants) == 0 {
		return DefaultWakeProfile()
	}
	return &p
}

// Save writes the profile to path as pretty JSON, creating parent dirs.
func (p *WakeProfile) Save(path string) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create wake profile dir: %w", err)
		}
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal wake profile: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write wake profile: %w", err)
	}
	return nil
}

// Matches reports whether transcript should trigger the wake word. A transcript
// matches a variant when the (normalized) variant is a substring of it, or when
// their Levenshtein distance is within max(2, len(variant)/4).
func (p *WakeProfile) Matches(transcript string) bool {
	norm := normalize(transcript)
	if norm == "" {
		return false
	}
	for _, v := range p.Variants {
		nv := normalize(v)
		if nv == "" {
			continue
		}
		if strings.Contains(norm, nv) {
			return true
		}
		if levenshtein(norm, nv) <= wakeThreshold(nv) {
			return true
		}
	}
	return false
}

// wakeThreshold is the maximum edit distance allowed for a fuzzy wake match.
func wakeThreshold(variant string) int {
	if t := len(variant) / 4; t > 2 {
		return t
	}
	return 2
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/voice/ -run 'WakeProfile|LoadWakeProfile' -v`
Expected: PASS (all subtests).

- [ ] **Step 5: Commit**

```bash
git add internal/voice/wake_profile.go internal/voice/wake_profile_test.go
git commit -m "feat(voice): add WakeProfile with fuzzy match and JSON persistence"
```

---

## Task 3: BuildProfileFromSamples

**Files:**
- Modify: `internal/voice/wake_profile.go` (append function + const)
- Test: `internal/voice/wake_profile_test.go` (append tests)

- [ ] **Step 1: Write the failing test**

Append to `internal/voice/wake_profile_test.go`:

```go
func TestBuildProfileFromSamples(t *testing.T) {
	p, warns := BuildProfileFromSamples([]string{"Hey Ptolemy", "hey  tolemy ", "hey tolemy", ""})
	// Lowercased, whitespace-collapsed, deduped, default always present.
	want := map[string]bool{"hey ptolemy": false, "hey tolemy": false}
	for _, v := range p.Variants {
		if _, ok := want[v]; !ok {
			t.Errorf("unexpected variant %q", v)
		}
		want[v] = true
	}
	for v, seen := range want {
		if !seen {
			t.Errorf("missing expected variant %q (got %v)", v, p.Variants)
		}
	}
	if len(warns) != 0 {
		t.Errorf("did not expect warnings, got %v", warns)
	}
}

func TestBuildProfileFromSamplesTooFewUsable(t *testing.T) {
	p, warns := BuildProfileFromSamples([]string{"", "   "})
	if len(p.Variants) != 1 || p.Variants[0] != "hey ptolemy" {
		t.Fatalf("expected default-only profile, got %v", p.Variants)
	}
	if len(warns) == 0 {
		t.Fatal("expected a warning about too few usable samples")
	}
}

func TestBuildProfileAlwaysIncludesDefault(t *testing.T) {
	p, _ := BuildProfileFromSamples([]string{"hey tolemy", "hey tolomy"})
	found := false
	for _, v := range p.Variants {
		if v == "hey ptolemy" {
			found = true
		}
	}
	if !found {
		t.Fatalf("default wake phrase must always be present, got %v", p.Variants)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/voice/ -run TestBuildProfile`
Expected: FAIL — `undefined: BuildProfileFromSamples` (build failed).

- [ ] **Step 3: Write minimal implementation**

Append to `internal/voice/wake_profile.go`:

```go
// minUsableSamples is the fewest non-empty enrollment captures needed to build a
// personalized profile; below this we keep the default only.
const minUsableSamples = 2

// BuildProfileFromSamples turns raw enrollment transcripts into a WakeProfile.
// It normalizes each sample, drops empties, dedupes, and always includes the
// default wake phrase. If fewer than minUsableSamples usable samples were
// captured it returns a default-only profile plus a warning (never an error).
func BuildProfileFromSamples(samples []string) (*WakeProfile, []string) {
	var warnings []string
	seen := map[string]bool{}
	variants := []string{defaultWakePhrase}
	seen[defaultWakePhrase] = true

	usable := 0
	for _, s := range samples {
		n := normalize(s)
		if n == "" {
			continue
		}
		usable++
		if !seen[n] {
			seen[n] = true
			variants = append(variants, n)
		}
	}

	if usable < minUsableSamples {
		warnings = append(warnings, fmt.Sprintf(
			"only %d usable sample(s) captured (need %d); keeping default wake phrase only",
			usable, minUsableSamples))
		return DefaultWakeProfile(), warnings
	}
	return &WakeProfile{Variants: variants}, warnings
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/voice/ -run TestBuildProfile -v`
Expected: PASS.

- [ ] **Step 5: Run the whole package suite**

Run: `go test ./internal/voice/`
Expected: `ok  github.com/luannn010/ptolemy/internal/voice`

- [ ] **Step 6: Commit**

```bash
git add internal/voice/wake_profile.go internal/voice/wake_profile_test.go
git commit -m "feat(voice): build wake profile from enrollment samples"
```

---

## Task 4: Wire enrollment + matching into the voice catcher

**Files:**
- Modify: `cmd/ptolemy-voice/main.go`

This file is `//go:build windows`; it cannot compile on Linux. Verify it via the Windows build in Task 5. Make the edits carefully.

- [ ] **Step 1: Add the `-enroll` flag**

In `cmd/ptolemy-voice/main.go`, the flag block currently reads (around lines 58-61):

```go
	realtimeJSON := flag.Bool("realtime-json", false, "emit runtime events as JSON lines")
	listenOnly := flag.Bool("listen-only", false, "only stream recognized phrases; do not run wake/command state machine")
	noActions := flag.Bool("no-actions", false, "parse and acknowledge commands without executing system actions")
	flag.Parse()
```

Add the enroll flag:

```go
	realtimeJSON := flag.Bool("realtime-json", false, "emit runtime events as JSON lines")
	listenOnly := flag.Bool("listen-only", false, "only stream recognized phrases; do not run wake/command state machine")
	noActions := flag.Bool("no-actions", false, "parse and acknowledge commands without executing system actions")
	enroll := flag.Bool("enroll", false, "(re)run wake-phrase enrollment, then exit")
	flag.Parse()
```

- [ ] **Step 2: Run enrollment / load profile after the listener starts**

The code that starts the listener and prints the banner currently reads (around lines 75-89):

```go
	phrases, err := listener.Listen(ctx)
	if err != nil {
		fmt.Printf("failed to start listening: %v\n", err)
		os.Exit(1)
	}

	execClient := voice.NewHTTPExecutorClient(workerBaseURL())
	fmt.Printf("Voice catcher started. Executor: %s\n", workerBaseURL())
	fmt.Println("Say 'Hey Ptolemy' to activate.")
	fmt.Println("Every phrase the speech engine recognizes is printed below as `heard: \"...\"`.")
	fmt.Println("If you speak and see no `heard:` line, the microphone/speech engine is not picking up audio.")

	activeUntil := time.Time{}
	pendingShell := ""
	sessionID := ""
```

Replace it with (adds enrollment + profile load between `Listen` and the banner):

```go
	phrases, err := listener.Listen(ctx)
	if err != nil {
		fmt.Printf("failed to start listening: %v\n", err)
		os.Exit(1)
	}

	profilePath := voice.WakeProfilePath()
	if *enroll || !fileExists(profilePath) {
		samples := runEnrollment(ctx, phrases)
		profile, warnings := voice.BuildProfileFromSamples(samples)
		if err := profile.Save(profilePath); err != nil {
			fmt.Printf("could not save wake profile: %v\n", err)
		} else {
			fmt.Printf("Saved wake profile to %s\n", profilePath)
		}
		fmt.Printf("Wake variants: %s\n", strings.Join(profile.Variants, " | "))
		for _, w := range warnings {
			fmt.Printf("  note: %s\n", w)
		}
		if *enroll {
			fmt.Println("Enrollment complete.")
			return
		}
	}
	profile := voice.LoadWakeProfile(profilePath)

	execClient := voice.NewHTTPExecutorClient(workerBaseURL())
	fmt.Printf("Voice catcher started. Executor: %s\n", workerBaseURL())
	fmt.Println("Say 'Hey Ptolemy' to activate.")
	fmt.Println("Every phrase the speech engine recognizes is printed below as `heard: \"...\"`.")
	fmt.Println("If you speak and see no `heard:` line, the microphone/speech engine is not picking up audio.")

	activeUntil := time.Time{}
	pendingShell := ""
	sessionID := ""
```

- [ ] **Step 3: Use the profile for wake detection**

The wake check currently reads (around line 124):

```go
			if activeUntil.IsZero() {
				if voice.IsWakePhrase(normalized) {
```

Change the condition to use the profile:

```go
			if activeUntil.IsZero() {
				if profile.Matches(normalized) {
```

- [ ] **Step 4: Add the `runEnrollment` and `fileExists` helpers**

At the end of `cmd/ptolemy-voice/main.go`, add:

```go
// enrollmentSamples is how many times the user is asked to say the wake phrase.
const enrollmentSamples = 4

// perSampleTimeout is how long to wait for one spoken sample before re-prompting.
const perSampleTimeout = 10 * time.Second

// maxSampleAttempts caps re-prompts per sample when the user is silent.
const maxSampleAttempts = 3

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// runEnrollment guides the user through saying the wake phrase several times and
// returns the raw transcripts Vosk produced. Silence on a sample is re-prompted
// up to maxSampleAttempts times.
func runEnrollment(ctx context.Context, phrases <-chan string) []string {
	fmt.Println("--- Wake-phrase setup ---")
	fmt.Printf("Say \"hey ptolemy\" %d times so Ptolemy learns how you say it.\n", enrollmentSamples)
	samples := make([]string, 0, enrollmentSamples)
	for i := 1; i <= enrollmentSamples; i++ {
		captured := ""
		for attempt := 1; attempt <= maxSampleAttempts && captured == ""; attempt++ {
			fmt.Printf("  (%d/%d) Say \"hey ptolemy\" now...\n", i, enrollmentSamples)
			timer := time.NewTimer(perSampleTimeout)
			select {
			case <-ctx.Done():
				timer.Stop()
				return samples
			case phrase, ok := <-phrases:
				timer.Stop()
				if !ok {
					return samples
				}
				captured = strings.TrimSpace(phrase)
				if captured == "" {
					continue
				}
				fmt.Printf("      heard: %q\n", captured)
			case <-timer.C:
				fmt.Println("      (didn't catch that — try again)")
			}
		}
		if captured != "" {
			samples = append(samples, captured)
		}
	}
	fmt.Println("--- setup done ---")
	return samples
}
```

- [ ] **Step 5: Verify no leftover compile issues by inspection**

Confirm the file still imports `context`, `os`, `strings`, `time`, `fmt` (all already imported at lines 5-19) and that `IsWakePhrase` is no longer the only wake path (it remains defined in `internal/voice/command.go` and is still used by its own tests — leave it). No further edits.

- [ ] **Step 6: Commit**

```bash
git add cmd/ptolemy-voice/main.go
git commit -m "feat(voice): wake enrollment flow and profile-based wake matching"
```

---

## Task 5: Docs + Windows build/run verification

**Files:**
- Modify: `cmd/ptolemy-voice/README.md`

- [ ] **Step 1: Document enrollment in the README**

In `cmd/ptolemy-voice/README.md`, after the `### Convenience scripts` block (which ends with the ```` ```cmd ... run-vosk.bat -listen-only ... ``` ```` fence), add a new section:

````markdown
## Wake-phrase enrollment

Ptolemy learns how *you* say the wake phrase. The first time you run the Vosk
catcher with no saved profile, it runs a short setup: it asks you to say
"hey ptolemy" a few times and saves what the recognizer heard to
`.state\wake-profile.json`. Later runs load that profile automatically and
wake on close matches (not just the exact words).

Re-run setup any time:
```cmd
run-vosk.bat -enroll
```

The matcher always keeps the built-in "hey ptolemy" as a fallback, and uses a
small edit-distance tolerance so minor recognizer wobble still wakes it.
Override the profile location with `WAKE_PROFILE_PATH`.
````

- [ ] **Step 2: Build on Windows via interop**

Run: `cmd.exe /c "D:\Ptolemy\build-vosk.bat"`
Expected: `BUILD_EXIT=0` and `bin\ptolemy-voice-vosk.exe` is produced.

- [ ] **Step 3: Verify enrollment starts and exits cleanly (no mic interaction)**

Run a short, timed enrollment to confirm it reaches the prompt and the build wired up correctly (it will time out on silence and fall back, which is fine for a smoke test):

Run: `timeout 20 cmd.exe /c "D:\Ptolemy\run-vosk.bat -enroll > D:\Ptolemy\vosk-run.log 2>&1"` then read `vosk-run.log`.
Expected (in log): `--- Wake-phrase setup ---`, the `(1/4) Say "hey ptolemy" now...` prompts, and on silence the `(didn't catch that — try again)` lines; then a `Saved wake profile...` or default-only note, and `Enrollment complete.`

- [ ] **Step 4: Run the full Go test suite**

Run: `go test -p 1 ./...`
Expected: all packages `ok` / `no test files` (exit 0).

- [ ] **Step 5: Commit**

```bash
git add cmd/ptolemy-voice/README.md
git commit -m "docs(voice): document wake-phrase enrollment"
```

---

## Manual acceptance (user, on Windows)

These require a microphone and cannot be automated here:

1. `run-vosk.bat -enroll` → say "hey ptolemy" 4 times → confirm it echoes each `heard:` line and prints saved variants.
2. `run-vosk.bat` → say "hey ptolemy" → confirm `Wake phrase detected.` appears.
3. Inspect `.state\wake-profile.json` → confirm it contains your variants.
