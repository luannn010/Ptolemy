package action

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

var (
	ErrEmptyResponse     = errors.New("empty brain reply")
	ErrJSONArray         = errors.New("top-level JSON arrays are not allowed")
	ErrMultipleObjects   = errors.New("multiple JSON objects returned")
	ErrMissingActionType = errors.New("missing action or type")
	ErrEmptyTaskBatch    = errors.New("create_task_batch requires at least one task")
	ErrNestedTaskBatch   = errors.New("nested create_task_batch tasks are not allowed")
)

type ActionEnvelope struct {
	Action  string      `json:"action,omitempty"`
	Type    string      `json:"type,omitempty"`
	Command string      `json:"command,omitempty"`
	Path    string      `json:"path,omitempty"`
	Content string      `json:"content,omitempty"`
	Old     string      `json:"old,omitempty"`
	New     string      `json:"new,omitempty"`
	Marker  string      `json:"marker,omitempty"`
	Reason  string      `json:"reason,omitempty"`
	Tasks   []BatchTask `json:"tasks,omitempty"`
}

type TaskBatch struct {
	Tasks []BatchTask `json:"tasks"`
}

type BatchTask struct {
	Type    string `json:"type,omitempty"`
	Action  string `json:"action,omitempty"`
	Command string `json:"command,omitempty"`
	Path    string `json:"path,omitempty"`
	Content string `json:"content,omitempty"`
	Old     string `json:"old,omitempty"`
	New     string `json:"new,omitempty"`
	Marker  string `json:"marker,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

func ValidateSingleJSONAction(raw string) (*ActionEnvelope, error) {
	cleaned := trimJSONCodeFence(raw)
	if cleaned == "" {
		return nil, ErrEmptyResponse
	}

	trimmed := strings.TrimSpace(cleaned)
	if strings.HasPrefix(trimmed, "[") {
		return nil, ErrJSONArray
	}

	dec := json.NewDecoder(strings.NewReader(trimmed))
	dec.UseNumber()

	var rawValue json.RawMessage
	if err := dec.Decode(&rawValue); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	if strings.TrimSpace(string(rawValue)) == "" {
		return nil, ErrEmptyResponse
	}

	if hasTrailingValue(dec) {
		return nil, ErrMultipleObjects
	}

	var envelope ActionEnvelope
	if err := json.Unmarshal(rawValue, &envelope); err != nil {
		return nil, fmt.Errorf("invalid action object: %w", err)
	}

	envelope.Action = normalizeKind(envelope.Action, envelope.Type)
	if envelope.Action == "" {
		return nil, ErrMissingActionType
	}

	if envelope.Action == "create_task_batch" {
		if err := validateTaskBatch(&envelope); err != nil {
			return nil, err
		}
	}

	return &envelope, nil
}

func (t BatchTask) NormalizedType() string {
	return normalizeKind(t.Action, t.Type)
}

func trimJSONCodeFence(raw string) string {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	return strings.TrimSpace(cleaned)
}

func hasTrailingValue(dec *json.Decoder) bool {
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != nil {
		return !errors.Is(err, io.EOF)
	}
	return len(bytes.TrimSpace(extra)) > 0
}

func normalizeKind(primary string, fallback string) string {
	primary = strings.TrimSpace(primary)
	if primary != "" {
		return primary
	}
	return strings.TrimSpace(fallback)
}

func validateTaskBatch(envelope *ActionEnvelope) error {
	if len(envelope.Tasks) == 0 {
		return ErrEmptyTaskBatch
	}

	for i := range envelope.Tasks {
		normalized := envelope.Tasks[i].NormalizedType()
		if normalized == "" {
			return fmt.Errorf("task %d: %w", i, ErrMissingActionType)
		}
		if normalized == "create_task_batch" {
			return fmt.Errorf("task %d: %w", i, ErrNestedTaskBatch)
		}
		envelope.Tasks[i].Type = normalized
		envelope.Tasks[i].Action = ""
	}

	return nil
}
