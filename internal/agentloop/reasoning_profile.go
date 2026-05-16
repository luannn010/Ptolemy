package agentloop

import (
	"context"
	"fmt"
)

// ReasoningProfile represents the reasoning profile for a phase
// Version 1: Model adapter receives only resolved configuration
// workerd owns reasoning profile policy

type ReasoningProfile string

const (
	ProfileLow    ReasoningProfile = "low"
	ProfileNormal ReasoningProfile = "normal"
	ProfileHigh   ReasoningProfile = "high"
)

// PhaseToProfile maps execution phases to reasoning profiles
// Policy: workerd owns reasoning profile policy
// Model adapter only receives resolved configuration
func PhaseToProfile(phase string) ReasoningProfile {
	switch phase {
	case "plan":
		return ProfileHigh
	case "read":
		return ProfileLow
	case "search":
		return ProfileLow
	case "edit":
		return ProfileNormal
	case "validate":
		return ProfileNormal
	case "report":
		return ProfileLow
	default:
		return ProfileNormal
	}
}

// ProfileForPhase preserves the legacy string-returning helper used by tests
// and older call sites while the runtime uses the typed profile API.
func ProfileForPhase(phase string) string {
	return string(PhaseToProfile(phase))
}

// PolicyGuard ensures model cannot override reasoning profile
// This prevents raw model parameters from setting reasoning directly
func PolicyGuard(ctx context.Context, phase string, rawParams map[string]interface{}) (ReasoningProfile, error) {
	// workerd owns reasoning profile policy
	// Model adapter only receives resolved configuration
	profile := PhaseToProfile(phase)

	// Check if model attempted to override
	if rawParams != nil {
		if _, exists := rawParams["reasoning_profile"]; exists {
			return profile, fmt.Errorf("policy violation: model cannot override reasoning profile")
		}
	}

	return profile, nil
}

// GetResolvedProfile returns the final resolved profile for a phase
// This is the configuration the model adapter receives
func GetResolvedProfile(ctx context.Context, phase string) (ReasoningProfile, error) {
	return PolicyGuard(ctx, phase, nil)
}
