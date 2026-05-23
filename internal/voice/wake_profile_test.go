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
