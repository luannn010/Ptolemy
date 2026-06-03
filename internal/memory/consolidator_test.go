package memory

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestConsolidator_Run_CancellationClosesDone verifies that cancelling the
// context causes Consolidator.Run to close the done channel promptly.
// Uses freshDB so the migrations are applied; skips locally without DATABASE_URL.
func TestConsolidator_Run_CancellationClosesDone(t *testing.T) {
	conn := freshDB(t)
	c := NewConsolidator(conn, NewPgStore(conn), fakeChat{resp: "{}"}, ConsolidateConfig{
		Interval: time.Millisecond,
		Buffer:   99, MinAtoms: 99,
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before Run so runLoop exits on the first select
	done := make(chan struct{})
	c.Run(ctx, done)
	select {
	case <-done:
		// expected
	default:
		t.Fatal("done channel was not closed after Run returned")
	}
}

// TestConsolidator_BuildSynthChunk exercises the pure helper that constructs the
// synthesis Chunk from a Synthesis struct without any DB or LLM calls.
func TestConsolidator_BuildSynthChunk_Shape(t *testing.T) {
	c := NewConsolidator(nil, nil, fakeChat{resp: "{}"}, ConsolidateConfig{})
	syn := Synthesis{Content: "the gc sweep archives stale rows", SourceIDs: []string{"a1", "a2"}}
	vec := []float32{0.1, 0.2, 0.3, 0.4}
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	chunk := c.buildSynthChunk("userA", "ptolemy", syn, vec, now)

	if !strings.HasPrefix(chunk.ID, "synth:") {
		t.Fatalf("ID should start with 'synth:', got %q", chunk.ID)
	}
	if chunk.Content != syn.Content {
		t.Fatalf("Content mismatch: got %q, want %q", chunk.Content, syn.Content)
	}
	if chunk.Scope != "project" {
		t.Fatalf("Scope = %q, want 'project'", chunk.Scope)
	}
	if chunk.Status != "active" {
		t.Fatalf("Status = %q, want 'active'", chunk.Status)
	}
	if *chunk.SubjectID != "userA" || *chunk.ProjectID != "ptolemy" {
		t.Fatalf("SubjectID/ProjectID mismatch: %q / %q", *chunk.SubjectID, *chunk.ProjectID)
	}
	kind, _ := chunk.Metadata["kind"].(string)
	if kind != "synthesis" {
		t.Fatalf("metadata.kind = %q, want 'synthesis'", kind)
	}
	// ID must be deterministic: same inputs → same ID.
	chunk2 := c.buildSynthChunk("userA", "ptolemy", syn, vec, now)
	if chunk.ID != chunk2.ID {
		t.Fatalf("buildSynthChunk not deterministic: %q vs %q", chunk.ID, chunk2.ID)
	}
}

// TestConsolidator_ConsolidateOnce_NoDue exercises consolidateOnce against an
// empty DB: dueSubjectProjects returns nothing, so the function returns nil
// without calling any LLM.
func TestConsolidator_ConsolidateOnce_NoDue(t *testing.T) {
	conn := freshDB(t)
	c := NewConsolidator(conn, NewPgStore(conn), fakeChat{resp: "{}"}, ConsolidateConfig{Buffer: 5, MinAtoms: 3})
	if err := c.consolidateOnce(context.Background()); err != nil {
		t.Fatalf("consolidateOnce on empty DB: %v", err)
	}
}

// TestConsolidator_ConsolidateOnce_WithDue covers the loop body in
// consolidateOnce: when there is one due (subject,project), it calls
// ConsolidateSubjectProject which returns early because atom count < MinAtoms.
func TestConsolidator_ConsolidateOnce_WithDue(t *testing.T) {
	conn := freshDB(t)
	store := NewPgStore(conn)
	subj, proj := "userOnce", "ptolemy"
	mkAtom := func(id string) Chunk {
		ss, pr, pe := subj, proj, "factual"
		return Chunk{ID: id, Content: id + " content", Embedding: []float32{1, 0, 0, 0}, PublishedAt: time.Now().UTC(),
			Scope: "project", Importance: 0.5, SubjectID: &ss, ProjectID: &pr, Perspective: &pe,
			Metadata: map[string]any{"kind": "atom"}}
	}
	// Insert exactly Buffer atoms so dueSubjectProjects returns this pair.
	atoms := make([]Chunk, 4)
	for i := range atoms {
		atoms[i] = mkAtom(strings.Repeat("a", i+1))
	}
	if err := store.Upsert(context.Background(), atoms); err != nil {
		t.Fatal(err)
	}
	// MinAtoms=10 means ConsolidateSubjectProject returns early (len(atoms) < MinAtoms).
	c := NewConsolidator(conn, store, fakeChat{resp: "{}"}, ConsolidateConfig{Buffer: 3, MinAtoms: 10})
	if err := c.consolidateOnce(context.Background()); err != nil {
		t.Fatalf("consolidateOnce with due pair but below MinAtoms: %v", err)
	}
}

func TestConsolidator_BuildPromptAndParse(t *testing.T) {
	atoms := []Chunk{
		{ID: "a1", Content: "the archive threshold is 0.1"},
		{ID: "a2", Content: "the sweep runs hourly"},
	}
	resp := `{"content":"On this project the GC archive threshold is 0.1 and the sweep runs hourly.","source_ids":["a1","a2"]}`
	c := NewConsolidator(nil, nil, fakeChat{resp: resp}, ConsolidateConfig{MinAtoms: 1})
	syn, err := c.synthesize(context.Background(), "", atoms)
	if err != nil {
		t.Fatal(err)
	}
	if syn.Content == "" || len(syn.SourceIDs) != 2 {
		t.Fatalf("bad synthesis: %+v", syn)
	}
}

func TestConsolidator_ParseHandlesFence(t *testing.T) {
	c := NewConsolidator(nil, nil, fakeChat{resp: "```json\n{\"content\":\"x summary text\",\"source_ids\":[\"a1\"]}\n```"}, ConsolidateConfig{})
	syn, err := c.synthesize(context.Background(), "prev", []Chunk{{ID: "a1", Content: "x"}})
	if err != nil || syn.Content != "x summary text" {
		t.Fatalf("fence parse failed: %+v err=%v", syn, err)
	}
}

func TestConsolidator_RevisionSupersedesPriorSummary(t *testing.T) {
	conn := freshDB(t)
	store := NewPgStore(conn)
	emb := fakeEmbedder{vecs: [][]float32{{1, 0, 0, 0}}}
	subj, proj := "userA", "ptolemy"
	mkAtom := func(id, content string) Chunk {
		ss, pr, pe := subj, proj, "factual"
		return Chunk{ID: id, Content: content, Embedding: []float32{1, 0, 0, 0}, PublishedAt: time.Now().UTC(),
			Scope: "project", Importance: 0.5, SubjectID: &ss, ProjectID: &pr, Perspective: &pe,
			Metadata: map[string]any{"kind": "atom"}}
	}
	if err := store.Upsert(context.Background(), []Chunk{mkAtom("a1", "archive threshold is 0.2")}); err != nil {
		t.Fatal(err)
	}
	c1 := NewConsolidator(conn, store, fakeChat{resp: `{"content":"archive threshold is 0.2 on ptolemy","source_ids":["a1"]}`}, ConsolidateConfig{MinAtoms: 1}).WithEmbedder(emb)
	if err := c1.ConsolidateSubjectProject(context.Background(), subj, proj); err != nil {
		t.Fatal(err)
	}
	if err := store.Upsert(context.Background(), []Chunk{mkAtom("a2", "archive threshold is now 0.1")}); err != nil {
		t.Fatal(err)
	}
	c2 := NewConsolidator(conn, store, fakeChat{resp: `{"content":"archive threshold is 0.1 on ptolemy","source_ids":["a1","a2"]}`}, ConsolidateConfig{MinAtoms: 1}).WithEmbedder(emb)
	if err := c2.ConsolidateSubjectProject(context.Background(), subj, proj); err != nil {
		t.Fatal(err)
	}
	var nActive int
	var content string
	if err := conn.QueryRow(context.Background(), `
		SELECT count(*), max(content) FROM chunks
		WHERE metadata->>'kind'='synthesis' AND status='active' AND subject_id=$1 AND project_id=$2`,
		subj, proj).Scan(&nActive, &content); err != nil {
		t.Fatal(err)
	}
	if nActive != 1 {
		t.Fatalf("want exactly 1 active synthesis, got %d", nActive)
	}
	if !containsFold(content, "0.1") || containsFold(content, "0.2") {
		t.Fatalf("active summary must reflect current truth 0.1, not stale 0.2: %q", content)
	}
}

func TestConsolidator_DueRespectsBuffer(t *testing.T) {
	conn := freshDB(t)
	store := NewPgStore(conn)
	subj, proj := "userA", "ptolemy"
	mkAtom := func(id string) Chunk {
		ss, pr, pe := subj, proj, "factual"
		return Chunk{ID: id, Content: id + " content detail", Embedding: []float32{1, 0, 0, 0}, PublishedAt: time.Now().UTC(),
			Scope: "project", Importance: 0.5, SubjectID: &ss, ProjectID: &pr, Perspective: &pe, Metadata: map[string]any{"kind": "atom"}}
	}
	if err := store.Upsert(context.Background(), []Chunk{mkAtom("a1"), mkAtom("a2")}); err != nil {
		t.Fatal(err)
	}
	c := NewConsolidator(conn, store, fakeChat{resp: "{}"}, ConsolidateConfig{Buffer: 3, MinAtoms: 1})
	due, err := c.dueSubjectProjects(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Fatalf("2 atoms < buffer 3 → not due; got %v", due)
	}
	if err := store.Upsert(context.Background(), []Chunk{mkAtom("a3")}); err != nil {
		t.Fatal(err)
	}
	due, err = c.dueSubjectProjects(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 {
		t.Fatalf("3 atoms >= buffer 3 → due; got %v", due)
	}
}
