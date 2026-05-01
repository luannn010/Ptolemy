package main

import (
	"context"
	"fmt"
	"strings"
	"testing"

	actionpkg "github.com/luannn010/ptolemy/internal/action"
	"github.com/luannn010/ptolemy/internal/worker"
)

func TestValidateAndNormalizeActionReplaceBlockRequiresFields(t *testing.T) {
	action := &testActionEnvelope{Action: "replace_block", Path: "a.go", New: "x"}
	got := validateAndNormalizeAction(action.toEnvelope())
	if got == "" {
		t.Fatal("expected corrective prompt")
	}
	if !strings.Contains(got, "replace_block") {
		t.Fatalf("unexpected corrective prompt: %q", got)
	}
}

func TestValidateAndNormalizeActionInsertAfterRequiresFields(t *testing.T) {
	action := &testActionEnvelope{Action: "insert_after", Path: "a.go", Marker: "m"}
	got := validateAndNormalizeAction(action.toEnvelope())
	if got == "" {
		t.Fatal("expected corrective prompt")
	}
	if !strings.Contains(got, "insert_after") {
		t.Fatalf("unexpected corrective prompt: %q", got)
	}
}

func TestValidateAndNormalizeActionCreateFileNormalizesToWriteFile(t *testing.T) {
	envelope := (&testActionEnvelope{
		Action:  "create_file",
		Path:    "a.txt",
		Content: "hello",
	}).toEnvelope()

	got := validateAndNormalizeAction(envelope)
	if got != "" {
		t.Fatalf("unexpected corrective prompt: %q", got)
	}
	if envelope.Action != "write_file" {
		t.Fatalf("action = %q, want write_file", envelope.Action)
	}
}

func TestValidateAndNormalizeActionUpdateFilePatchNormalizesToReplaceBlock(t *testing.T) {
	envelope := (&testActionEnvelope{
		Action: "update_file",
		Path:   "a.txt",
		Old:    "old",
		New:    "new",
	}).toEnvelope()

	got := validateAndNormalizeAction(envelope)
	if got != "" {
		t.Fatalf("unexpected corrective prompt: %q", got)
	}
	if envelope.Action != "replace_block" {
		t.Fatalf("action = %q, want replace_block", envelope.Action)
	}
}

func TestProcessBrainReplyIncompleteActionReturnsCorrectivePrompt(t *testing.T) {
	chdirTemp(t)
	runtime, _ := newTestRuntime(t)

	reply := `{"action":"replace_block","path":"internal/client/workspace/guard.go","new":"new block"}`
	action, result, ok := processBrainReply(context.Background(), runtime, "session-1", ".", "my-task", 1, reply, false, &progressGuard{})

	if ok {
		t.Fatal("expected ok=false for incomplete action")
	}
	if action != nil {
		t.Fatalf("expected nil action, got %+v", action)
	}
	if !strings.Contains(result.Display, "INCOMPLETE ACTION") {
		t.Fatalf("unexpected display: %q", result.Display)
	}
	if !strings.Contains(result.Brain, "replace_block action is incomplete") {
		t.Fatalf("unexpected corrective brain text: %q", result.Brain)
	}
}

func TestProcessBrainReplyRepeatedReadFileTriggersProgressGuard(t *testing.T) {
	chdirTemp(t)
	runtime, _ := newTestRuntime(t)
	runtime.workerClient = failingReadWorkerClient{}
	guard := &progressGuard{}
	reply := `{"action":"read_file","path":"internal/client/workspace/guard.go","reason":"inspect current implementation"}`

	action, result, ok := processBrainReply(context.Background(), runtime, "session-1", ".", "my-task", 1, reply, false, guard)
	if !ok {
		t.Fatal("expected first read_file to execute before the guard triggers")
	}
	if action == nil || action.Action != "read_file" {
		t.Fatalf("expected parsed action on first attempt, got %+v", action)
	}
	if !strings.Contains(result.Display, "ERROR reading file") {
		t.Fatalf("expected worker read failure on first attempt, got %q", result.Display)
	}
	if strings.Contains(result.Display, "PROGRESS GUARD") {
		t.Fatalf("first attempt should not trigger progress guard: %q", result.Display)
	}

	action, result, ok = processBrainReply(context.Background(), runtime, "session-1", ".", "my-task", 2, reply, false, guard)
	if ok {
		t.Fatal("expected repeated read_file to be blocked")
	}
	if action != nil {
		t.Fatalf("expected nil action when progress guard blocks, got %+v", action)
	}
	if !strings.Contains(result.Display, "PROGRESS GUARD") {
		t.Fatalf("unexpected display: %q", result.Display)
	}
	if !strings.Contains(result.Brain, "Stop rereading the same file") {
		t.Fatalf("unexpected corrective brain text: %q", result.Brain)
	}
}

type failingReadWorkerClient struct {
	noopWorkerClient
}

func (failingReadWorkerClient) ReadFile(ctx context.Context, reqBody worker.ReadFileRequest) (*worker.ReadFileResponse, error) {
	return nil, fmt.Errorf("forced read failure")
}

func TestValidateAndNormalizeActionRejectsInstructionAssetMutation(t *testing.T) {
	envelope := (&testActionEnvelope{
		Action:  "insert_after",
		Path:    "docs/tasks/packs/ptolemy-client-server-pack/task-scripts/04-client-workspace-guard.md",
		Marker:  "## Instructions",
		Content: "new snippet",
	}).toEnvelope()

	got := validateAndNormalizeAction(envelope)
	if got == "" {
		t.Fatal("expected corrective prompt")
	}
	if !strings.Contains(got, "instruction assets") {
		t.Fatalf("unexpected corrective prompt: %q", got)
	}
}

type testActionEnvelope struct {
	Action  string
	Path    string
	Content string
	Old     string
	New     string
	Marker  string
}

func (t *testActionEnvelope) toEnvelope() *actionEnvelope {
	return &actionpkg.ActionEnvelope{
		Action:  t.Action,
		Path:    t.Path,
		Content: t.Content,
		Old:     t.Old,
		New:     t.New,
		Marker:  t.Marker,
	}
}

type actionEnvelope = actionpkg.ActionEnvelope
