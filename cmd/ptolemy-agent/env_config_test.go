package main

import "testing"

func TestGetenvDefaultUsesFallbackForBlank(t *testing.T) {
	t.Setenv("PTOLEMY_TEST_VALUE", "   ")

	got := getenvDefault("PTOLEMY_TEST_VALUE", "fallback")
	if got != "fallback" {
		t.Fatalf("expected fallback, got %q", got)
	}
}

func TestGetenvDefaultUsesTrimmedEnvValue(t *testing.T) {
	t.Setenv("PTOLEMY_TEST_VALUE", "  custom-value  ")

	got := getenvDefault("PTOLEMY_TEST_VALUE", "fallback")
	if got != "custom-value" {
		t.Fatalf("expected trimmed env value, got %q", got)
	}
}
