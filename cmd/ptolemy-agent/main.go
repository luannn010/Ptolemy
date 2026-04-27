package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/luannn010/ptolemy/internal/brain"
	"github.com/luannn010/ptolemy/internal/inspect"
	"github.com/luannn010/ptolemy/internal/worker"
)

const maxBrainPreviewChars = 1200
const artifactDir = ".state/agent-artifacts"

type BrainAction struct {
	Action  string `json:"action"`
	Command string `json:"command,omitempty"`
	Path    string `json:"path,omitempty"`
	Content string `json:"content,omitempty"`
	Old     string `json:"old,omitempty"`
	New     string `json:"new,omitempty"`
	Marker  string `json:"marker,omitempty"`
	Reason  string `json:"reason"`
}

type ActionResult struct {
	Display string
	Brain   string
}

func main() {
	taskFile := flag.String("task-file", "", "markdown task file to execute")
	maxSteps := flag.Int("max-steps", 8, "max agent steps")
	allowScripts := flag.Bool("allow-scripts", false, "allow script creation/execution for approved bootstrap tasks")
	flag.Parse()

	task := strings.Join(flag.Args(), " ")

	if *taskFile != "" {
		data, err := os.ReadFile(*taskFile)
		if err != nil {
			fmt.Printf("failed to read task file: %v\n", err)
			os.Exit(1)
		}
		task = string(data)
	}

	if strings.TrimSpace(task) == "" {
		fmt.Println("usage: ptolemy-agent [--task-file path] [--max-steps 8] <task>")
		os.Exit(1)
	}

	workspace, err := os.Getwd()
	if err != nil {
		fmt.Printf("failed to get workspace: %v\n", err)
		os.Exit(1)
	}

	snapshot := inspect.InspectWorkspace(workspace)

	snapshotJSON, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		fmt.Printf("failed to marshal workspace snapshot: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	brainClient := brain.NewClient("http://127.0.0.1:8088", "gemma-4-e2b")
	workerClient := worker.NewClient("http://127.0.0.1:8080")

	session, err := workerClient.CreateSession(ctx, worker.CreateSessionRequest{
		Name:        "ptolemy-agent",
		Workspace:   workspace,
		Description: "created by ptolemy-agent",
	})
	if err != nil {
		fmt.Printf("failed to create worker session: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("session: %s\n", session.ID)

	observations := []string{}

	for step := 1; step <= *maxSteps; step++ {
		fmt.Printf("\n--- step %d/%d ---\n", step, *maxSteps)

		reply, err := brainClient.Chat(ctx, []brain.Message{
			{
				Role: "system",
				Content: `You are Ptolemy local executor brain.

You must respond in JSON ONLY.

Format:
{
  "action": "run_command | read_file | write_file | replace_block | insert_after | explain | ask_approval",
  "command": "<shell command>",
  "path": "<relative file path>",
  "content": "<file content for write_file>",
  "old": "<exact old text for replace_block>",
  "new": "<exact new text for replace_block>",
  "marker": "<text marker for insert_after>",
  "reason": "<short explanation>"
}

Rules:
- Be concise.
- Do not execute anything yourself.
- Use read_file before editing a file.
- For exact text replacement, use replace_block with old and new fields.
- Use insert_after when you need to add a small function, rule, or block after a known marker.
- Prefer insert_after over replace_block when exact old text matching is fragile.
- insert_after must only insert one small block at a time.
- For editing existing code, prefer replace_block instead of write_file.
- Use write_file only for new small files or full overwrite when explicitly required.
- Never rewrite full source files unless explicitly required.
- Keep write_file content under 4000 characters.
- Use run_command for tests, formatting, and validation.
- If dangerous, action = ask_approval.
- If task is complete, action = explain.
- You must return EXACTLY ONE JSON object per response.
- Never return multiple JSON objects.
- Never chain multiple actions in one response.
- If multiple changes are needed, do them step-by-step across multiple iterations.
- For insert_after, you MUST provide both marker and content fields.
- The marker field must be the exact text to search for.
- NEVER return markdown.
- NEVER return plain text.
- NEVER include reasoning_content.
`,
			},
			{
				Role: "user",
				Content: fmt.Sprintf(`Workspace snapshot:
%s

Original task:
%s

Observations so far:
%s`, string(snapshotJSON), task, strings.Join(observations, "\n\n")),
			},
		})
		if err != nil {
			fmt.Printf("brain error: %v\n", err)
			os.Exit(1)
		}

		action, err := parseBrainAction(reply)
		if err != nil {
			summary := summarizeError(err, reply)
			artifactPath := saveArtifact(step, "brain-parse-error", reply)

			fmt.Printf(
				"%s\nartifact: %s\n",
				summary,
				artifactPath,
			)

			observations = append(observations, fmt.Sprintf(
				"Previous brain response was invalid JSON. Summary: %s. Artifact: %s. Return exactly ONE JSON object only. Do not return multiple JSON objects. Do not chain actions. Do one action now and continue later.",
				summary,
				artifactPath,
			))

			continue
		}

		fmt.Printf("brain action: %s\n", action.Action)
		fmt.Printf("reason: %s\n", action.Reason)

		result := executeAction(ctx, workerClient, session.ID, workspace, step, action, *allowScripts)

		fmt.Println(result.Display)
		observations = append(observations, result.Brain)

		if action.Action == "explain" || action.Action == "ask_approval" {
			return
		}
	}

	fmt.Println("max steps reached")
	os.Exit(1)
}

func executeAction(
	ctx context.Context,
	workerClient *worker.Client,
	sessionID string,
	workspace string,
	step int,
	action *BrainAction,
	allowScripts bool,
) ActionResult {
	switch action.Action {
	case "explain":
		msg := "DONE: " + action.Reason
		return ActionResult{Display: msg, Brain: msg}

	case "ask_approval":
		msg := fmt.Sprintf("APPROVAL REQUIRED: %s command=%s path=%s", action.Reason, action.Command, action.Path)
		return ActionResult{Display: msg, Brain: msg}

	case "run_command":
		if strings.TrimSpace(action.Command) == "" {
			return both("ERROR: empty command")
		}

		if commandRunsScript(action.Command) && !allowScripts {
			return both(fmt.Sprintf("APPROVAL REQUIRED: running script commands requires explicit permission command=%s", action.Command))
		}

		result, err := workerClient.RunCommand(ctx, sessionID, worker.RunCommandRequest{
			Command: action.Command,
			CWD:     workspace,
			Timeout: 120,
		})
		if err != nil {
			return both(fmt.Sprintf("ERROR running command: %v", err))
		}

		combinedOutput := result.Output
		if result.ErrorOutput != "" {
			combinedOutput += "\n" + result.ErrorOutput
		}

		artifactPath := saveArtifact(step, "command-output", combinedOutput)

		display := fmt.Sprintf(
			"COMMAND RESULT\ncommand: %s\nexit_code: %d\nartifact: %s",
			result.Command,
			result.ExitCode,
			artifactPath,
		)

		brain := fmt.Sprintf(
			"COMMAND RESULT\ncommand: %s\nexit_code: %d\nartifact: %s\noutput_preview:\n%s",
			result.Command,
			result.ExitCode,
			artifactPath,
			previewText(combinedOutput, maxBrainPreviewChars),
		)

		return ActionResult{Display: display, Brain: brain}

	case "read_file":
		if strings.TrimSpace(action.Path) == "" {
			return both("ERROR: empty path")
		}

		result, err := workerClient.ReadFile(ctx, worker.ReadFileRequest{
			SessionID: sessionID,
			Path:      action.Path,
		})
		if err != nil {
			return both(fmt.Sprintf("ERROR reading file: %v", err))
		}

		artifactPath := saveArtifact(step, "read-"+result.Path, result.Content)

		display := fmt.Sprintf(
			"FILE READ OK\npath: %s\nbytes: %d\nartifact: %s",
			result.Path,
			len(result.Content),
			artifactPath,
		)

		brain := fmt.Sprintf(
			"FILE READ OK\npath: %s\nbytes: %d\nartifact: %s\npreview_for_reasoning:\n%s",
			result.Path,
			len(result.Content),
			artifactPath,
			previewText(result.Content, maxBrainPreviewChars),
		)

		return ActionResult{Display: display, Brain: brain}

	case "write_file":
		if strings.TrimSpace(action.Path) == "" {
			return both("ERROR: empty path")
		}

		if isScriptPath(action.Path) && !allowScripts {
			return both(fmt.Sprintf("APPROVAL REQUIRED: creating script file requires explicit permission path=%s", action.Path))
		}

		if len(action.Content) > 4000 {
			return both(fmt.Sprintf(
				"ERROR: write_file content too large. path=%s bytes=%d. Use replace_block instead.",
				action.Path,
				len(action.Content),
			))
		}

		artifactPath := saveArtifact(step, "write-"+action.Path, action.Content)

		result, err := workerClient.WriteFile(ctx, worker.WriteFileRequest{
			SessionID: sessionID,
			Path:      action.Path,
			Content:   action.Content,
		})
		if err != nil {
			return both(fmt.Sprintf("ERROR writing file: %v", err))
		}

		msg := fmt.Sprintf(
			"FILE WRITE OK\npath: %s\nbytes: %d\nartifact: %s",
			result.Path,
			len(action.Content),
			artifactPath,
		)

		return ActionResult{Display: msg, Brain: msg}

	case "insert_after":
		if strings.TrimSpace(action.Path) == "" {
			return both("ERROR: empty path")
		}

		if strings.TrimSpace(action.Marker) == "" {
			return both("ERROR: insert_after marker is empty")
		}

		if strings.TrimSpace(action.Content) == "" {
			return both("ERROR: insert_after content is empty")
		}

		if !isSafeReplacePath(action.Path) {
			return both(fmt.Sprintf("ERROR: insert_after path is not allowed: %s", action.Path))
		}

		file, err := workerClient.ReadFile(ctx, worker.ReadFileRequest{
			SessionID: sessionID,
			Path:      action.Path,
		})
		if err != nil {
			return both(fmt.Sprintf("ERROR reading file for insert_after: %v", err))
		}

		idx := strings.Index(file.Content, action.Marker)
		if idx == -1 {
			artifactPath := saveArtifact(step, "insert-after-marker-not-found-"+action.Path, file.Content)
			return both(fmt.Sprintf(
				"ERROR: insert_after marker not found\npath: %s\nartifact: %s",
				action.Path,
				artifactPath,
			))
		}

		insertAt := idx + len(action.Marker)

		afterMarker := file.Content[insertAt:]
		if strings.Contains(afterMarker, action.Content) {
			msg := fmt.Sprintf(
				"INSERT AFTER SKIPPED\npath: %s\nreason: content already exists after marker",
				action.Path,
			)
			return ActionResult{Display: msg, Brain: msg}
		}

		insertText := action.Content
		if !strings.HasPrefix(insertText, "\n") {
			insertText = "\n" + insertText
		}

		updated := file.Content[:insertAt] + insertText + file.Content[insertAt:]
		artifactPath := saveArtifact(step, "insert-after-"+action.Path, updated)

		result, err := workerClient.WriteFile(ctx, worker.WriteFileRequest{
			SessionID: sessionID,
			Path:      action.Path,
			Content:   updated,
		})
		if err != nil {
			return both(fmt.Sprintf("ERROR writing insert_after result: %v", err))
		}

		msg := fmt.Sprintf(
			"INSERT AFTER OK\npath: %s\nbytes: %d\nartifact: %s",
			result.Path,
			len(updated),
			artifactPath,
		)

		return ActionResult{Display: msg, Brain: msg}

	case "replace_block":
		if strings.TrimSpace(action.Path) == "" {
			return both("ERROR: empty path")
		}

		if isScriptPath(action.Path) && !allowScripts {
			return both(fmt.Sprintf("APPROVAL REQUIRED: editing script file requires explicit permission path=%s", action.Path))
		}

		if strings.TrimSpace(action.Old) == "" {
			return both("ERROR: replace_block old text is empty")
		}

		if !isSafeReplacePath(action.Path) {
			return both(fmt.Sprintf("ERROR: replace_block path is not allowed: %s", action.Path))
		}

		file, err := workerClient.ReadFile(ctx, worker.ReadFileRequest{
			SessionID: sessionID,
			Path:      action.Path,
		})
		if err != nil {
			return both(fmt.Sprintf("ERROR reading file for replace_block: %v", err))
		}

		if !strings.Contains(file.Content, action.Old) {
			artifactPath := saveArtifact(step, "replace-old-not-found-"+action.Path, file.Content)
			return both(fmt.Sprintf(
				"ERROR: replace_block old text not found\npath: %s\nartifact: %s",
				action.Path,
				artifactPath,
			))
		}

		updated := strings.Replace(file.Content, action.Old, action.New, 1)
		artifactPath := saveArtifact(step, "replace-"+action.Path, updated)

		result, err := workerClient.WriteFile(ctx, worker.WriteFileRequest{
			SessionID: sessionID,
			Path:      action.Path,
			Content:   updated,
		})
		if err != nil {
			return both(fmt.Sprintf("ERROR writing replace_block result: %v", err))
		}

		msg := fmt.Sprintf(
			"REPLACE BLOCK OK\npath: %s\nbytes: %d\nartifact: %s",
			result.Path,
			len(updated),
			artifactPath,
		)

		return ActionResult{Display: msg, Brain: msg}

	default:
		return both(fmt.Sprintf("ERROR: unknown action %s", action.Action))
	}
}

func both(message string) ActionResult {
	return ActionResult{
		Display: message,
		Brain:   message,
	}
}

func isSafeReplacePath(p string) bool {
	if p == "" {
		return false
	}

	cleaned := strings.TrimSpace(p)

	if strings.HasPrefix(cleaned, "/etc") ||
		strings.HasPrefix(cleaned, "/usr") ||
		strings.HasPrefix(cleaned, "/var") ||
		strings.HasPrefix(cleaned, "/home") {
		return false
	}

	if strings.Contains(cleaned, "..") {
		return false
	}

	return strings.HasSuffix(cleaned, ".go") ||
		strings.HasSuffix(cleaned, ".md") ||
		strings.HasSuffix(cleaned, ".txt")
}

func hasExplicitScriptPermission(reason string) bool {
	lower := strings.ToLower(reason)
	return strings.Contains(lower, "explicit permission") ||
		strings.Contains(lower, "user approved script") ||
		strings.Contains(lower, "permission granted")
}

func isScriptPath(p string) bool {
	lower := strings.ToLower(strings.TrimSpace(p))
	return strings.HasSuffix(lower, ".sh") ||
		strings.HasSuffix(lower, ".py") ||
		strings.HasSuffix(lower, ".js") ||
		strings.HasSuffix(lower, ".ts") ||
		strings.HasSuffix(lower, ".rb") ||
		strings.HasSuffix(lower, ".pl")
}

func commandRunsScript(command string) bool {
	lower := strings.ToLower(command)
	return strings.Contains(lower, "python ") ||
		strings.Contains(lower, "python3 ") ||
		strings.Contains(lower, "bash ") ||
		strings.Contains(lower, "sh ") ||
		strings.Contains(lower, "node ") ||
		strings.Contains(lower, "ts-node ") ||
		strings.Contains(lower, "ruby ") ||
		strings.Contains(lower, "perl ")
}

func saveArtifact(step int, name string, content string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}

	_ = os.MkdirAll(artifactDir, 0755)

	safeName := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		":", "-",
		" ", "-",
	).Replace(name)

	path := fmt.Sprintf("%s/step-%03d-%s.txt", artifactDir, step, safeName)

	_ = os.WriteFile(path, []byte(content), 0644)

	return path
}

func previewText(text string, maxChars int) string {
	if len(text) <= maxChars {
		return text
	}

	return text[:maxChars] + "\n...[truncated]"
}

func parseBrainAction(reply string) (*BrainAction, error) {
	cleaned := strings.TrimSpace(reply)

	if cleaned == "" {
		return nil, fmt.Errorf("empty brain reply")
	}

	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	start := strings.Index(cleaned, "{")
	end := strings.LastIndex(cleaned, "}")

	if start == -1 || end == -1 || end <= start {
		return nil, fmt.Errorf("no JSON object found in reply: %q", reply)
	}

	cleaned = cleaned[start : end+1]

	var action BrainAction
	if err := json.Unmarshal([]byte(cleaned), &action); err != nil {
		return nil, fmt.Errorf("invalid JSON %q: %w", cleaned, err)
	}

	return &action, nil
}
func summarizeError(err error, raw string) string {
	msg := err.Error()

	// multiple JSON objects (most common failure)
	if strings.Count(raw, "{") > 1 {
		return "ERROR: multiple JSON objects returned (agent must return ONE action)"
	}

	// JSON parse errors
	if strings.Contains(msg, "invalid character") {
		return "ERROR: invalid JSON (likely bad escaping or formatting)"
	}

	if strings.Contains(msg, "unexpected end of JSON") {
		return "ERROR: incomplete JSON response"
	}

	if strings.Contains(msg, "no JSON object found") {
		return "ERROR: no JSON object in response"
	}

	// fallback
	return "ERROR: " + msg
}
