package main

import "testing"

func TestMemoryScopeDefaults(t *testing.T) {
	t.Setenv("PTOLEMY_MEMORY_SUBJECT", "")
	t.Setenv("PTOLEMY_MEMORY_PROJECT", "")
	subject, project := memoryScope()
	if subject != defaultSubject {
		t.Fatalf("expected default subject %q, got %q", defaultSubject, subject)
	}
	if project != defaultProject {
		t.Fatalf("expected default project %q, got %q", defaultProject, project)
	}
}

func TestMemoryScopeFromEnv(t *testing.T) {
	t.Setenv("PTOLEMY_MEMORY_SUBJECT", "alice")
	t.Setenv("PTOLEMY_MEMORY_PROJECT", "proj-x")
	subject, project := memoryScope()
	if subject != "alice" || project != "proj-x" {
		t.Fatalf("expected (alice, proj-x), got (%q, %q)", subject, project)
	}
}

func TestBuildMemoryDepsSkipsWhenUnconfigured(t *testing.T) {
	// LoadConfig fails fast when DATABASE_URL is unset; buildMemoryDeps must
	// report ok=false (skip) rather than panic or crash the server.
	t.Setenv("DATABASE_URL", "")
	_, cleanup, ok := buildMemoryDeps(t.Context())
	if ok {
		t.Fatal("expected ok=false when DATABASE_URL is unset")
	}
	if cleanup != nil {
		t.Fatal("expected nil cleanup when memory is skipped")
	}
}
