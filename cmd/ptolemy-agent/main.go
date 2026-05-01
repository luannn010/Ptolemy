package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	actionpkg "github.com/luannn010/ptolemy/internal/action"
	"github.com/luannn010/ptolemy/internal/brain"
	"github.com/luannn010/ptolemy/internal/config"
	"github.com/luannn010/ptolemy/internal/inspect"
	logspkg "github.com/luannn010/ptolemy/internal/logs"
	storepkg "github.com/luannn010/ptolemy/internal/store"
	taskspkg "github.com/luannn010/ptolemy/internal/tasks"
	"github.com/luannn010/ptolemy/internal/worker"
)

const maxBrainPreviewChars = 1200
const artifactDir = ".state/agent-artifacts"

var errNoJSONObject = errors.New("no JSON object found")

type ActionResult struct {
	Display string
	Brain   string
}

type workerClient interface {
	CreateSession(ctx context.Context, reqBody worker.CreateSessionRequest) (*worker.Session, error)
	RunCommand(ctx context.Context, sessionID string, reqBody worker.RunCommandRequest) (*worker.CommandResult, error)
	ReadFile(ctx context.Context, reqBody worker.ReadFileRequest) (*worker.ReadFileResponse, error)
	WriteFile(ctx context.Context, reqBody worker.WriteFileRequest) (*worker.WriteFileResponse, error)
}

type agentRuntime struct {
	workerClient workerClient
	actionStore  *actionpkg.Store
	logStore     *logspkg.Store
	splitter     actionpkg.TaskSplitter
	taskPackRoot string
}

type progressGuard struct {
	lastSignature string
	repeatCount   int
}

func main() {
	taskFile := flag.String("task-file", "", "markdown task file to execute")
	maxSteps := flag.Int("max-steps", 8, "max agent steps")
	allowScripts := flag.Bool("allow-scripts", false, "allow script creation/execution for approved bootstrap tasks")
	flag.Parse()

	task := strings.Join(flag.Args(), " ")
	taskPackRoot := ""

	if *taskFile != "" {
		data, err := os.ReadFile(*taskFile)
		if err != nil {
			fmt.Printf("failed to read task file: %v\n", err)
			os.Exit(1)
		}
		task, taskPackRoot, err = loadTaskInput(*taskFile, data)
		if err != nil {
			fmt.Printf("failed to prepare task file: %v\n", err)
			os.Exit(1)
		}
	}

	if strings.TrimSpace(task) == "" {
		fmt.Println("usage: ptolemy-agent [--task-file path] [--max-steps 8] <task>")
		os.Exit(1)
	}

	taskName := deriveTaskName(*taskFile, task)

	workspace, err := os.Getwd()
	if err != nil {
		fmt.Printf("failed to get workspace: %v\n", err)
		os.Exit(1)
	}
	if strings.TrimSpace(taskPackRoot) != "" {
		if rel, relErr := filepath.Rel(workspace, taskPackRoot); relErr == nil {
			cleanedRel := filepath.ToSlash(filepath.Clean(rel))
			if cleanedRel == "." {
				taskPackRoot = ""
			} else if cleanedRel != ".." && !strings.HasPrefix(cleanedRel, "../") {
				taskPackRoot = cleanedRel
			}
		}
	}

	snapshot := inspect.InspectWorkspace(workspace)

	snapshotJSON, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		fmt.Printf("failed to marshal workspace snapshot: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("failed to load config: %v\n", err)
		os.Exit(1)
	}

	baseStore, err := storepkg.Open(cfg.DBPath)
	if err != nil {
		fmt.Printf("failed to open store: %v\n", err)
		os.Exit(1)
	}
	defer baseStore.Close()

	if err := storepkg.RunMigrations(ctx, baseStore.SQLDB()); err != nil {
		fmt.Printf("failed to run migrations: %v\n", err)
		os.Exit(1)
	}

	brainClient := brain.NewClient("http://127.0.0.1:8088", "gemma-4-e2b")
	runtime := &agentRuntime{
		workerClient: worker.NewClient("http://127.0.0.1:8080"),
		actionStore:  actionpkg.NewStore(baseStore.SQLDB()),
		logStore:     logspkg.NewStore(baseStore.SQLDB()),
		splitter:     actionpkg.PlaceholderTaskSplitter{},
		taskPackRoot: taskPackRoot,
	}

	session, err := runtime.workerClient.CreateSession(ctx, worker.CreateSessionRequest{
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
	guard := &progressGuard{}

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
- If you think multiple actions are needed, return only the FIRST action now and wait for the next turn.
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

		action, result, ok := processBrainReply(ctx, runtime, session.ID, workspace, taskName, step, reply, *allowScripts, guard)
		if !ok {
			fmt.Println(result.Display)
			observations = append(observations, result.Brain)
			continue
		}

		fmt.Printf("brain action: %s\n", action.Action)
		fmt.Printf("reason: %s\n", action.Reason)

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
	runtime *agentRuntime,
	sessionID string,
	workspace string,
	taskName string,
	step int,
	action *actionpkg.ActionEnvelope,
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

		result, err := runtime.workerClient.RunCommand(ctx, sessionID, worker.RunCommandRequest{
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

		artifactPath := saveArtifact(taskName, step, "command-output", combinedOutput)

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

		path := action.Path
		if remappedPath, ok := resolvePackReferencePath(runtime.taskPackRoot, action.Path); ok {
			path = remappedPath
		}

		result, err := runtime.workerClient.ReadFile(ctx, worker.ReadFileRequest{
			SessionID: sessionID,
			Path:      path,
		})
		if err != nil {
			return both(fmt.Sprintf("ERROR reading file: %v", err))
		}

		artifactPath := saveArtifact(taskName, step, "read-"+result.Path, result.Content)

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

		artifactPath := saveArtifact(taskName, step, "write-"+action.Path, action.Content)

		result, err := runtime.workerClient.WriteFile(ctx, worker.WriteFileRequest{
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

		file, err := runtime.workerClient.ReadFile(ctx, worker.ReadFileRequest{
			SessionID: sessionID,
			Path:      action.Path,
		})
		if err != nil {
			if isMissingFileError(err) {
				return both(fmt.Sprintf(
					"ERROR: insert_after target file does not exist: %s. Use write_file to create a new file inside allowed_files, or read an existing implementation file first.",
					action.Path,
				))
			}
			return both(fmt.Sprintf("ERROR reading file for insert_after: %v", err))
		}

		idx := strings.Index(file.Content, action.Marker)
		if idx == -1 {
			artifactPath := saveArtifact(taskName, step, "insert-after-marker-not-found-"+action.Path, file.Content)
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
		artifactPath := saveArtifact(taskName, step, "insert-after-"+action.Path, updated)

		result, err := runtime.workerClient.WriteFile(ctx, worker.WriteFileRequest{
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

		file, err := runtime.workerClient.ReadFile(ctx, worker.ReadFileRequest{
			SessionID: sessionID,
			Path:      action.Path,
		})
		if err != nil {
			if isMissingFileError(err) {
				return both(fmt.Sprintf(
					"ERROR: replace_block target file does not exist: %s. Use write_file to create a new file inside allowed_files, or read an existing implementation file first.",
					action.Path,
				))
			}
			return both(fmt.Sprintf("ERROR reading file for replace_block: %v", err))
		}

		if !strings.Contains(file.Content, action.Old) {
			artifactPath := saveArtifact(taskName, step, "replace-old-not-found-"+action.Path, file.Content)
			return both(fmt.Sprintf(
				"ERROR: replace_block old text not found\npath: %s\nartifact: %s",
				action.Path,
				artifactPath,
			))
		}

		updated := strings.Replace(file.Content, action.Old, action.New, 1)
		artifactPath := saveArtifact(taskName, step, "replace-"+action.Path, updated)

		result, err := runtime.workerClient.WriteFile(ctx, worker.WriteFileRequest{
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

	case "create_task_batch":
		return queueTaskBatch(ctx, runtime, sessionID, action)

	default:
		return both(fmt.Sprintf("ERROR: unknown action %s", action.Action))
	}
}

func processBrainReply(
	ctx context.Context,
	runtime *agentRuntime,
	sessionID string,
	workspace string,
	taskName string,
	step int,
	reply string,
	allowScripts bool,
	guard *progressGuard,
) (*actionpkg.ActionEnvelope, ActionResult, bool) {
	action, warning, err := parseFirstValidJSONAction(reply)
	if err != nil {
		return nil, handleInvalidModelOutput(ctx, runtime, sessionID, taskName, step, reply, err), false
	}
	if warning != "" {
		_, _ = runtime.logStore.Create(ctx, logspkg.Log{
			SessionID: sessionID,
			Level:     "warn",
			Message:   warning,
			Metadata: mustMarshalJSON(map[string]any{
				"warning": "ignored_extra_json_objects",
			}),
		})
	}

	if corrective := validateAndNormalizeAction(action); corrective != "" {
		_, _ = runtime.logStore.Create(ctx, logspkg.Log{
			SessionID: sessionID,
			Level:     "warn",
			Message:   "incomplete action ignored",
			Metadata: mustMarshalJSON(map[string]any{
				"action":     action.Action,
				"corrective": corrective,
			}),
		})

		display := "INCOMPLETE ACTION\n" + corrective + "\nNo tools were executed."
		brain := corrective
		if warning != "" {
			display += "\nwarning: " + warning
			brain += "\nwarning: " + warning
		}
		return nil, ActionResult{Display: display, Brain: brain}, false
	}

	if corrective := guard.observe(action); corrective != "" {
		_, _ = runtime.logStore.Create(ctx, logspkg.Log{
			SessionID: sessionID,
			Level:     "warn",
			Message:   "repeated action blocked",
			Metadata: mustMarshalJSON(map[string]any{
				"action":     action.Action,
				"path":       action.Path,
				"command":    action.Command,
				"corrective": corrective,
			}),
		})

		display := "PROGRESS GUARD\n" + corrective + "\nNo tools were executed."
		brain := corrective
		if warning != "" {
			display += "\nwarning: " + warning
			brain += "\nwarning: " + warning
		}
		return nil, ActionResult{Display: display, Brain: brain}, false
	}

	result := executeAction(ctx, runtime, sessionID, workspace, taskName, step, action, allowScripts)
	if warning != "" {
		result.Display = result.Display + "\nwarning: " + warning
		result.Brain = result.Brain + "\nwarning: " + warning
	}
	return action, result, true
}

func (g *progressGuard) observe(action *actionpkg.ActionEnvelope) string {
	if g == nil || action == nil {
		return ""
	}

	signature, ok := repeatedActionSignature(action)
	if !ok {
		g.lastSignature = ""
		g.repeatCount = 0
		return ""
	}

	if signature == g.lastSignature {
		g.repeatCount++
	} else {
		g.lastSignature = signature
		g.repeatCount = 1
	}

	if g.repeatCount < 2 {
		return ""
	}

	switch action.Action {
	case "read_file":
		return fmt.Sprintf(
			`Stop rereading the same file. You already requested read_file for "%s". Return exactly one JSON object that makes one concrete code edit inside allowed_files, or use explain to state the blocking scope mismatch.`,
			strings.TrimSpace(action.Path),
		)
	case "run_command":
		return fmt.Sprintf(
			`Stop rerunning the same command without a change. You already requested run_command "%s". Return exactly one JSON object that makes one concrete code edit inside allowed_files, or use explain to state what specific missing context blocks progress.`,
			strings.TrimSpace(action.Command),
		)
	default:
		return fmt.Sprintf(
			`This action repeated without making progress. Return exactly one JSON object for a different next step that makes a concrete implementation change inside allowed_files, or use explain to describe the blocker.`,
		)
	}
}

func repeatedActionSignature(action *actionpkg.ActionEnvelope) (string, bool) {
	if action == nil {
		return "", false
	}

	switch strings.TrimSpace(action.Action) {
	case "read_file":
		path := strings.TrimSpace(action.Path)
		if path == "" {
			return "", false
		}
		return "read_file|" + path, true
	case "run_command":
		command := strings.TrimSpace(action.Command)
		if command == "" {
			return "", false
		}
		return "run_command|" + command, true
	case "insert_after":
		path := strings.TrimSpace(action.Path)
		marker := strings.TrimSpace(action.Marker)
		content := strings.TrimSpace(action.Content)
		if path == "" || marker == "" || content == "" {
			return "", false
		}
		return "insert_after|" + path + "|" + marker + "|" + content, true
	case "replace_block":
		path := strings.TrimSpace(action.Path)
		old := strings.TrimSpace(action.Old)
		newValue := strings.TrimSpace(action.New)
		if path == "" || old == "" || newValue == "" {
			return "", false
		}
		return "replace_block|" + path + "|" + old + "|" + newValue, true
	case "write_file":
		path := strings.TrimSpace(action.Path)
		content := strings.TrimSpace(action.Content)
		if path == "" || content == "" {
			return "", false
		}
		return "write_file|" + path + "|" + content, true
	default:
		return "", false
	}
}

func validateAndNormalizeAction(action *actionpkg.ActionEnvelope) string {
	path := strings.TrimSpace(action.Path)
	if isMutatingAction(action.Action) && isInstructionAssetPath(path) {
		return `This action targets instruction assets and is not allowed. Do not edit task files, task-scripts, snippets, or pack metadata. Return exactly one JSON object that modifies only implementation files.`
	}

	switch strings.TrimSpace(action.Action) {
	case "replace_block":
		if strings.TrimSpace(action.Path) == "" {
			return `Your replace_block action is incomplete. Return exactly one JSON object with "action":"replace_block", "path", and either "old" (old_block) or "marker" (anchor), plus "new" (new_block).`
		}
		if strings.TrimSpace(action.Old) == "" && strings.TrimSpace(action.Marker) == "" {
			return `Your replace_block action is incomplete. Return exactly one JSON object with "action":"replace_block", "path", and either "old" (old_block) or "marker" (anchor), plus "new" (new_block).`
		}
		if strings.TrimSpace(action.New) == "" {
			return `Your replace_block action is incomplete. Return exactly one JSON object with "action":"replace_block", "path", and either "old" (old_block) or "marker" (anchor), plus "new" (new_block).`
		}
		if strings.TrimSpace(action.Old) == "" && strings.TrimSpace(action.Marker) != "" {
			action.Old = action.Marker
		}
	case "insert_after":
		if strings.TrimSpace(action.Path) == "" || strings.TrimSpace(action.Marker) == "" || strings.TrimSpace(action.Content) == "" {
			return `Your insert_after action is incomplete. Return exactly one JSON object with "action":"insert_after", "path" (target_file), "marker" (anchor), and "content" (snippet).`
		}
	case "create_file":
		if strings.TrimSpace(action.Path) == "" || strings.TrimSpace(action.Content) == "" {
			return `Your create_file action is incomplete. Return exactly one JSON object with "action":"create_file", "path" (target_file), and "content".`
		}
		action.Action = "write_file"
	case "update_file":
		if strings.TrimSpace(action.Path) == "" {
			return `Your update_file action is incomplete. Return exactly one JSON object with "action":"update_file", "path" (target_file), and either "content" or a patch pair ("old" + "new").`
		}
		if strings.TrimSpace(action.Content) != "" {
			action.Action = "write_file"
			break
		}
		if strings.TrimSpace(action.Old) != "" && strings.TrimSpace(action.New) != "" {
			action.Action = "replace_block"
			break
		}
		return `Your update_file action is incomplete. Return exactly one JSON object with "action":"update_file", "path" (target_file), and either "content" or a patch pair ("old" + "new").`
	case "write_file":
		if strings.TrimSpace(action.Path) == "" || strings.TrimSpace(action.Content) == "" {
			return `Your write_file action is incomplete. Return exactly one JSON object with "action":"write_file", "path", and "content".`
		}
	}
	return ""
}

func isMutatingAction(action string) bool {
	switch strings.TrimSpace(action) {
	case "replace_block", "insert_after", "write_file", "create_file", "update_file":
		return true
	default:
		return false
	}
}

func isInstructionAssetPath(path string) bool {
	if path == "" {
		return false
	}
	norm := strings.ToLower(filepath.ToSlash(filepath.Clean(path)))
	if strings.HasPrefix(norm, "docs/tasks/packs/") {
		if strings.Contains(norm, "/task-scripts/") || strings.Contains(norm, "/snippets/") || strings.Contains(norm, "/inbox/") {
			return true
		}
		if strings.HasSuffix(norm, "/pack_manifest.yaml") || strings.HasSuffix(norm, "/task_plan.md") || strings.HasSuffix(norm, "/readme.md") {
			return true
		}
	}
	return false
}

func parseFirstValidJSONAction(raw string) (*actionpkg.ActionEnvelope, string, error) {
	jsonText, remainder, err := extractFirstJSONObject(raw)
	if err != nil {
		return nil, "", err
	}

	action, err := actionpkg.ValidateSingleJSONAction(jsonText)
	if err != nil {
		return nil, "", err
	}

	warning := ""
	if _, _, extraErr := extractFirstJSONObject(remainder); extraErr == nil {
		warning = "ignored extra JSON objects after the first valid action"
	} else if extraErr != nil && !errors.Is(extraErr, errNoJSONObject) && !errors.Is(extraErr, actionpkg.ErrEmptyResponse) {
		return nil, "", extraErr
	}

	return action, warning, nil
}

func extractFirstJSONObject(raw string) (string, string, error) {
	cleaned := strings.TrimSpace(raw)
	if cleaned == "" {
		return "", "", actionpkg.ErrEmptyResponse
	}

	var lastErr error
	for i := 0; i < len(cleaned); i++ {
		if cleaned[i] != '{' {
			continue
		}

		dec := json.NewDecoder(strings.NewReader(cleaned[i:]))
		dec.UseNumber()

		var rawValue json.RawMessage
		if err := dec.Decode(&rawValue); err != nil {
			lastErr = err
			continue
		}

		trimmed := bytes.TrimSpace(rawValue)
		if len(trimmed) == 0 || trimmed[0] != '{' {
			continue
		}

		offset := int(dec.InputOffset())
		return string(trimmed), cleaned[i+offset:], nil
	}

	if lastErr != nil {
		return "", "", fmt.Errorf("invalid JSON: %w", lastErr)
	}
	return "", "", errNoJSONObject
}

func handleInvalidModelOutput(
	ctx context.Context,
	runtime *agentRuntime,
	sessionID string,
	taskName string,
	step int,
	reply string,
	err error,
) ActionResult {
	summary := summarizeError(err)
	artifactPath := saveArtifact(taskName, step, "brain-parse-error", reply)

	recoveryPayload := map[string]any{
		"status":           "invalid_model_output",
		"error":            summary,
		"safe_to_continue": false,
		"next_step":        "split_into_task_batch",
	}
	metadata := map[string]any{
		"parser_error":      err.Error(),
		"safe_to_continue":  false,
		"next_step":         "split_into_task_batch",
		"splitter_strategy": "placeholder",
		"splitter_error":    splitterMessage(runtime.splitter),
		"artifact_path":     artifactPath,
	}

	recordedAction, createErr := runtime.actionStore.Create(ctx, actionpkg.Action{
		SessionID: sessionID,
		Type:      "model.output",
		Input:     reply,
		Output:    mustMarshalJSON(recoveryPayload),
		Status:    "invalid_model_output",
		Metadata:  mustMarshalJSON(metadata),
	})
	if createErr == nil {
		_, _ = runtime.logStore.Create(ctx, logspkg.Log{
			SessionID: sessionID,
			ActionID:  recordedAction.ID,
			Level:     "warn",
			Message:   summary,
			Metadata:  mustMarshalJSON(metadata),
		})
	}

	display := fmt.Sprintf(
		"INVALID MODEL OUTPUT\nstatus: invalid_model_output\nerror: %s\nsafe_to_continue: false\nnext_step: split_into_task_batch\nartifact: %s",
		summary,
		artifactPath,
	)
	brain := fmt.Sprintf(
		"Previous brain response was invalid. Status: invalid_model_output. Error: %s. Artifact: %s. Return exactly ONE JSON object only. Do not return multiple JSON objects. Do not chain actions. Use create_task_batch if multiple tasks are needed.",
		summary,
		artifactPath,
	)

	return ActionResult{Display: display, Brain: brain}
}

func queueTaskBatch(ctx context.Context, runtime *agentRuntime, sessionID string, action *actionpkg.ActionEnvelope) ActionResult {
	parentMeta := mustMarshalJSON(map[string]any{
		"task_count": len(action.Tasks),
	})

	parent, err := runtime.actionStore.Create(ctx, actionpkg.Action{
		SessionID: sessionID,
		Type:      "create_task_batch",
		Input:     mustMarshalJSON(action),
		Status:    "queued",
		Metadata:  parentMeta,
	})
	if err != nil {
		return both(fmt.Sprintf("ERROR queueing task batch: %v", err))
	}

	for i, task := range action.Tasks {
		_, err := runtime.actionStore.Create(ctx, actionpkg.Action{
			SessionID: sessionID,
			Type:      task.NormalizedType(),
			Input:     mustMarshalJSON(task),
			Status:    "pending",
			Metadata: mustMarshalJSON(map[string]any{
				"batch_action_id": parent.ID,
				"batch_index":     i,
			}),
		})
		if err != nil {
			return both(fmt.Sprintf("ERROR queueing task batch item: %v", err))
		}
	}

	_, _ = runtime.logStore.Create(ctx, logspkg.Log{
		SessionID: sessionID,
		ActionID:  parent.ID,
		Level:     "info",
		Message:   "task batch queued",
		Metadata:  parentMeta,
	})

	msg := fmt.Sprintf("TASK BATCH QUEUED\nstatus: queued\ncount: %d", len(action.Tasks))
	return ActionResult{Display: msg, Brain: msg}
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

func saveArtifact(taskName string, step int, label string, content string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}

	_ = os.MkdirAll(artifactDir, 0755)

	path := artifactPath(taskName, step, label, time.Now())
	path = uniqueArtifactPath(path)

	_ = os.WriteFile(path, []byte(content), 0644)

	return path
}

func saveArtifactAt(now time.Time, taskName string, step int, label string, content string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}

	_ = os.MkdirAll(artifactDir, 0755)

	path := artifactPath(taskName, step, label, now)
	path = uniqueArtifactPath(path)

	_ = os.WriteFile(path, []byte(content), 0644)

	return path
}

func artifactPath(taskName string, step int, label string, now time.Time) string {
	name := fmt.Sprintf(
		"%s-%s-step%03d-%s.txt",
		now.UTC().Format("020106"),
		slugArtifactPart(taskName, "task"),
		step,
		slugArtifactPart(label, "artifact"),
	)
	return filepath.Join(artifactDir, name)
}

func uniqueArtifactPath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}

	dir := filepath.Dir(path)
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(filepath.Base(path), ext)

	for i := 2; ; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s-%d%s", base, i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

func slugArtifactPart(value string, fallback string) string {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "" {
		return fallback
	}

	var b strings.Builder
	lastDash := false

	for _, r := range trimmed {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}

	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return fallback
	}

	return slug
}

func deriveTaskName(taskFile string, task string) string {
	if strings.TrimSpace(taskFile) != "" {
		base := filepath.Base(taskFile)
		return strings.TrimSuffix(base, filepath.Ext(base))
	}

	for _, line := range strings.Split(task, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "#") {
			return strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		}

		return trimmed
	}

	return "task"
}

func loadTaskInput(taskFile string, data []byte) (string, string, error) {
	task := string(data)

	parsedTask, err := taskspkg.ParseTaskMarkdown(taskFile, data)
	if err != nil {
		return task, "", nil
	}

	if len(parsedTask.Scripts) == 0 && len(parsedTask.Snippets) == 0 {
		return task, "", nil
	}

	packRoot, ok := findTaskPackRoot(taskFile)
	if !ok {
		return task, "", nil
	}

	pack, err := taskspkg.LoadTaskPack(packRoot)
	if err != nil {
		return "", "", err
	}

	taskPath, err := filepath.Abs(taskFile)
	if err != nil {
		return "", "", err
	}

	var matched *taskspkg.Task
	for i := range pack.Tasks {
		candidatePath, candidateErr := filepath.Abs(pack.Tasks[i].Path)
		if candidateErr != nil {
			continue
		}
		if filepath.Clean(candidatePath) == filepath.Clean(taskPath) {
			matched = &pack.Tasks[i]
			break
		}
	}
	if matched == nil {
		return task, packRoot, nil
	}

	resolvedTask := *matched
	if resolvedTask.PackContext != nil {
		ctxCopy := *resolvedTask.PackContext
		ctxCopy.AgentMode = taskspkg.AgentModeBoundedMarkdownContract
		resolvedTask.PackContext = &ctxCopy
	}

	contract, err := taskspkg.BuildTaskExecutionContract(resolvedTask)
	if err != nil {
		return "", "", err
	}
	return contract, packRoot, nil
}

func findTaskPackRoot(taskFile string) (string, bool) {
	current, err := filepath.Abs(filepath.Dir(taskFile))
	if err != nil {
		return "", false
	}

	for {
		manifestPath := filepath.Join(current, "PACK_MANIFEST.yaml")
		planPath := filepath.Join(current, "TASK_PLAN.md")

		if fileExists(manifestPath) && fileExists(planPath) {
			return current, true
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}
		current = parent
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func resolvePackReferencePath(packRoot string, actionPath string) (string, bool) {
	if strings.TrimSpace(packRoot) == "" {
		return "", false
	}

	cleaned := filepath.ToSlash(filepath.Clean(strings.TrimSpace(actionPath)))
	switch {
	case strings.HasPrefix(cleaned, "task-scripts/"),
		strings.HasPrefix(cleaned, "snippets/"),
		strings.HasPrefix(cleaned, "scripts/"):
		return filepath.ToSlash(filepath.Join(packRoot, filepath.FromSlash(cleaned))), true
	default:
		return "", false
	}
}

func isMissingFileError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such file or directory")
}

func previewText(text string, maxChars int) string {
	if len(text) <= maxChars {
		return text
	}

	return text[:maxChars] + "\n...[truncated]"
}

func summarizeError(err error) string {
	msg := err.Error()

	if strings.Contains(msg, "multiple JSON objects returned") {
		return "ERROR: multiple JSON objects returned (agent must return ONE action)"
	}

	if strings.Contains(msg, "invalid character") {
		return "ERROR: invalid JSON (likely bad escaping or formatting)"
	}

	if strings.Contains(msg, "unexpected end of JSON") {
		return "ERROR: incomplete JSON response"
	}

	if strings.Contains(msg, "no JSON object found") {
		return "ERROR: no JSON object in response"
	}

	if strings.Contains(msg, "top-level JSON arrays are not allowed") {
		return "ERROR: top-level JSON arrays are not allowed"
	}

	if strings.Contains(msg, "missing action or type") {
		return "ERROR: missing action or type"
	}

	// fallback
	return "ERROR: " + msg
}

func mustMarshalJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return `{}`
	}
	return string(data)
}

func splitterMessage(splitter actionpkg.TaskSplitter) string {
	if splitter == nil {
		return "no splitter configured"
	}

	_, err := splitter.Split("")
	if err != nil {
		return err.Error()
	}

	return "splitter available"
}
