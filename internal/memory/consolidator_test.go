package memory

import (
	"context"
	"testing"
	"time"
)

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
