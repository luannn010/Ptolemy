package bootstrap

import (
	"strings"
	"testing"
)

func TestBuildReturnsDefaultBootstrapPayload(t *testing.T) {
	resp := Build(Request{})

	if resp.RootPath != ".ptolemy" {
		t.Fatalf("unexpected root path: %q", resp.RootPath)
	}
	if resp.ServerURL != "http://localhost:8080" {
		t.Fatalf("unexpected server url: %q", resp.ServerURL)
	}
	if resp.ProjectName != "example-project" {
		t.Fatalf("unexpected project name: %q", resp.ProjectName)
	}
	if len(resp.ContextFiles) != 5 || resp.ContextFiles[0] != "architecture.md" {
		t.Fatalf("unexpected context files: %+v", resp.ContextFiles)
	}
	if !strings.Contains(resp.ClientConfigTemplate, `server_url: "http://localhost:8080"`) {
		t.Fatalf("unexpected client config template: %s", resp.ClientConfigTemplate)
	}
}

func TestBuildAppliesRequestOverrides(t *testing.T) {
	resp := Build(Request{
		ServerURL:   "https://ptolemy.example",
		ProjectName: "demo-project",
	})

	if resp.ServerURL != "https://ptolemy.example" {
		t.Fatalf("unexpected server url: %q", resp.ServerURL)
	}
	if resp.ProjectName != "demo-project" {
		t.Fatalf("unexpected project name: %q", resp.ProjectName)
	}
	if !strings.Contains(resp.ClientConfigTemplate, `project_name: "demo-project"`) {
		t.Fatalf("unexpected client config template: %s", resp.ClientConfigTemplate)
	}
}
