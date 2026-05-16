package action

import (
	"encoding/json"
	"strings"
)

type ActionMetadata struct {
	Title         string `json:"title,omitempty"`
	Purpose       string `json:"purpose,omitempty"`
	ReasoningStep string `json:"reasoning_step,omitempty"`
	Target        string `json:"target,omitempty"`
}

func (m ActionMetadata) IsZero() bool {
	return strings.TrimSpace(m.Title) == "" &&
		strings.TrimSpace(m.Purpose) == "" &&
		strings.TrimSpace(m.ReasoningStep) == "" &&
		strings.TrimSpace(m.Target) == ""
}

func ParseMetadata(raw string) ActionMetadata {
	if strings.TrimSpace(raw) == "" {
		return ActionMetadata{}
	}

	var meta ActionMetadata
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		return ActionMetadata{}
	}

	return meta
}

func MergeMetadata(raw string, meta ActionMetadata) string {
	if strings.TrimSpace(raw) == "" && meta.IsZero() {
		return "{}"
	}

	payload := map[string]any{}
	if strings.TrimSpace(raw) != "" {
		_ = json.Unmarshal([]byte(raw), &payload)
	}

	if strings.TrimSpace(meta.Title) != "" {
		payload["title"] = meta.Title
	}
	if strings.TrimSpace(meta.Purpose) != "" {
		payload["purpose"] = meta.Purpose
	}
	if strings.TrimSpace(meta.ReasoningStep) != "" {
		payload["reasoning_step"] = meta.ReasoningStep
	}
	if strings.TrimSpace(meta.Target) != "" {
		payload["target"] = meta.Target
	}

	if len(payload) == 0 {
		return "{}"
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}

	return string(data)
}

func DisplayType(actionType string) string {
	switch strings.TrimSpace(actionType) {
	case "read_file":
		return "file.read"
	case "write_file":
		return "file.write"
	case "replace_block":
		return "file.replace_block"
	case "insert_after":
		return "file.insert_after"
	case "run_command":
		return "command.exec"
	default:
		return strings.TrimSpace(actionType)
	}
}

func FormatDisplay(prefix string, actionType string, meta ActionMetadata) string {
	header := strings.TrimSpace(prefix)
	if header == "" {
		header = "Used Ptolemy"
	}
	if strings.TrimSpace(meta.Title) != "" {
		header += " — " + strings.TrimSpace(meta.Title)
	}

	lines := []string{header}
	if strings.TrimSpace(meta.Purpose) != "" {
		lines = append(lines, "Purpose: "+strings.TrimSpace(meta.Purpose))
	}
	if strings.TrimSpace(meta.ReasoningStep) != "" {
		lines = append(lines, "Reasoning step: "+strings.TrimSpace(meta.ReasoningStep))
	}
	if strings.TrimSpace(actionType) != "" {
		lines = append(lines, "Action: "+DisplayType(actionType))
	}
	if strings.TrimSpace(meta.Target) != "" {
		lines = append(lines, "Target: "+strings.TrimSpace(meta.Target))
	}

	return strings.Join(lines, "\n")
}
