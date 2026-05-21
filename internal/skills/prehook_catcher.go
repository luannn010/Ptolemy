package skills

import "strings"

type PrehookResult struct {
	IsTask bool   `json:"is_task"`
	Level  string `json:"level"`
	Route  string `json:"route"`
	Reason string `json:"reason"`
}

func IsImplementationTask(prompt string) bool {
	p := normalize(prompt)
	if p == "" {
		return false
	}

	nonImplementationSignals := []string{
		"what", "why", "explain", "summary", "summarize", "template for", "give me the curl command",
		"command help", "how do i",
	}
	if containsAny(p, nonImplementationSignals) {
		return false
	}

	implementationSignals := []string{
		"implement", "add ", "fix ", "create ", "update ", "refactor", "remove ", "rename ",
		"write ", "modify ", "change ", "edit ", "delete ", "build ",
	}
	if !containsAny(p, implementationSignals) {
		return false
	}

	// Guardrail: template/prompt-only asks are not implementation tasks unless file/repo write is explicit.
	if containsAny(p, []string{"template", "prompt"}) &&
		!containsAny(p, []string{"create ", "update ", "write ", "edit ", "in the repo", "file"}) {
		return false
	}

	return true
}

func ClassifyTaskLevel(prompt string) string {
	if !IsImplementationTask(prompt) {
		return "none"
	}

	p := normalize(prompt)

	largeSignals := []string{
		"whole", "system-wide", "multi-module", "across multiple", "full ", "migration", "infra",
		"architecture", "auth system", "security", "task pack", "taskpack", "worker nodes",
		"split tasks", "refactor the whole", "authentication", "auth",
	}
	if containsAny(p, largeSignals) {
		return "large"
	}

	mediumSignals := []string{
		"feature", "workflow", "classification", "validation", "retry handling", "one executor",
		"decision_gate", "planning",
	}
	if containsAny(p, mediumSignals) {
		return "medium"
	}

	return "small"
}

func EvaluatePrehook(prompt string) PrehookResult {
	level := ClassifyTaskLevel(prompt)
	if level == "none" {
		return PrehookResult{
			IsTask: false,
			Level:  "none",
			Route:  "answer_directly",
			Reason: "The prompt does not request repository implementation changes.",
		}
	}

	switch level {
	case "small":
		return PrehookResult{
			IsTask: true,
			Level:  "small",
			Route:  "direct_execute",
			Reason: "The prompt asks for a small, localized implementation change.",
		}
	case "medium":
		return PrehookResult{
			IsTask: true,
			Level:  "medium",
			Route:  "plan_template",
			Reason: "The prompt asks for a feature-level implementation affecting one workflow.",
		}
	default:
		return PrehookResult{
			IsTask: true,
			Level:  "large",
			Route:  "task_pack_creator",
			Reason: "The prompt asks for a large multi-module or staged implementation effort.",
		}
	}
}

func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func containsAny(s string, tokens []string) bool {
	for _, t := range tokens {
		if strings.Contains(s, t) {
			return true
		}
	}
	return false
}
