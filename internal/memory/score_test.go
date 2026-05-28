package memory

import (
	"math"
	"testing"
)

func TestDecayScore_GlobalAndPinnedAreImmune(t *testing.T) {
	// global scope → 1.0 regardless of age/access.
	if got := decayScore(0.5, "global", false, 0, 365, 0.05); got != 1.0 {
		t.Fatalf("global should be 1.0, got %v", got)
	}
	// pinned project → 1.0 regardless of age.
	if got := decayScore(0.5, "project", true, 0, 365, 0.05); got != 1.0 {
		t.Fatalf("pinned should be 1.0, got %v", got)
	}
}

func TestDecayScore_DecaysWithAge(t *testing.T) {
	young := decayScore(1.0, "project", false, 0, 1, 0.05)
	old := decayScore(1.0, "project", false, 0, 100, 0.05)
	if !(old < young) {
		t.Fatalf("older project row must score lower: young=%v old=%v", young, old)
	}
	if young > 1.0 || old < 0 {
		t.Fatalf("score out of [0,1] range: young=%v old=%v", young, old)
	}
}

func TestDecayScore_ReinforcementFlattens(t *testing.T) {
	// Same age, more accesses → higher score (curve flattens).
	cold := decayScore(1.0, "project", false, 0, 30, 0.05)
	hot := decayScore(1.0, "project", false, 50, 30, 0.05)
	if !(hot > cold) {
		t.Fatalf("reinforcement should raise score: cold=%v hot=%v", cold, hot)
	}
}

func TestDecayScore_Formula(t *testing.T) {
	// Spot-check the exact formula for a project row.
	got := decayScore(0.8, "project", false, 3, 10, 0.05)
	want := 0.8 * math.Exp(-0.05*10/(1+3))
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("formula mismatch: got %v want %v", got, want)
	}
}
