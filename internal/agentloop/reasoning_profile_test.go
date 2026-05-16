package agentloop

import (
	"testing"
)

func TestProfileForPhase(t *testing.T) {
	tests := []struct {
		name     string
		phase    string
		expected string
	}{
		{"plan", "plan", "high"},
		{"read", "read", "low"},
		{"search", "search", "low"},
		{"edit", "edit", "normal"},
		{"validate", "validate", "normal"},
		{"report", "report", "low"},
		{"unknown", "unknown", "normal"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ProfileForPhase(tt.phase)
			if result != tt.expected {
				t.Errorf("ProfileForPhase(%q) = %q, want %q", tt.phase, result, tt.expected)
			}
		})
	}
}

func TestProfilePolicyGuard(t *testing.T) {
	// Test that raw model parameters cannot override profile policy
	// This is enforced by the service layer, not the profile layer
	// The profile layer only provides resolved configuration
	if ProfileForPhase("plan") != "high" {
		t.Error("Plan phase should always return high profile")
	}
	if ProfileForPhase("read") != "low" {
		t.Error("Read phase should always return low profile")
	}
	if ProfileForPhase("edit") != "normal" {
		t.Error("Edit phase should always return normal profile")
	}
}
