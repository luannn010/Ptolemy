package tasks

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const supportedPackExecutionMode = "sequential_first"

type PackManifest struct {
	PackID        string
	Name          string
	Version       int
	CreatedBy     string
	Entrypoint    string
	Folders       PackFolders
	Requires      []string
	Validation    []string
	ExecutionMode string
	Rules         PackRules
}

type PackFolders struct {
	Inbox       string
	Scripts     string
	TaskScripts string
	Snippets    string
}

type PackRules struct {
	MaxAllowedFiles   int
	RequireValidation bool
	RequireBranch     bool
	StopOnFailure     bool
}

type TaskPack struct {
	Root         string
	PlanPath     string
	ManifestPath string
	ReadmePath   string
	Manifest     PackManifest
	Tasks        []Task
}

type TaskPackContext struct {
	Root           string
	ScriptsDir     string
	TaskScriptsDir string
	SnippetsDir    string
}

func BuildPackPlanPreview(packDir string) ([]string, []ValidationError, error) {
	pack, err := LoadTaskPack(packDir)
	if err != nil {
		return nil, nil, err
	}
	return buildPlanPreviewForTasks(pack.Tasks)
}

func RunTaskPack(ctx context.Context, packDir string, workspace string) SchedulerResult {
	result := SchedulerResult{
		PlannedTaskIDs:   []string{},
		CompletedTaskIDs: []string{},
		ValidationErrors: []ValidationError{},
	}

	pack, err := LoadTaskPack(packDir)
	if err != nil {
		result.ValidationErrors = append(result.ValidationErrors, ValidationError{
			TaskID: "<pack>",
			Field:  "pack",
			Reason: err.Error(),
		})
		return result
	}

	return runTaskList(ctx, pack.Tasks, workspace)
}

func LoadTaskPack(packDir string) (TaskPack, error) {
	root, err := filepath.Abs(packDir)
	if err != nil {
		return TaskPack{}, fmt.Errorf("resolve pack dir: %w", err)
	}

	pack := TaskPack{
		Root:         root,
		PlanPath:     filepath.Join(root, "TASK_PLAN.md"),
		ManifestPath: filepath.Join(root, "PACK_MANIFEST.yaml"),
		ReadmePath:   filepath.Join(root, "README.md"),
	}

	for _, requiredFile := range []string{pack.PlanPath, pack.ManifestPath, pack.ReadmePath} {
		if err := requireFile(requiredFile); err != nil {
			return TaskPack{}, err
		}
	}

	manifestContent, err := os.ReadFile(pack.ManifestPath)
	if err != nil {
		return TaskPack{}, fmt.Errorf("read pack manifest: %w", err)
	}

	manifest, err := ParsePackManifest(manifestContent)
	if err != nil {
		return TaskPack{}, err
	}
	if manifest.Folders.Inbox == "" {
		return TaskPack{}, fmt.Errorf("pack manifest missing folders.inbox")
	}
	if manifest.Folders.Scripts == "" {
		return TaskPack{}, fmt.Errorf("pack manifest missing folders.scripts")
	}
	if manifest.Folders.TaskScripts == "" {
		return TaskPack{}, fmt.Errorf("pack manifest missing folders.task_scripts")
	}
	if manifest.Folders.Snippets == "" {
		return TaskPack{}, fmt.Errorf("pack manifest missing folders.snippets")
	}
	if manifest.ExecutionMode != supportedPackExecutionMode {
		return TaskPack{}, fmt.Errorf("unsupported execution_mode: %s", manifest.ExecutionMode)
	}
	if strings.TrimSpace(manifest.Entrypoint) == "" {
		return TaskPack{}, fmt.Errorf("pack manifest missing entrypoint")
	}

	entrypointPath := filepath.Join(root, filepath.FromSlash(manifest.Entrypoint))
	if err := requireFile(entrypointPath); err != nil {
		return TaskPack{}, err
	}

	for _, requiredDir := range []string{
		filepath.Join(root, filepath.FromSlash(manifest.Folders.Inbox)),
		filepath.Join(root, filepath.FromSlash(manifest.Folders.Scripts)),
		filepath.Join(root, filepath.FromSlash(manifest.Folders.TaskScripts)),
		filepath.Join(root, filepath.FromSlash(manifest.Folders.Snippets)),
	} {
		if err := requireDir(requiredDir); err != nil {
			return TaskPack{}, err
		}
	}

	pack.Manifest = manifest

	tasks, err := ScanInbox(filepath.Join(root, filepath.FromSlash(manifest.Folders.Inbox)))
	if err != nil {
		return TaskPack{}, err
	}

	context := &TaskPackContext{
		Root:           root,
		ScriptsDir:     normalizeRelativeDir(manifest.Folders.Scripts),
		TaskScriptsDir: normalizeRelativeDir(manifest.Folders.TaskScripts),
		SnippetsDir:    normalizeRelativeDir(manifest.Folders.Snippets),
	}

	for i := range tasks {
		tasks[i].PackContext = context
	}
	pack.Tasks = tasks

	return pack, nil
}

func ParsePackManifest(content []byte) (PackManifest, error) {
	manifest := PackManifest{
		Requires:      []string{},
		Validation:    []string{},
		ExecutionMode: supportedPackExecutionMode,
	}

	lines := strings.Split(string(content), "\n")
	currentSection := ""
	currentList := ""

	for _, rawLine := range lines {
		line := strings.TrimRight(rawLine, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := len(line) - len(strings.TrimLeft(line, " "))

		if indent == 0 {
			currentSection = ""
			currentList = ""

			if strings.HasSuffix(trimmed, ":") {
				section := strings.TrimSuffix(trimmed, ":")
				switch section {
				case "folders", "rules", "requires", "validation":
					currentSection = section
					if section == "requires" || section == "validation" {
						currentList = section
					}
					continue
				default:
					return PackManifest{}, fmt.Errorf("unsupported manifest section: %s", section)
				}
			}

			key, value, ok := splitManifestKeyValue(trimmed)
			if !ok {
				return PackManifest{}, fmt.Errorf("invalid manifest line: %q", trimmed)
			}
			if err := assignManifestScalar(&manifest, key, value); err != nil {
				return PackManifest{}, err
			}
			continue
		}

		if currentSection == "" {
			return PackManifest{}, fmt.Errorf("invalid manifest indentation: %q", trimmed)
		}

		if strings.HasPrefix(trimmed, "- ") {
			item := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			switch currentList {
			case "requires":
				manifest.Requires = append(manifest.Requires, item)
			case "validation":
				manifest.Validation = append(manifest.Validation, item)
			default:
				return PackManifest{}, fmt.Errorf("unexpected list item in %s: %q", currentSection, trimmed)
			}
			continue
		}

		key, value, ok := splitManifestKeyValue(trimmed)
		if !ok {
			return PackManifest{}, fmt.Errorf("invalid manifest line: %q", trimmed)
		}

		switch currentSection {
		case "folders":
			assignManifestFolder(&manifest, key, value)
		case "rules":
			if err := assignManifestRule(&manifest, key, value); err != nil {
				return PackManifest{}, err
			}
		default:
			return PackManifest{}, fmt.Errorf("unexpected nested manifest entry in %s", currentSection)
		}
	}

	return manifest, nil
}

func splitManifestKeyValue(line string) (string, string, bool) {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	key := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])
	value = strings.Trim(value, "\"")
	value = strings.Trim(value, "'")
	return key, value, true
}

func assignManifestScalar(manifest *PackManifest, key string, value string) error {
	switch key {
	case "pack_id":
		manifest.PackID = value
	case "name":
		manifest.Name = value
	case "version":
		if value == "" {
			manifest.Version = 0
			return nil
		}
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid version: %w", err)
		}
		manifest.Version = parsed
	case "created_by":
		manifest.CreatedBy = value
	case "entrypoint":
		manifest.Entrypoint = value
	case "execution_mode":
		manifest.ExecutionMode = value
	default:
		return fmt.Errorf("unsupported manifest key: %s", key)
	}
	return nil
}

func assignManifestFolder(manifest *PackManifest, key string, value string) {
	switch key {
	case "inbox":
		manifest.Folders.Inbox = value
	case "scripts":
		manifest.Folders.Scripts = value
	case "task_scripts":
		manifest.Folders.TaskScripts = value
	case "snippets":
		manifest.Folders.Snippets = value
	}
}

func assignManifestRule(manifest *PackManifest, key string, value string) error {
	switch key {
	case "max_allowed_files":
		if strings.TrimSpace(value) == "" {
			return nil
		}
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid max_allowed_files: %w", err)
		}
		manifest.Rules.MaxAllowedFiles = parsed
	case "require_validation":
		manifest.Rules.RequireValidation = parseManifestBool(value)
	case "require_branch":
		manifest.Rules.RequireBranch = parseManifestBool(value)
	case "stop_on_failure":
		manifest.Rules.StopOnFailure = parseManifestBool(value)
	}
	return nil
}

func parseManifestBool(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "true")
}

func requireFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("required file missing: %s", path)
		}
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("expected file, found directory: %s", path)
	}
	return nil
}

func requireDir(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("required directory missing: %s", path)
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("expected directory, found file: %s", path)
	}
	return nil
}

func normalizeRelativeDir(path string) string {
	cleaned := filepath.ToSlash(filepath.Clean(path))
	return strings.TrimSuffix(cleaned, "/")
}
