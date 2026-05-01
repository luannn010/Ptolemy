package skillsync

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListSkillsAndSync(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/skills":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"id":"go/fmt","name":"Go Format","content":"# go fmt\n"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".ptolemy"), 0o755); err != nil {
		t.Fatal(err)
	}
	clientYAML := "server_url: \"" + server.URL + "\"\nskills_cache: \".ptolemy/cache/skills\"\n"
	if err := os.WriteFile(filepath.Join(workspace, ".ptolemy", "client.yaml"), []byte(clientYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	skills, err := ListSkills(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 || skills[0].ID != "go/fmt" {
		t.Fatalf("unexpected skills: %+v", skills)
	}

	result, err := SyncSkills(workspace, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Downloaded) != 1 {
		t.Fatalf("unexpected downloaded: %+v", result.Downloaded)
	}

	data, err := os.ReadFile(filepath.Join(workspace, ".ptolemy", "cache", "skills", "go-fmt.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "go fmt") {
		t.Fatalf("unexpected cached skill content: %q", string(data))
	}
}

func TestListSkillsOfflineGracefulError(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".ptolemy"), 0o755); err != nil {
		t.Fatal(err)
	}
	clientYAML := "server_url: \"http://127.0.0.1:1\"\nskills_cache: \".ptolemy/cache/skills\"\n"
	if err := os.WriteFile(filepath.Join(workspace, ".ptolemy", "client.yaml"), []byte(clientYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ListSkills(workspace)
	if err == nil {
		t.Fatal("expected offline error")
	}
	if !strings.Contains(err.Error(), ErrServerUnavailable.Error()) {
		t.Fatalf("expected graceful offline error, got %v", err)
	}
}
