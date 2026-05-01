package main

import (
	"errors"
	"testing"

	actionpkg "github.com/luannn010/ptolemy/internal/action"
)

func TestParseFirstValidJSONActionCleanSingleObject(t *testing.T) {
	action, warning, err := parseFirstValidJSONAction(`{"action":"read_file","path":"README.md"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if warning != "" {
		t.Fatalf("unexpected warning: %q", warning)
	}
	if action.Action != "read_file" {
		t.Fatalf("unexpected action: %+v", action)
	}
}

func TestParseFirstValidJSONActionJSONInsideMarkdown(t *testing.T) {
	action, warning, err := parseFirstValidJSONAction("```json\n{\"action\":\"read_file\",\"path\":\"README.md\"}\n```")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if warning != "" {
		t.Fatalf("unexpected warning: %q", warning)
	}
	if action.Path != "README.md" {
		t.Fatalf("unexpected action: %+v", action)
	}
}

func TestParseFirstValidJSONActionMultipleObjectsWarns(t *testing.T) {
	action, warning, err := parseFirstValidJSONAction("{\"action\":\"read_file\"}\n{\"action\":\"run_command\"}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action.Action != "read_file" {
		t.Fatalf("unexpected action: %+v", action)
	}
	if warning == "" {
		t.Fatal("expected warning for extra JSON objects")
	}
}

func TestParseFirstValidJSONActionProseBeforeAfterJSON(t *testing.T) {
	action, warning, err := parseFirstValidJSONAction("I will inspect a file.\n{\"action\":\"read_file\",\"path\":\"README.md\"}\nDone.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if warning != "" {
		t.Fatalf("unexpected warning: %q", warning)
	}
	if action.Path != "README.md" {
		t.Fatalf("unexpected action: %+v", action)
	}
}

func TestParseFirstValidJSONActionInvalidJSON(t *testing.T) {
	_, _, err := parseFirstValidJSONAction(`{"action":"read_file",}`)
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, errNoJSONObject) || errors.Is(err, actionpkg.ErrEmptyResponse) {
		t.Fatalf("expected invalid JSON error, got %v", err)
	}
}
