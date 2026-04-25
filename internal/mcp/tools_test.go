package mcp

import "testing"

func TestNewTool(t *testing.T) {
	schema := map[string]any{
		"type": "object",
	}

	tool := NewTool("ptolemy.test", "test tool", schema)

	if tool.Name != "ptolemy.test" {
		t.Fatalf("expected tool name ptolemy.test, got %s", tool.Name)
	}

	if tool.Description != "test tool" {
		t.Fatalf("expected description test tool, got %s", tool.Description)
	}

	if tool.InputSchema["type"] != "object" {
		t.Fatalf("expected object schema")
	}
}

func TestTextResult(t *testing.T) {
	result := TextResult([]byte(`{"ok":true}`))

	content, ok := result["content"].([]map[string]any)
	if !ok {
		t.Fatalf("expected content to be []map[string]any")
	}

	if len(content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(content))
	}

	if content[0]["type"] != "text" {
		t.Fatalf("expected type text, got %v", content[0]["type"])
	}

	if content[0]["text"] != `{"ok":true}` {
		t.Fatalf("unexpected text result: %v", content[0]["text"])
	}
}
