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
