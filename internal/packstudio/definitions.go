package packstudio

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/luannn010/ptolemy/internal/tasks"
)

const (
	packsDir    = "docs/tasks/packs"
	programsDir = "docs/tasks/programs"
)

func ListPacks(root string) ([]PackCatalogItem, error) {
	dir := filepath.Join(root, packsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []PackCatalogItem{}, nil
		}
		return nil, err
	}

	items := make([]PackCatalogItem, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		item := PackCatalogItem{
			ID:    entry.Name(),
			Name:  entry.Name(),
			Path:  filepath.Join(dir, entry.Name()),
			Valid: true,
		}
		pack, loadErr := tasks.LoadTaskPack(item.Path)
		if loadErr != nil {
			item.Valid = false
			item.ValidationErrors = []string{loadErr.Error()}
			items = append(items, item)
			continue
		}
		item.ID = pack.Manifest.PackID
		item.Name = pack.Manifest.Name
		item.TaskCount = len(pack.Tasks)
		items = append(items, item)
	}

	slices.SortFunc(items, func(a, b PackCatalogItem) int {
		return strings.Compare(a.ID, b.ID)
	})
	return items, nil
}

func GetPack(root string, packID string) (PackDetail, error) {
	packPath := filepath.Join(root, packsDir, packID)
	pack, err := tasks.LoadTaskPack(packPath)
	if err != nil {
		return PackDetail{
			PackCatalogItem: PackCatalogItem{
				ID:               packID,
				Name:             packID,
				Path:             packPath,
				Valid:            false,
				ValidationErrors: []string{err.Error()},
			},
		}, nil
	}

	readme, _ := os.ReadFile(pack.ReadmePath)
	plan, _ := os.ReadFile(pack.PlanPath)
	detail := PackDetail{
		PackCatalogItem: PackCatalogItem{
			ID:        pack.Manifest.PackID,
			Name:      pack.Manifest.Name,
			Path:      pack.Root,
			TaskCount: len(pack.Tasks),
			Valid:     true,
		},
		Goal:   firstMarkdownParagraph(string(plan)),
		Readme: string(readme),
		Manifest: map[string]string{
			"pack_id":        pack.Manifest.PackID,
			"name":           pack.Manifest.Name,
			"created_by":     pack.Manifest.CreatedBy,
			"entrypoint":     pack.Manifest.Entrypoint,
			"execution_mode": pack.Manifest.ExecutionMode,
		},
		Tasks: make([]PackTaskSummary, 0, len(pack.Tasks)),
	}

	taskValidationErrs := tasks.ValidateTasks(pack.Tasks)
	errMap := map[string][]string{}
	for _, validationErr := range taskValidationErrs {
		errMap[validationErr.TaskID] = append(errMap[validationErr.TaskID], fmt.Sprintf("%s: %s", validationErr.Field, validationErr.Reason))
	}

	for _, task := range pack.Tasks {
		detail.Tasks = append(detail.Tasks, PackTaskSummary{
			ID:               task.ID,
			Title:            extractTaskTitle(task),
			Path:             task.Path,
			Branch:           task.Branch,
			Status:           task.Status,
			DependsOn:        append([]string{}, task.DependsOn...),
			AllowedFiles:     append([]string{}, task.AllowedFiles...),
			Validation:       append([]string{}, task.Validation...),
			Checklist:        runtimeChecklist(task),
			ValidationErrors: errMap[task.ID],
		})
	}

	return detail, nil
}

func ListPrograms(root string) ([]ProgramCatalogItem, error) {
	dir := filepath.Join(root, programsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []ProgramCatalogItem{}, nil
		}
		return nil, err
	}

	items := make([]ProgramCatalogItem, 0, len(entries))
	knownPacks, _ := ListPacks(root)
	knownPackIDs := map[string]bool{}
	for _, item := range knownPacks {
		knownPackIDs[item.ID] = item.Valid
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name(), "PROGRAM.yaml")
		definition, loadErr := LoadProgramDefinition(path)
		item := ProgramCatalogItem{
			ID:   entry.Name(),
			Name: entry.Name(),
			Path: path,
		}
		if loadErr != nil {
			item.Valid = false
			item.ValidationErrors = []string{loadErr.Error()}
			items = append(items, item)
			continue
		}
		item.ID = definition.ID
		item.Name = definition.Name
		item.PackCount = len(definition.Packs)
		item.Valid = true
		for _, ref := range definition.Packs {
			if !knownPackIDs[ref.PackID] {
				item.Valid = false
				item.ValidationErrors = append(item.ValidationErrors, fmt.Sprintf("missing or invalid pack: %s", ref.PackID))
			}
		}
		items = append(items, item)
	}

	slices.SortFunc(items, func(a, b ProgramCatalogItem) int {
		return strings.Compare(a.ID, b.ID)
	})
	return items, nil
}

func GetProgram(root string, programID string) (ProgramDefinition, []string, error) {
	path := filepath.Join(root, programsDir, programID, "PROGRAM.yaml")
	definition, err := LoadProgramDefinition(path)
	if err != nil {
		return ProgramDefinition{}, nil, err
	}
	validationErrs := []string{}
	knownPacks, _ := ListPacks(root)
	knownPackIDs := map[string]bool{}
	for _, item := range knownPacks {
		knownPackIDs[item.ID] = item.Valid
	}
	for _, ref := range definition.Packs {
		if !knownPackIDs[ref.PackID] {
			validationErrs = append(validationErrs, fmt.Sprintf("missing or invalid pack: %s", ref.PackID))
		}
	}
	return definition, validationErrs, nil
}

func LoadProgramDefinition(path string) (ProgramDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ProgramDefinition{}, err
	}

	lines := strings.Split(string(data), "\n")
	definition := ProgramDefinition{Path: path, Packs: []ProgramPackRef{}}
	inPacks := false
	current := -1
	inDependsOn := false

	for _, rawLine := range lines {
		line := strings.TrimRight(rawLine, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !inPacks {
			if trimmed == "packs:" {
				inPacks = true
				continue
			}
			key, value, ok := splitKeyValue(trimmed)
			if !ok {
				return ProgramDefinition{}, fmt.Errorf("invalid program line: %q", trimmed)
			}
			switch key {
			case "program_id":
				definition.ID = value
			case "name":
				definition.Name = value
			case "description":
				definition.Description = value
			default:
				return ProgramDefinition{}, fmt.Errorf("unsupported program key: %s", key)
			}
			continue
		}

		if strings.HasPrefix(trimmed, "- pack_id:") {
			current++
			packID := strings.TrimSpace(strings.TrimPrefix(trimmed, "- pack_id:"))
			definition.Packs = append(definition.Packs, ProgramPackRef{
				PackID:    strings.Trim(packID, `"'`),
				DependsOn: []string{},
				Order:     current,
			})
			inDependsOn = false
			continue
		}
		if current < 0 {
			return ProgramDefinition{}, fmt.Errorf("packs section missing initial pack entry")
		}
		if trimmed == "depends_on:" {
			inDependsOn = true
			continue
		}
		if inDependsOn && strings.HasPrefix(trimmed, "- ") {
			definition.Packs[current].DependsOn = append(definition.Packs[current].DependsOn, strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")), `"'`))
			continue
		}
		return ProgramDefinition{}, fmt.Errorf("invalid program packs line: %q", trimmed)
	}

	if strings.TrimSpace(definition.ID) == "" {
		return ProgramDefinition{}, fmt.Errorf("program_id is required")
	}
	if strings.TrimSpace(definition.Name) == "" {
		return ProgramDefinition{}, fmt.Errorf("name is required")
	}
	if len(definition.Packs) == 0 {
		return ProgramDefinition{}, fmt.Errorf("at least one pack is required")
	}
	return definition, nil
}

func WritePack(root string, input CreatePackInput) (PackDetail, error) {
	if strings.TrimSpace(input.PackID) == "" {
		return PackDetail{}, fmt.Errorf("pack_id is required")
	}
	if strings.TrimSpace(input.Name) == "" {
		return PackDetail{}, fmt.Errorf("name is required")
	}
	if len(input.Tasks) == 0 {
		return PackDetail{}, fmt.Errorf("at least one task is required")
	}
	if input.MaxAllowedFiles <= 0 {
		input.MaxAllowedFiles = 8
	}
	if len(input.Validation) == 0 {
		input.Validation = []string{"go test ./..."}
	}
	if len(input.Requires) == 0 {
		input.Requires = []string{"git"}
	}
	if strings.TrimSpace(input.CreatedBy) == "" {
		input.CreatedBy = "pack-studio"
	}

	rootDir := filepath.Join(root, packsDir, input.PackID)
	if _, err := os.Stat(rootDir); err == nil {
		return PackDetail{}, fmt.Errorf("pack already exists: %s", input.PackID)
	}

	for _, dir := range []string{
		rootDir,
		filepath.Join(rootDir, "inbox"),
		filepath.Join(rootDir, "scripts"),
		filepath.Join(rootDir, "task-scripts"),
		filepath.Join(rootDir, "snippets"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return PackDetail{}, err
		}
	}

	if err := os.WriteFile(filepath.Join(rootDir, "PACK_MANIFEST.yaml"), []byte(renderPackManifest(input)), 0o644); err != nil {
		return PackDetail{}, err
	}
	if err := os.WriteFile(filepath.Join(rootDir, "TASK_PLAN.md"), []byte(renderTaskPlan(input)), 0o644); err != nil {
		return PackDetail{}, err
	}
	if err := os.WriteFile(filepath.Join(rootDir, "README.md"), []byte(renderPackReadme(input)), 0o644); err != nil {
		return PackDetail{}, err
	}

	for i, taskInput := range input.Tasks {
		taskFile := filepath.Join(rootDir, "inbox", renderTaskFilename(i, taskInput))
		if err := os.WriteFile(taskFile, []byte(renderTaskMarkdown(taskInput, i, input.Tasks)), 0o644); err != nil {
			return PackDetail{}, err
		}
	}

	return GetPack(root, input.PackID)
}

func WriteProgram(root string, input CreateProgramInput) (ProgramDefinition, error) {
	if strings.TrimSpace(input.ProgramID) == "" {
		return ProgramDefinition{}, fmt.Errorf("program_id is required")
	}
	if strings.TrimSpace(input.Name) == "" {
		return ProgramDefinition{}, fmt.Errorf("name is required")
	}
	if len(input.Packs) == 0 {
		return ProgramDefinition{}, fmt.Errorf("at least one pack is required")
	}

	rootDir := filepath.Join(root, programsDir, input.ProgramID)
	if _, err := os.Stat(rootDir); err == nil {
		return ProgramDefinition{}, fmt.Errorf("program already exists: %s", input.ProgramID)
	}
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return ProgramDefinition{}, err
	}
	for i := range input.Packs {
		input.Packs[i].Order = i
	}
	if err := os.WriteFile(filepath.Join(rootDir, "PROGRAM.yaml"), []byte(renderProgramYAML(input)), 0o644); err != nil {
		return ProgramDefinition{}, err
	}
	if err := os.WriteFile(filepath.Join(rootDir, "README.md"), []byte(renderProgramReadme(input)), 0o644); err != nil {
		return ProgramDefinition{}, err
	}
	return LoadProgramDefinition(filepath.Join(rootDir, "PROGRAM.yaml"))
}

func renderPackManifest(input CreatePackInput) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("pack_id: %s\n", input.PackID))
	builder.WriteString(fmt.Sprintf("name: %s\n", input.Name))
	builder.WriteString("version: 1\n")
	builder.WriteString(fmt.Sprintf("created_by: %s\n\n", input.CreatedBy))
	builder.WriteString("entrypoint: TASK_PLAN.md\n\n")
	builder.WriteString("folders:\n")
	builder.WriteString("  inbox: inbox\n")
	builder.WriteString("  scripts: scripts\n")
	builder.WriteString("  task_scripts: task-scripts\n")
	builder.WriteString("  snippets: snippets\n\n")
	builder.WriteString("execution_mode: sequential_first\n\n")
	builder.WriteString("requires:\n")
	for _, item := range input.Requires {
		builder.WriteString("  - " + item + "\n")
	}
	builder.WriteString("\nvalidation:\n")
	for _, item := range input.Validation {
		builder.WriteString("  - " + item + "\n")
	}
	builder.WriteString("\nrules:\n")
	builder.WriteString(fmt.Sprintf("  max_allowed_files: %d\n", input.MaxAllowedFiles))
	builder.WriteString(fmt.Sprintf("  require_validation: %t\n", input.RequireValidation))
	builder.WriteString(fmt.Sprintf("  require_branch: %t\n", input.RequireBranch))
	builder.WriteString(fmt.Sprintf("  stop_on_failure: %t\n", input.StopOnFailure))
	return builder.String()
}

func renderTaskPlan(input CreatePackInput) string {
	var builder strings.Builder
	builder.WriteString("# Task Plan: " + input.Name + "\n\n")
	if strings.TrimSpace(input.Goal) != "" {
		builder.WriteString("## Goal\n\n")
		builder.WriteString(strings.TrimSpace(input.Goal) + "\n\n")
	}
	builder.WriteString("## Execution Order\n\n")
	for i, task := range input.Tasks {
		builder.WriteString(fmt.Sprintf("%d. `%s`\n", i+1, task.Title))
	}
	builder.WriteString("\n## Notes\n\n")
	builder.WriteString("- Generated by Pack Studio.\n")
	builder.WriteString("- Tasks run sequentially through Ptolemy agent-runs.\n")
	builder.WriteString("- Use the Run Monitor for live progress, terminal output, and task history.\n")
	return builder.String()
}

func renderPackReadme(input CreatePackInput) string {
	var builder strings.Builder
	builder.WriteString("# " + input.Name + "\n\n")
	if strings.TrimSpace(input.Description) != "" {
		builder.WriteString(strings.TrimSpace(input.Description) + "\n\n")
	}
	builder.WriteString("## Goal\n\n")
	builder.WriteString(strings.TrimSpace(firstNonEmpty(input.Goal, input.Description, "Generated by Pack Studio.")) + "\n\n")
	builder.WriteString("## Tasks\n\n")
	for _, task := range input.Tasks {
		builder.WriteString("- `" + task.ID + "` " + task.Title + "\n")
	}
	return builder.String()
}

func renderTaskFilename(index int, task PackTaskInput) string {
	slug := slugify(firstNonEmpty(task.Title, task.ID))
	return fmt.Sprintf("%02d-%s.md", index+1, slug)
}

func renderTaskMarkdown(task PackTaskInput, index int, all []PackTaskInput) string {
	group := strings.TrimSpace(task.ExecutionGroup)
	if group == "" {
		group = "sequential"
	}
	var builder strings.Builder
	builder.WriteString("---\n")
	builder.WriteString(fmt.Sprintf("task_id: %s\n", task.ID))
	builder.WriteString("priority: normal\n")
	builder.WriteString("parent_task: null\n")
	builder.WriteString("owner: unassigned\n")
	builder.WriteString("status: inbox\n")
	builder.WriteString(fmt.Sprintf("branch: %s\n", firstNonEmpty(task.Branch, fmt.Sprintf("ptolemy/%s", slugify(task.ID)))))
	builder.WriteString("created_by: pack-studio\n")
	builder.WriteString(fmt.Sprintf("execution_group: %s\n", group))
	builder.WriteString("allowed_files:\n")
	for _, item := range uniqueStrings(task.AllowedFiles) {
		builder.WriteString("  - " + item + "\n")
	}
	if len(task.DependsOn) > 0 {
		builder.WriteString("depends_on:\n")
		for _, dep := range uniqueStrings(task.DependsOn) {
			builder.WriteString("  - " + dep + "\n")
		}
	}
	if len(task.Validation) > 0 {
		builder.WriteString("validation:\n")
		for _, validation := range uniqueStrings(task.Validation) {
			builder.WriteString("  - " + validation + "\n")
		}
	}
	if len(task.Scripts) > 0 {
		builder.WriteString("scripts:\n")
		for _, script := range uniqueStrings(task.Scripts) {
			builder.WriteString("  - " + script + "\n")
		}
	}
	if len(task.Snippets) > 0 {
		builder.WriteString("snippets:\n")
		for _, snippet := range uniqueStrings(task.Snippets) {
			builder.WriteString("  - " + snippet + "\n")
		}
	}
	builder.WriteString("---\n")
	builder.WriteString("# " + firstNonEmpty(task.Title, task.ID) + "\n\n")
	builder.WriteString(taskSummary(task) + "\n\n")
	builder.WriteString("## Checklist\n\n")
	for _, item := range generatedChecklist(task, index, len(all)) {
		builder.WriteString("- [ ] " + item.Text + "\n")
	}
	builder.WriteString("\n## Notes\n\n")
	builder.WriteString("- Execute only this task.\n")
	builder.WriteString("- Use Pack Studio monitoring for progress and terminal output.\n")
	return builder.String()
}

func renderProgramYAML(input CreateProgramInput) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("program_id: %s\n", input.ProgramID))
	builder.WriteString(fmt.Sprintf("name: %s\n", input.Name))
	builder.WriteString(fmt.Sprintf("description: %s\n\n", input.Description))
	builder.WriteString("packs:\n")
	for _, ref := range input.Packs {
		builder.WriteString(fmt.Sprintf("  - pack_id: %s\n", ref.PackID))
		if len(ref.DependsOn) > 0 {
			builder.WriteString("    depends_on:\n")
			for _, dep := range uniqueStrings(ref.DependsOn) {
				builder.WriteString("      - " + dep + "\n")
			}
		}
	}
	return builder.String()
}

func renderProgramReadme(input CreateProgramInput) string {
	var builder strings.Builder
	builder.WriteString("# " + input.Name + "\n\n")
	if strings.TrimSpace(input.Description) != "" {
		builder.WriteString(strings.TrimSpace(input.Description) + "\n\n")
	}
	builder.WriteString("## Packs\n\n")
	for _, ref := range input.Packs {
		builder.WriteString("- `" + ref.PackID + "`\n")
	}
	return builder.String()
}

func generatedChecklist(task PackTaskInput, index int, total int) []ChecklistItem {
	items := []ChecklistItem{
		{Text: "Task planned"},
		{Text: "Task running"},
		{Text: "Validation passed"},
		{Text: "Task completed"},
	}
	if index == total-1 {
		items = append(items, ChecklistItem{Text: "Pack final validation reviewed"})
	}
	return items
}

func runtimeChecklist(task tasks.Task) []ChecklistItem {
	items := parseChecklist(task.Body)
	if len(items) == 0 {
		return statusChecklist(task.Status)
	}
	return applyStatusToChecklist(items, task.Status)
}

func parseChecklist(body string) []ChecklistItem {
	lines := strings.Split(body, "\n")
	inChecklist := false
	items := []ChecklistItem{}
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if strings.HasPrefix(line, "## ") {
			if strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(line, "## ")), "Checklist") {
				inChecklist = true
				continue
			}
			if inChecklist {
				break
			}
		}
		if !inChecklist {
			continue
		}
		switch {
		case strings.HasPrefix(line, "- [ ] "):
			items = append(items, ChecklistItem{Text: strings.TrimSpace(strings.TrimPrefix(line, "- [ ] "))})
		case strings.HasPrefix(line, "- [x] "), strings.HasPrefix(line, "- [X] "):
			items = append(items, ChecklistItem{Text: strings.TrimSpace(line[6:]), Checked: true})
		}
	}
	return items
}

func statusChecklist(status string) []ChecklistItem {
	items := []ChecklistItem{
		{Text: "Task planned"},
		{Text: "Task running"},
		{Text: "Validation passed"},
		{Text: "Task completed"},
		{Text: "Task failed"},
	}
	return applyStatusToChecklist(items, status)
}

func applyStatusToChecklist(items []ChecklistItem, status string) []ChecklistItem {
	out := make([]ChecklistItem, len(items))
	copy(out, items)
	switch {
	case status == StatusRunning || status == StatusWaitingOnAgent || status == tasks.StatusRunning:
		if len(out) > 0 {
			out[0].Checked = true
		}
		if len(out) > 1 {
			out[1].Checked = true
		}
	case status == StatusCompleted || status == tasks.StatusCompleted:
		for i := range out {
			if strings.EqualFold(out[i].Text, "Task failed") {
				continue
			}
			out[i].Checked = true
		}
	case status == StatusFailed || status == StatusCancelled || status == tasks.StatusFailed:
		if len(out) > 0 {
			out[0].Checked = true
		}
		if len(out) > 1 {
			out[1].Checked = true
		}
		for i := range out {
			if strings.EqualFold(out[i].Text, "Task failed") {
				out[i].Checked = true
			}
		}
	}
	return out
}

func splitKeyValue(line string) (string, string, bool) {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.Trim(strings.TrimSpace(parts[1]), `"'`), true
}

func extractTaskTitle(task tasks.Task) string {
	for _, line := range strings.Split(task.Body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		}
	}
	return task.ID
}

func firstMarkdownParagraph(content string) string {
	for _, block := range strings.Split(content, "\n\n") {
		trimmed := strings.TrimSpace(block)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		return trimmed
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", "-")
	replacer := strings.NewReplacer(" ", "-", "/", "-", "\\", "-", ":", "-", ".", "-", ",", "-", "(", "", ")", "")
	value = replacer.Replace(value)
	value = strings.Trim(value, "-")
	if value == "" {
		return "task"
	}
	return value
}

func uniqueStrings(items []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func taskSummary(task PackTaskInput) string {
	if strings.TrimSpace(task.Summary) != "" {
		return strings.TrimSpace(task.Summary)
	}
	payload, _ := json.Marshal(map[string]any{
		"task_id": task.ID,
		"purpose": "Fill in the exact implementation summary for this task.",
	})
	return fmt.Sprintf("Generated task placeholder. Update this summary if needed.\n\n```json\n%s\n```", string(payload))
}
