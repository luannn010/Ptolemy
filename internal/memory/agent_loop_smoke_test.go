//go:build smoke

package memory_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/luannn010/ptolemy/internal/memory"
	"github.com/luannn010/ptolemy/internal/memory/eval"
)

// buildAgentModule loads config, swaps in the eval DB, force-enables the agent
// loop, builds the module, and ingests the fixture corpus. Skips when the
// required env (BRAIN_*/EMBEDDING_*/DATABASE_URL/RAG_FIXTURE_DIR) is absent.
// Returns the orchestrator (with AgentLoop attached) and a cleanup func.
func buildAgentModule(t *testing.T) (*memory.Orchestrator, func()) {
	t.Helper()
	for _, k := range []string{"BRAIN_BASE_URL", "BRAIN_MODEL", "EMBEDDING_BASE_URL", "EMBEDDING_MODEL", "RAG_FIXTURE_DIR"} {
		if strings.TrimSpace(os.Getenv(k)) == "" {
			t.Skipf("%s not set; run via make smoke-agent", k)
		}
	}
	// Force the loop on regardless of the ambient AGENT_LOOP_ENABLED.
	t.Setenv("AGENT_LOOP_ENABLED", "true")
	cfg, err := memory.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if evalURL := strings.TrimSpace(os.Getenv("MEMORY_EVAL_DATABASE_URL")); evalURL != "" {
		cfg.DatabaseURL = evalURL
	}
	ctx := context.Background()
	orch, conn, err := memory.NewModule(ctx, cfg)
	if err != nil {
		t.Fatalf("NewModule: %v", err)
	}
	if orch.AgentLoop == nil {
		_ = conn.Close(ctx)
		t.Fatal("expected AgentLoop to be wired when AGENT_LOOP_ENABLED=true")
	}
	docs, err := eval.LoadFixtureCorpus(os.Getenv("RAG_FIXTURE_DIR"))
	if err != nil {
		_ = conn.Close(ctx)
		t.Fatalf("LoadFixtureCorpus: %v", err)
	}
	for _, d := range docs {
		if err := orch.Ingest(ctx, d); err != nil {
			_ = conn.Close(ctx)
			t.Fatalf("ingest %s: %v", d.Source, err)
		}
	}
	return orch, func() { _ = conn.Close(ctx) }
}

func seedQuestion(t *testing.T, id string) eval.Question {
	t.Helper()
	seed, err := eval.LoadSeed("eval/testdata/seed.json")
	if err != nil {
		t.Fatalf("LoadSeed: %v", err)
	}
	for _, q := range seed.Questions {
		if q.ID == id {
			return q
		}
	}
	t.Fatalf("seed question %q not found", id)
	return eval.Question{}
}

func TestAgentLoopSmoke_KnownQuestion(t *testing.T) {
	orch, cleanup := buildAgentModule(t)
	defer cleanup()
	q := seedQuestion(t, "p03-fileops-paraphrase")
	ans, err := orch.Answer(context.Background(), memory.Query{Text: q.Text, K: 5})
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	t.Logf("answer=%q gaveup=%v cites=%v", ans.Text, ans.GaveUp, ans.Citations)
	if ans.GaveUp {
		t.Fatalf("known question should produce a grounded answer, but gave up: %q", ans.Text)
	}
	if len(ans.Citations) == 0 {
		t.Fatal("expected at least one citation on a grounded answer")
	}
}

func TestAgentLoopSmoke_UnanswerableQuestion(t *testing.T) {
	orch, cleanup := buildAgentModule(t)
	defer cleanup()
	q := seedQuestion(t, "n01-no-grpc")
	ans, err := orch.Answer(context.Background(), memory.Query{Text: q.Text, K: 5})
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	t.Logf("answer=%q gaveup=%v", ans.Text, ans.GaveUp)
	if !ans.GaveUp {
		t.Fatalf("unanswerable question should give_up, but answered: %q", ans.Text)
	}
}
