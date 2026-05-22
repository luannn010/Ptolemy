package voice

import (
	"testing"
	"time"
)

func TestIsWakePhrase(t *testing.T) {
	if !IsWakePhrase("hey ptolemy") {
		t.Fatal("expected wake phrase to match")
	}
	if IsWakePhrase("hello there") {
		t.Fatal("did not expect non-wake phrase to match")
	}
}

func TestParseSleepCommand(t *testing.T) {
	cmd, ok := ParseCommand("sleep pc", time.Now())
	if !ok {
		t.Fatal("expected command to parse")
	}
	if cmd.Type != CommandSleepPC {
		t.Fatalf("expected sleep command, got %s", cmd.Type)
	}
}

func TestParseSetAlarm(t *testing.T) {
	now := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	cmd, ok := ParseCommand("set alarm 7:30 am", now)
	if !ok {
		t.Fatal("expected command to parse")
	}
	if cmd.Type != CommandSetAlarm {
		t.Fatalf("expected alarm command, got %s", cmd.Type)
	}
	if cmd.When.Hour() != 7 || cmd.When.Minute() != 30 {
		t.Fatalf("expected 07:30 alarm, got %s", cmd.When.Format(time.RFC3339))
	}
}

func TestParseReminderInMinutes(t *testing.T) {
	now := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	cmd, ok := ParseCommand("set reminder buy milk in 10 minutes", now)
	if !ok {
		t.Fatal("expected reminder command to parse")
	}
	if cmd.Type != CommandSetReminder {
		t.Fatalf("expected reminder command, got %s", cmd.Type)
	}
	if cmd.Message != "buy milk" {
		t.Fatalf("expected message buy milk, got %q", cmd.Message)
	}
	if cmd.When.Sub(now) != 10*time.Minute {
		t.Fatalf("expected 10 minute reminder, got %s", cmd.When.Sub(now))
	}
}

func TestCommandWindowTimeout(t *testing.T) {
	until := time.Date(2026, 5, 22, 10, 0, 30, 0, time.UTC)
	if IsCommandWindowExpired(until, until.Add(-1*time.Second)) {
		t.Fatal("window should not be expired")
	}
	if !IsCommandWindowExpired(until, until) {
		t.Fatal("window should be expired at deadline")
	}
}
