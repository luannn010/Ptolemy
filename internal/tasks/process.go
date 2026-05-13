package tasks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	ProcessStatusPending = "pending"
	ProcessStatusRunning = "running"
	ProcessStatusDone    = "done"
	ProcessStatusFailed  = "failed"
	ProcessStatusSkipped = "skipped"
)

type ProcessManifest struct {
	PackID             string             `json:"pack_id"`
	ParentTaskFile     string             `json:"parent_task_file"`
	Branch             string             `json:"branch"`
	Status             string             `json:"status"`
	CurrentChildTaskID string             `json:"current_child_task_id"`
	Dependencies       []string           `json:"dependencies"`
	AllowedFiles       []string           `json:"allowed_files"`
	ValidationCommands []string           `json:"validation_commands"`
	ChildTasks         []ProcessChildTask `json:"child_tasks"`
}

type ProcessChildTask struct {
	ID                  string   `json:"id"`
	Title               string   `json:"title"`
	Phase               string   `json:"phase"`
	Body                string   `json:"body"`
	Dependencies        []string `json:"dependencies"`
	AllowedFiles        []string `json:"allowed_files"`
	ValidationCommands  []string `json:"validation_commands"`
	Status              string   `json:"status"`
	ResultSummaryPath   string   `json:"result_summary_path"`
	FilesChanged        []string `json:"files_changed"`
	CommandsRun         []string `json:"commands_run"`
	FailureReason       string   `json:"failure_reason"`
	MaxSteps            int      `json:"max_steps"`
	EstimatedBodyTokens int      `json:"estimated_body_tokens"`
	Warning             string   `json:"warning,omitempty"`
}

type ProcessPaths struct {
	Root         string
	ManifestPath string
	TodoPath     string
	StateDir     string
}

type ProcessSummary struct {
	ChildTaskID         string
	Title               string
	Status              string
	FilesInspected      []string
	FilesChanged        []string
	CommandsRun         []string
	ValidationResult    string
	ImportantDecisions  []string
	ErrorsBlockers      []string
	NextRecommendedStep string
}

func BuildProcessManifest(task Task) ProcessManifest {
	childTasks := buildProcessChildTasks(task)
	return ProcessManifest{
		PackID:             processPackID(task),
		ParentTaskFile:     task.Path,
		Branch:             task.Branch,
		Status:             ProcessStatusPending,
		CurrentChildTaskID: "",
		Dependencies:       append([]string{}, task.DependsOn...),
		AllowedFiles:       append([]string{}, task.AllowedFiles...),
		ValidationCommands: append([]string{}, task.Validation...),
		ChildTasks:         childTasks,
	}
}

func EnsureProcessFiles(workspace string, manifest ProcessManifest) (ProcessPaths, error) {
	root := filepath.Join(workspace, ".ptolemy", "tasks", "process", sanitizeArtifactName(manifest.PackID))
	paths := ProcessPaths{
		Root:         root,
		ManifestPath: filepath.Join(root, "manifest.json"),
		TodoPath:     filepath.Join(root, "todo.md"),
		StateDir:     filepath.Join(root, "state"),
	}

	for _, dir := range []string{root, paths.StateDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return ProcessPaths{}, fmt.Errorf("create %s: %w", dir, err)
		}
	}

	if err := WriteProcessManifest(paths.ManifestPath, manifest); err != nil {
		return ProcessPaths{}, err
	}
	if err := WriteProcessTodo(paths.TodoPath, manifest); err != nil {
		return ProcessPaths{}, err
	}

	return paths, nil
}

func LoadProcessManifest(paths ProcessPaths) (ProcessManifest, error) {
	data, err := os.ReadFile(paths.ManifestPath)
	if err != nil {
		return ProcessManifest{}, err
	}

	var manifest ProcessManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return ProcessManifest{}, fmt.Errorf("decode process manifest: %w", err)
	}
	return manifest, nil
}

func WriteProcessManifest(path string, manifest ProcessManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode process manifest: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func WriteProcessTodo(path string, manifest ProcessManifest) error {
	return os.WriteFile(path, []byte(RenderProcessTodo(manifest)), 0o644)
}

func WriteProcessSummary(paths ProcessPaths, summary ProcessSummary) (string, error) {
	name := sanitizeArtifactName(summary.ChildTaskID) + "-result.md"
	target := filepath.Join(paths.StateDir, name)
	if err := os.WriteFile(target, []byte(RenderProcessSummary(summary)), 0o644); err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Join(".ptolemy", "tasks", "process", filepath.Base(paths.Root), "state", name)), nil
}

func RenderProcessTodo(manifest ProcessManifest) string {
	var builder strings.Builder

	builder.WriteString("# Task Process\n\n")
	builder.WriteString(fmt.Sprintf("- Pack ID: `%s`\n", manifest.PackID))
	builder.WriteString(fmt.Sprintf("- Parent task file: `%s`\n", manifest.ParentTaskFile))
	builder.WriteString(fmt.Sprintf("- Branch: `%s`\n", manifest.Branch))
	builder.WriteString(fmt.Sprintf("- Status: `%s`\n", manifest.Status))
	if strings.TrimSpace(manifest.CurrentChildTaskID) != "" {
		builder.WriteString(fmt.Sprintf("- Current child task: `%s`\n", manifest.CurrentChildTaskID))
	}

	builder.WriteString("\n## Child Tasks\n")
	for _, child := range manifest.ChildTasks {
		builder.WriteString(fmt.Sprintf("- [%s] `%s` %s (%s)\n", todoCheckbox(child.Status), child.ID, child.Title, child.Phase))
	}

	return builder.String()
}

func RenderProcessSummary(summary ProcessSummary) string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("# %s\n\n", summary.ChildTaskID))
	builder.WriteString(fmt.Sprintf("- Title: %s\n", summary.Title))
	builder.WriteString(fmt.Sprintf("- Status: %s\n", summary.Status))
	builder.WriteString(fmt.Sprintf("- Validation result: %s\n", withDefault(summary.ValidationResult, "not run")))

	builder.WriteString("\n## Files inspected\n")
	builder.WriteString(markdownListOrNone(summary.FilesInspected))

	builder.WriteString("\n## Files changed\n")
	builder.WriteString(markdownListOrNone(summary.FilesChanged))

	builder.WriteString("\n## Commands run\n")
	builder.WriteString(markdownListOrNone(summary.CommandsRun))

	builder.WriteString("\n## Important decisions\n")
	builder.WriteString(markdownListOrNone(summary.ImportantDecisions))

	builder.WriteString("\n## Errors or blockers\n")
	builder.WriteString(markdownListOrNone(summary.ErrorsBlockers))

	builder.WriteString("\n## Next recommended step\n")
	builder.WriteString(withDefault(strings.TrimSpace(summary.NextRecommendedStep), "none"))
	builder.WriteString("\n")

	return builder.String()
}

func ProcessPathsForWorkspace(workspace string, packID string) ProcessPaths {
	root := filepath.Join(workspace, ".ptolemy", "tasks", "process", sanitizeArtifactName(packID))
	return ProcessPaths{
		Root:         root,
		ManifestPath: filepath.Join(root, "manifest.json"),
		TodoPath:     filepath.Join(root, "todo.md"),
		StateDir:     filepath.Join(root, "state"),
	}
}

func buildProcessChildTasks(task Task) []ProcessChildTask {
	sections := splitTaskSections(task.Body)
	if len(sections) >= 2 {
		children := make([]ProcessChildTask, 0, len(sections))
		for i, section := range sections {
			phase := inferChildTaskPhase(section.Title + "\n" + section.Body)
			allowedFiles := narrowAllowedFiles(section.Title+"\n"+section.Body, task.AllowedFiles, phase)
			if len(allowedFiles) == 0 {
				allowedFiles = firstN(task.AllowedFiles, 1)
			}
			child := newProcessChildTask(task, i+1, section.Title, phase, buildSectionChildBody(task, section, phase, allowedFiles), allowedFiles, validationForPhase(task, phase))
			if len(children) > 0 {
				child.Dependencies = []string{children[len(children)-1].ID}
			}
			children = append(children, child)
		}
		return children
	}

	return buildDefaultProcessChildTasks(task)
}

func buildDefaultProcessChildTasks(task Task) []ProcessChildTask {
	phases := []struct {
		Title      string
		Phase      string
		Body       string
		Allowed    []string
		Validation []string
	}{
		{
			Title:      "Inspect current scope",
			Phase:      "inspect",
			Allowed:    narrowAllowedFiles(task.Body, task.AllowedFiles, "inspect"),
			Validation: nil,
		},
		{
			Title:      "Plan minimal edits",
			Phase:      "plan",
			Allowed:    narrowAllowedFiles(task.Body, task.AllowedFiles, "plan"),
			Validation: nil,
		},
		{
			Title:      "Implement core change",
			Phase:      "edit",
			Allowed:    narrowAllowedFiles(task.Body, task.AllowedFiles, "edit"),
			Validation: nil,
		},
	}

	if hasTestScope(task) {
		phases = append(phases, struct {
			Title      string
			Phase      string
			Body       string
			Allowed    []string
			Validation []string
		}{
			Title:      "Add focused tests",
			Phase:      "test",
			Allowed:    narrowAllowedFiles(task.Body, task.AllowedFiles, "test"),
			Validation: validationForPhase(task, "test"),
		})
	}

	if len(task.Validation) > 0 {
		phases = append(phases, struct {
			Title      string
			Phase      string
			Body       string
			Allowed    []string
			Validation []string
		}{
			Title:      "Run validation",
			Phase:      "validate",
			Allowed:    narrowAllowedFiles(task.Body, task.AllowedFiles, "validate"),
			Validation: append([]string{}, task.Validation...),
		})
	}

	phases = append(phases, struct {
		Title      string
		Phase      string
		Body       string
		Allowed    []string
		Validation []string
	}{
		Title:      "Finalize summary",
		Phase:      "finalize",
		Allowed:    narrowAllowedFiles(task.Body, task.AllowedFiles, "finalize"),
		Validation: nil,
	})

	children := make([]ProcessChildTask, 0, len(phases))
	for i, phase := range phases {
		body := buildDefaultChildBody(task, phase.Title, phase.Phase, phase.Allowed, phase.Validation)
		child := newProcessChildTask(task, i+1, phase.Title, phase.Phase, body, phase.Allowed, phase.Validation)
		if len(children) > 0 {
			child.Dependencies = []string{children[len(children)-1].ID}
		}
		children = append(children, child)
	}
	return children
}

type taskSection struct {
	Title string
	Body  string
}

func splitTaskSections(body string) []taskSection {
	lines := strings.Split(body, "\n")
	var sections []taskSection
	var current *taskSection

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "### ") {
			if current != nil && strings.TrimSpace(current.Body) != "" {
				sections = append(sections, *current)
			}
			current = &taskSection{Title: strings.TrimSpace(strings.TrimLeft(line, "#"))}
			continue
		}
		if numberedSectionTitle(line) {
			if current != nil && strings.TrimSpace(current.Body) != "" {
				sections = append(sections, *current)
			}
			current = &taskSection{Title: line}
			continue
		}
		if current == nil {
			continue
		}
		current.Body += rawLine + "\n"
	}

	if current != nil && strings.TrimSpace(current.Body) != "" {
		sections = append(sections, *current)
	}

	return sections
}

func numberedSectionTitle(line string) bool {
	if len(line) < 3 {
		return false
	}
	for i, r := range line {
		if r == '.' && i > 0 {
			return i+1 < len(line) && line[i+1] == ' '
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	return false
}

func inferChildTaskPhase(text string) string {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "inspect"), strings.Contains(lower, "research"), strings.Contains(lower, "discover"):
		return "inspect"
	case strings.Contains(lower, "plan"):
		return "plan"
	case strings.Contains(lower, "validat"), strings.Contains(lower, "verify"), strings.Contains(lower, "smoke"):
		return "validate"
	case strings.Contains(lower, "test"):
		return "test"
	case strings.Contains(lower, "doc"), strings.Contains(lower, "readme"):
		return "docs"
	case strings.Contains(lower, "final"):
		return "finalize"
	default:
		return "edit"
	}
}

func validationForPhase(task Task, phase string) []string {
	switch phase {
	case "test", "validate":
		return append([]string{}, task.Validation...)
	default:
		return nil
	}
}

func buildSectionChildBody(task Task, section taskSection, phase string, allowedFiles []string) string {
	goal := strings.TrimSpace(section.Title)
	scope := strings.TrimSpace(section.Body)
	if scope == "" {
		scope = strings.TrimSpace(task.Body)
	}

	return buildChildBody(taskTitleFromBody(task.Body, task.ID), goal, phase, scope, allowedFiles, validationForPhase(task, phase))
}

func buildDefaultChildBody(task Task, title string, phase string, allowedFiles []string, validation []string) string {
	scope := strings.TrimSpace(task.Body)
	return buildChildBody(taskTitleFromBody(task.Body, task.ID), title, phase, scope, allowedFiles, validation)
}

func buildChildBody(parentTitle string, title string, phase string, scope string, allowedFiles []string, validation []string) string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("# %s\n\n", title))
	builder.WriteString(fmt.Sprintf("Parent goal: %s\n", parentTitle))
	builder.WriteString(fmt.Sprintf("Phase: %s\n\n", phase))
	builder.WriteString("Goal:\n")
	builder.WriteString(fmt.Sprintf("- %s\n\n", strings.TrimSpace(title)))
	builder.WriteString("Scope:\n")
	builder.WriteString(strings.TrimSpace(scope))
	builder.WriteString("\n\nAllowed files:\n")
	for _, file := range allowedFiles {
		builder.WriteString(fmt.Sprintf("- %s\n", file))
	}
	builder.WriteString("\nExplicit steps:\n")
	for _, step := range childTaskSteps(phase, len(validation) > 0) {
		builder.WriteString(fmt.Sprintf("- %s\n", step))
	}
	builder.WriteString("\nRules:\n")
	for _, rule := range []string{
		"Execute only this child task.",
		"Do not solve later child tasks.",
		"Do not request the whole repository.",
		"Stay inside allowed_files.",
		"Do not mix push, PR, or reporting work into implementation steps.",
	} {
		builder.WriteString(fmt.Sprintf("- %s\n", rule))
	}
	if len(validation) > 0 {
		builder.WriteString("\nValidation target:\n")
		for _, command := range validation {
			builder.WriteString(fmt.Sprintf("- %s\n", command))
		}
	}
	return builder.String()
}

func childTaskSteps(phase string, includeValidation bool) []string {
	switch phase {
	case "inspect":
		return []string{
			"Read only the scoped files needed to understand the current behavior.",
			"Capture concrete constraints, dependencies, and likely edit points.",
			"Do not edit files; finish with explain summarizing the findings.",
		}
	case "plan":
		return []string{
			"Read only the scoped files required to plan the smallest safe change.",
			"Identify the exact files and edit strategy for the next child task.",
			"Finish with explain summarizing the plan and any blocker.",
		}
	case "test":
		return []string{
			"Add or adjust only the focused tests required for this scope.",
			"Keep the test changes narrow and directly tied to the target behavior.",
			"Run the relevant validation command if one is listed, then finish with explain.",
		}
	case "validate":
		return []string{
			"Run only the listed validation commands.",
			"Summarize the concrete pass or failure result with any actionable error.",
			"Do not broaden the scope; finish with explain.",
		}
	case "docs":
		return []string{
			"Update only the scoped documentation that reflects the completed behavior.",
			"Keep the documentation aligned with the actual implementation state.",
			"Finish with explain summarizing the doc update.",
		}
	case "finalize":
		return []string{
			"Review the child-task outcomes already completed for this process.",
			"Summarize what changed, what was validated, and any remaining risk.",
			"Finish with explain and do not start new implementation work.",
		}
	default:
		steps := []string{
			"Read only the scoped files required for this change.",
			"Make the smallest reversible implementation change for this child task.",
			"Finish with explain summarizing files changed and any remaining follow-up.",
		}
		if includeValidation {
			steps = []string{
				"Read only the scoped files required for this change.",
				"Make the smallest reversible implementation change for this child task.",
				"Run the relevant validation command if needed, then finish with explain.",
			}
		}
		return steps
	}
}

func newProcessChildTask(parent Task, index int, title string, phase string, body string, allowedFiles []string, validation []string) ProcessChildTask {
	id := fmt.Sprintf("%03d-%s", index, sanitizeArtifactName(strings.ToLower(strings.ReplaceAll(title, " ", "-"))))
	id = strings.TrimSuffix(strings.TrimSuffix(id, "-"), ".md")
	if len(allowedFiles) == 0 {
		allowedFiles = append([]string{}, parent.AllowedFiles...)
	}
	warning := ChildTaskBodyWarning(body)
	return ProcessChildTask{
		ID:                  id,
		Title:               title,
		Phase:               phase,
		Body:                body,
		Dependencies:        []string{},
		AllowedFiles:        append([]string{}, allowedFiles...),
		ValidationCommands:  append([]string{}, validation...),
		Status:              ProcessStatusPending,
		ResultSummaryPath:   "",
		FilesChanged:        []string{},
		CommandsRun:         []string{},
		FailureReason:       "",
		MaxSteps:            StepBudgetForSize(ClassifyTaskBody(body)),
		EstimatedBodyTokens: EstimateTokens(body),
		Warning:             warning,
	}
}

func hasTestScope(task Task) bool {
	for _, file := range task.AllowedFiles {
		lower := strings.ToLower(file)
		if strings.Contains(lower, "_test.go") || strings.Contains(lower, "test") {
			return true
		}
	}
	if len(task.Validation) > 0 {
		return true
	}
	return strings.Contains(strings.ToLower(task.Body), "test")
}

func narrowAllowedFiles(text string, allowedFiles []string, phase string) []string {
	matched := matchAllowedFiles(text, allowedFiles)
	if len(matched) > 0 {
		return firstN(matched, 5)
	}

	switch phase {
	case "test":
		if tests := filterAllowedFiles(allowedFiles, func(path string) bool {
			lower := strings.ToLower(path)
			return strings.Contains(lower, "_test.go") || strings.Contains(lower, "test")
		}); len(tests) > 0 {
			return firstN(tests, 4)
		}
	case "docs", "finalize":
		if docs := filterAllowedFiles(allowedFiles, func(path string) bool {
			lower := strings.ToLower(path)
			return strings.HasSuffix(lower, ".md") || strings.Contains(lower, "doc")
		}); len(docs) > 0 {
			return firstN(docs, 4)
		}
	default:
		if sources := filterAllowedFiles(allowedFiles, func(path string) bool {
			lower := strings.ToLower(path)
			return !strings.Contains(lower, "_test.go") && !strings.HasSuffix(lower, ".md")
		}); len(sources) > 0 {
			return firstN(sources, 4)
		}
	}

	return firstN(allowedFiles, 4)
}

func matchAllowedFiles(text string, allowedFiles []string) []string {
	lower := strings.ToLower(text)
	matches := make([]string, 0)
	for _, path := range allowedFiles {
		base := strings.ToLower(filepath.Base(path))
		full := strings.ToLower(filepath.ToSlash(path))
		if strings.Contains(lower, base) || strings.Contains(lower, full) {
			matches = append(matches, path)
		}
	}
	sort.Strings(matches)
	return dedupeStrings(matches)
}

func filterAllowedFiles(paths []string, keep func(string) bool) []string {
	filtered := make([]string, 0, len(paths))
	for _, path := range paths {
		if keep(path) {
			filtered = append(filtered, path)
		}
	}
	return filtered
}

func firstN(items []string, limit int) []string {
	if len(items) == 0 {
		return nil
	}
	if limit <= 0 || len(items) <= limit {
		return append([]string{}, items...)
	}
	return append([]string{}, items[:limit]...)
}

func processPackID(task Task) string {
	if strings.TrimSpace(task.ID) != "" {
		return task.ID
	}
	base := strings.TrimSuffix(filepath.Base(task.Path), filepath.Ext(task.Path))
	if strings.TrimSpace(base) != "" {
		return sanitizeArtifactName(base)
	}
	return "task-process"
}

func taskTitleFromBody(body string, fallback string) string {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			return strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		}
	}
	if strings.TrimSpace(fallback) != "" {
		return fallback
	}
	return "Task"
}

func todoCheckbox(status string) string {
	switch status {
	case ProcessStatusDone:
		return "x"
	case ProcessStatusRunning:
		return ">"
	case ProcessStatusFailed:
		return "!"
	case ProcessStatusSkipped:
		return "-"
	default:
		return " "
	}
}

func markdownListOrNone(items []string) string {
	if len(items) == 0 {
		return "- none\n"
	}
	unique := dedupeStrings(items)
	sort.Strings(unique)
	var builder strings.Builder
	for _, item := range unique {
		builder.WriteString(fmt.Sprintf("- %s\n", item))
	}
	return builder.String()
}

func dedupeStrings(items []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	return out
}
