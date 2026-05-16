package logging

import (
	"testing"

	"github.com/rs/zerolog"
)

func TestSetupDebugLevel(t *testing.T) {
	Setup("debug")

	if zerolog.GlobalLevel() != zerolog.DebugLevel {
		t.Fatalf("expected debug level, got %v", zerolog.GlobalLevel())
	}
}

func TestSetupInfoLevel(t *testing.T) {
	Setup("info")

	if zerolog.GlobalLevel() != zerolog.InfoLevel {
		t.Fatalf("expected info level, got %v", zerolog.GlobalLevel())
	}
}

func TestSetupUnknownDefaultsToDebug(t *testing.T) {
	Setup("unknown")

	if zerolog.GlobalLevel() != zerolog.DebugLevel {
		t.Fatalf("expected debug level for unknown input, got %v", zerolog.GlobalLevel())
	}
}
