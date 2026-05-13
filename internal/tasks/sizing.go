package tasks

import "strings"

type TaskSize string

const (
	SizeSmall  TaskSize = "small"
	SizeMedium TaskSize = "medium"
	SizeLarge  TaskSize = "large"
)

const (
	ComfortableChildBodyCharsMin = 1200
	ComfortableChildBodyCharsMax = 2000
	RiskyChildBodyChars          = 4000
)

func EstimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	return (len(text) + 3) / 4
}

func ClassifyTaskBody(content string) TaskSize {
	lower := strings.ToLower(content)
	largeMarkers := []string{
		"multi-file",
		"multiple files",
		"refactor",
		"architecture",
		"pipeline",
		"full implementation",
		"multiple phases",
		"task runner",
		"split",
		"commit flow",
		"many requirements",
	}

	for _, marker := range largeMarkers {
		if strings.Contains(lower, marker) {
			return SizeLarge
		}
	}

	if len(content) < 1200 {
		return SizeSmall
	}
	if len(content) < 4000 {
		return SizeMedium
	}
	return SizeLarge
}

func StepBudgetForSize(size TaskSize) int {
	switch size {
	case SizeSmall:
		return 4
	case SizeMedium:
		return 8
	default:
		return 10
	}
}

func ChildTaskBodyWarning(body string) string {
	length := len(strings.TrimSpace(body))
	switch {
	case length == 0:
		return ""
	case length > RiskyChildBodyChars:
		return "child task body is risky (>4000 chars); split further or narrow the scope"
	case length > ComfortableChildBodyCharsMax:
		return "child task body is larger than the comfortable 1200-2000 char range"
	default:
		return ""
	}
}
