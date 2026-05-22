package skills

import "testing"

func TestIsImplementationTask(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		want   bool
	}{
		{name: "implementation task true", prompt: "add tests for auth service", want: true},
		{name: "explanation request false", prompt: "what does this code do?", want: false},
		{name: "template request only false", prompt: "give me a template for AGENTS.md", want: false},
		{name: "template file creation in repo true", prompt: "create PLAN_TEMPLATE.md in the repo", want: true},
		{name: "command help false", prompt: "give me the curl command", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsImplementationTask(tt.prompt)
			if got != tt.want {
				t.Fatalf("IsImplementationTask(%q)=%v want=%v", tt.prompt, got, tt.want)
			}
		})
	}
}

func TestClassifyTaskLevel(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		want   string
	}{
		{name: "non task none", prompt: "explain this error", want: "none"},
		{name: "small task", prompt: "fix typo in README", want: "small"},
		{name: "medium task", prompt: "implement decision_gate classification", want: "medium"},
		{name: "large task", prompt: "implement full decision gate prehook with routing, plan mode, and task pack creator", want: "large"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyTaskLevel(tt.prompt)
			if got != tt.want {
				t.Fatalf("ClassifyTaskLevel(%q)=%q want=%q", tt.prompt, got, tt.want)
			}
		})
	}
}

func TestEvaluatePrehookRoutes(t *testing.T) {
	tests := []struct {
		name      string
		prompt    string
		wantTask  bool
		wantLevel string
		wantRoute string
	}{
		{name: "not task route", prompt: "why is this test failing?", wantTask: false, wantLevel: "none", wantRoute: "answer_directly"},
		{name: "small route", prompt: "update AGENTS.md wording", wantTask: true, wantLevel: "small", wantRoute: "direct_execute"},
		{name: "medium route", prompt: "add validation for task plan format", wantTask: true, wantLevel: "medium", wantRoute: "plan_template"},
		{name: "large route", prompt: "add auth system", wantTask: true, wantLevel: "large", wantRoute: "task_pack_creator"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EvaluatePrehook(tt.prompt)
			if got.IsTask != tt.wantTask || got.Level != tt.wantLevel || got.Route != tt.wantRoute {
				t.Fatalf("EvaluatePrehook(%q)=%+v want is_task=%v level=%q route=%q", tt.prompt, got, tt.wantTask, tt.wantLevel, tt.wantRoute)
			}
			if got.Reason == "" {
				t.Fatalf("EvaluatePrehook(%q) reason should not be empty", tt.prompt)
			}
		})
	}
}

