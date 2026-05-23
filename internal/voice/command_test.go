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

func TestParseRunShell(t *testing.T) {
	now := time.Now()
	cases := map[string]string{
		"run go test ./...":       "go test ./...",
		"run command git status":  "git status",
		"RUN  go   version":       "go version",
	}
	for input, wantShell := range cases {
		cmd, ok := ParseCommand(input, now)
		if !ok {
			t.Fatalf("expected %q to parse", input)
		}
		if cmd.Type != CommandRunShell {
			t.Fatalf("%q: expected run_shell command, got %s", input, cmd.Type)
		}
		if cmd.Shell != wantShell {
			t.Fatalf("%q: expected shell %q, got %q", input, wantShell, cmd.Shell)
		}
	}
}

func TestParseRunShellKeepsExistingCommands(t *testing.T) {
	now := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	cases := map[string]CommandType{
		"sleep pc":                          CommandSleepPC,
		"set alarm 7:30 am":                 CommandSetAlarm,
		"set reminder buy milk in 10 minutes": CommandSetReminder,
	}
	for input, wantType := range cases {
		cmd, ok := ParseCommand(input, now)
		if !ok {
			t.Fatalf("expected %q to parse", input)
		}
		if cmd.Type != wantType {
			t.Fatalf("%q: expected %s, got %s (run-shell branch must not shadow it)", input, wantType, cmd.Type)
		}
	}

	// "run" with no command should not parse as a runnable shell command.
	if _, ok := ParseCommand("run", now); ok {
		t.Fatal("bare 'run' should not parse")
	}
}

func TestIsConfirmPhrase(t *testing.T) {
	for _, p := range []string{"confirm", "yes do it", "execute", "  Confirm  "} {
		if !IsConfirmPhrase(p) {
			t.Fatalf("expected %q to be a confirm phrase", p)
		}
	}
	for _, p := range []string{"cancel", "no", "run go test", ""} {
		if IsConfirmPhrase(p) {
			t.Fatalf("did not expect %q to be a confirm phrase", p)
		}
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
