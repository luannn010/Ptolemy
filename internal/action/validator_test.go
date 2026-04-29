package action

import (
	"errors"
	"testing"
)

func TestValidateSingleJSONAction(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		want      string
		wantErr   error
		invalidOK bool
	}{
		{
			name: "valid single object",
			raw:  `{"action":"read_file","path":"README.md"}`,
			want: "read_file",
		},
		{
			name:    "multiple objects",
			raw:     "{\"action\":\"read_file\"}\n{\"action\":\"run_command\"}",
			wantErr: ErrMultipleObjects,
		},
		{
			name:    "array rejected",
			raw:     `[{"action":"read_file"}]`,
			wantErr: ErrJSONArray,
		},
		{
			name:      "invalid json",
			raw:       `{"action":"read_file",}`,
			invalidOK: true,
		},
		{
			name:    "empty response",
			raw:     "   ",
			wantErr: ErrEmptyResponse,
		},
		{
			name: "valid create task batch",
			raw: `{
				"action":"create_task_batch",
				"tasks":[
					{"type":"read_file","path":"docs/PLAN.md"},
					{"type":"run_command","command":"go test ./..."}
				]
			}`,
			want: "create_task_batch",
		},
		{
			name: "type without action",
			raw:  `{"type":"read_file","path":"README.md"}`,
			want: "read_file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateSingleJSONAction(tt.raw)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ValidateSingleJSONAction() error = %v, want %v", err, tt.wantErr)
				}
				return
			}

			if tt.invalidOK {
				if err == nil {
					t.Fatal("ValidateSingleJSONAction() error = nil, want invalid JSON error")
				}
				return
			}

			if err != nil {
				t.Fatalf("ValidateSingleJSONAction() error = %v", err)
			}
			if got.Action != tt.want {
				t.Fatalf("ValidateSingleJSONAction() action = %q, want %q", got.Action, tt.want)
			}
		})
	}
}

func TestValidateSingleJSONActionRejectsInvalidTaskBatch(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr error
	}{
		{
			name:    "empty batch",
			raw:     `{"action":"create_task_batch","tasks":[]}`,
			wantErr: ErrEmptyTaskBatch,
		},
		{
			name: "nested batch",
			raw: `{
				"action":"create_task_batch",
				"tasks":[{"type":"create_task_batch","tasks":[{"type":"read_file"}]}]
			}`,
			wantErr: ErrNestedTaskBatch,
		},
		{
			name: "missing child type",
			raw: `{
				"action":"create_task_batch",
				"tasks":[{"path":"README.md"}]
			}`,
			wantErr: ErrMissingActionType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateSingleJSONAction(tt.raw)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidateSingleJSONAction() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
