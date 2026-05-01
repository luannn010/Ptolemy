package main

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	actionpkg "github.com/luannn010/ptolemy/internal/action"
	"github.com/luannn010/ptolemy/internal/logs"
	"github.com/luannn010/ptolemy/internal/store"
	"github.com/luannn010/ptolemy/internal/worker"
)

type noopWorkerClient struct{}

func (noopWorkerClient) CreateSession(ctx context.Context, reqBody worker.CreateSessionRequest) (*worker.Session, error) {
	return nil, nil
}

func (noopWorkerClient) RunCommand(ctx context.Context, sessionID string, reqBody worker.RunCommandRequest) (*worker.CommandResult, error) {
	return nil, nil
}

func (noopWorkerClient) ReadFile(ctx context.Context, reqBody worker.ReadFileRequest) (*worker.ReadFileResponse, error) {
	return nil, nil
}

func (noopWorkerClient) WriteFile(ctx context.Context, reqBody worker.WriteFileRequest) (*worker.WriteFileResponse, error) {
	return nil, nil
}

func TestProcessBrainReplyMultipleObjectsUsesFirstValidActionAndWarns(t *testing.T) {
	chdirTemp(t)
	runtime, db := newTestRuntime(t)

	reply := "{\"action\":\"explain\",\"reason\":\"done\"}\n{\"action\":\"run_command\"}"
	action, result, ok := processBrainReply(context.Background(), runtime, "session-1", ".", "my-task", 2, reply, false, &progressGuard{})
	if !ok {
		t.Fatal("processBrainReply() ok = false, want true")
	}
	if action.Action != "explain" {
		t.Fatalf("action.Action = %q, want explain", action.Action)
	}
	if !strings.Contains(result.Display, "ignored extra JSON objects") {
		t.Fatalf("result.Display = %q, want warning", result.Display)
	}

	var message string
	if err := db.QueryRow(`SELECT message FROM logs LIMIT 1`).Scan(&message); err != nil {
		t.Fatalf("query logs: %v", err)
	}
	if !strings.Contains(message, "ignored extra JSON objects") {
		t.Fatalf("message = %q, want ignored extra JSON objects", message)
	}
}

func TestQueueTaskBatchQueuesChildrenWithoutExecution(t *testing.T) {
	chdirTemp(t)
	runtime, db := newTestRuntime(t)

	action := &actionpkg.ActionEnvelope{
		Action: "create_task_batch",
		Tasks: []actionpkg.BatchTask{
			{Type: "read_file", Path: "docs/PLAN.md"},
			{Type: "run_command", Command: "go test ./..."},
		},
	}

	result := queueTaskBatch(context.Background(), runtime, "session-2", action)
	if !strings.Contains(result.Display, "TASK BATCH QUEUED") {
		t.Fatalf("result.Display = %q", result.Display)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM actions`).Scan(&count); err != nil {
		t.Fatalf("count actions: %v", err)
	}
	if count != 3 {
		t.Fatalf("actions count = %d, want 3", count)
	}

	rows, err := db.Query(`SELECT type, status FROM actions ORDER BY created_at ASC`)
	if err != nil {
		t.Fatalf("query actions: %v", err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var typ string
		var status string
		if err := rows.Scan(&typ, &status); err != nil {
			t.Fatal(err)
		}
		got = append(got, typ+":"+status)
	}

	want := []string{"create_task_batch:queued", "read_file:pending", "run_command:pending"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("actions = %v, want %v", got, want)
	}
}

func newTestRuntime(t *testing.T) (*agentRuntime, *sql.DB) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	baseStore, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		_ = baseStore.Close()
	})

	if err := store.RunMigrations(context.Background(), baseStore.SQLDB()); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	return &agentRuntime{
		workerClient: noopWorkerClient{},
		actionStore:  actionpkg.NewStore(baseStore.SQLDB()),
		logStore:     logs.NewStore(baseStore.SQLDB()),
		splitter:     actionpkg.PlaceholderTaskSplitter{},
	}, baseStore.SQLDB()
}
