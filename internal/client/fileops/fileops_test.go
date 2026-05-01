package fileops

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadWrite(t *testing.T) {
	ws := t.TempDir()
	path := filepath.Join(ws, "a.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	client, err := New(Options{Workspace: ws})
	if err != nil {
		t.Fatal(err)
	}

	content, err := client.Read("a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if content != "hello" {
		t.Fatalf("read content = %q", content)
	}

	if err := client.Write("a.txt", "world"); err != nil {
		t.Fatal(err)
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(updated) != "world" {
		t.Fatalf("updated content = %q", string(updated))
	}
}

func TestInsertAfter(t *testing.T) {
	ws := t.TempDir()
	path := filepath.Join(ws, "b.txt")
	initial := "line1\nmarker\nline2\n"
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}
	client, err := New(Options{Workspace: ws})
	if err != nil {
		t.Fatal(err)
	}

	if err := client.InsertAfter("b.txt", "marker", "inserted"); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "marker\ninserted\nline2") {
		t.Fatalf("unexpected content: %q", string(got))
	}
}

func TestReplaceBetween(t *testing.T) {
	ws := t.TempDir()
	path := filepath.Join(ws, "c.txt")
	initial := "before\nSTART\nold\nEND\nafter\n"
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}
	client, err := New(Options{Workspace: ws})
	if err != nil {
		t.Fatal(err)
	}

	if err := client.ReplaceBetween("c.txt", "START\n", "\nEND", "new"); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "before\nSTART\nnew\nEND\nafter\n" {
		t.Fatalf("unexpected content: %q", string(got))
	}
}

func TestDeleteRequiresPermission(t *testing.T) {
	ws := t.TempDir()
	path := filepath.Join(ws, "d.txt")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	client, err := New(Options{Workspace: ws})
	if err != nil {
		t.Fatal(err)
	}

	err = client.Delete("d.txt", false)
	if !errors.Is(err, ErrDeleteNotPermitted) {
		t.Fatalf("expected ErrDeleteNotPermitted, got %v", err)
	}

	if err := client.Delete("d.txt", true); err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("expected file deleted, stat err = %v", statErr)
	}
}

func TestWriteRejectsOutsideWorkspace(t *testing.T) {
	ws := t.TempDir()
	client, err := New(Options{Workspace: ws})
	if err != nil {
		t.Fatal(err)
	}

	err = client.Write("../escape.txt", "x")
	if err == nil {
		t.Fatal("expected outside-workspace error")
	}
}
