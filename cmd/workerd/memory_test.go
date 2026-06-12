package main

import (
	"context"
	"testing"
)

func TestMemoryScope_Defaults(t *testing.T) {
	t.Setenv("PTOLEMY_MEMORY_SUBJECT", "")
	t.Setenv("PTOLEMY_MEMORY_PROJECT", "")
	subject, project := memoryScope()
	if subject != "default" || project != "ptolemy" {
		t.Fatalf("expected default/ptolemy, got %q/%q", subject, project)
	}
}

func TestMemoryScope_EnvOverride(t *testing.T) {
	t.Setenv("PTOLEMY_MEMORY_SUBJECT", "alice")
	t.Setenv("PTOLEMY_MEMORY_PROJECT", "proj2")
	subject, project := memoryScope()
	if subject != "alice" || project != "proj2" {
		t.Fatalf("expected alice/proj2, got %q/%q", subject, project)
	}
}

func TestBuildRAGDeps_DisabledWithoutDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	deps, cleanup, ok := buildRAGDeps(context.Background())
	if ok {
		t.Fatal("expected ok=false without DATABASE_URL")
	}
	if cleanup != nil {
		t.Fatal("expected nil cleanup when disabled")
	}
	if deps.Answerer != nil {
		t.Fatalf("expected empty deps when disabled, got %+v", deps)
	}
}
