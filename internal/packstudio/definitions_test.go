package packstudio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWritePackAndDiscoverIt(t *testing.T) {
	root := t.TempDir()

	detail, err := WritePack(root, CreatePackInput{
		PackID:            "demo-pack",
		Name:              "Demo Pack",
		Description:       "Lightweight pack created from tests.",
		Goal:              "Generate a valid runtime-compatible task pack.",
		CreatedBy:         "tests",
		Requires:          []string{"git"},
		Validation:        []string{"go test ./..."},
		MaxAllowedFiles:   4,
		RequireValidation: true,
		RequireBranch:     true,
		StopOnFailure:     true,
		Tasks: []PackTaskInput{
			{
				ID:           "discover-context",
				Title:        "Discover context",
				Summary:      "Inspect the current implementation and outline the work.",
				AllowedFiles: []string{"internal/packstudio"},
				Validation:   []string{"go test ./internal/packstudio"},
			},
			{
				ID:           "ship-monitor",
				Title:        "Ship monitor",
				Summary:      "Build the first run monitor UI.",
				DependsOn:    []string{"discover-context"},
				AllowedFiles: []string{"internal/httpapi", "internal/packstudio"},
				Validation:   []string{"go test ./internal/httpapi"},
			},
		},
	})
	if err != nil {
		t.Fatalf("WritePack returned error: %v", err)
	}

	if !detail.Valid {
		t.Fatalf("expected generated pack to be valid, got validation errors: %v", detail.ValidationErrors)
	}
	if got := len(detail.Tasks); got != 2 {
		t.Fatalf("expected 2 generated tasks, got %d", got)
	}
	if !strings.Contains(detail.Readme, "Demo Pack") {
		t.Fatalf("expected README content in pack detail, got %q", detail.Readme)
	}
	if len(detail.Tasks[0].Checklist) == 0 {
		t.Fatal("expected generated checklist items")
	}

	manifestPath := filepath.Join(root, packsDir, "demo-pack", "PACK_MANIFEST.yaml")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("failed to read generated manifest: %v", err)
	}
	manifest := string(manifestData)
	for _, snippet := range []string{
		"pack_id: demo-pack",
		"name: Demo Pack",
		"entrypoint: TASK_PLAN.md",
		"task_scripts: task-scripts",
		"execution_mode: sequential_first",
	} {
		if !strings.Contains(manifest, snippet) {
			t.Fatalf("expected manifest to contain %q, got:\n%s", snippet, manifest)
		}
	}

	taskData, err := os.ReadFile(detail.Tasks[0].Path)
	if err != nil {
		t.Fatalf("failed to read generated task file: %v", err)
	}
	taskText := string(taskData)
	for _, snippet := range []string{
		"parent_task: null",
		"owner: unassigned",
		"created_by: pack-studio",
		"## Checklist",
		"- [ ] Task planned",
	} {
		if !strings.Contains(taskText, snippet) {
			t.Fatalf("expected generated task file to contain %q, got:\n%s", snippet, taskText)
		}
	}

	packs, err := ListPacks(root)
	if err != nil {
		t.Fatalf("ListPacks returned error: %v", err)
	}
	if len(packs) != 1 || packs[0].ID != "demo-pack" {
		t.Fatalf("expected generated pack in catalog, got %+v", packs)
	}
}

func TestWriteProgramAndValidatePackReferences(t *testing.T) {
	root := t.TempDir()

	for _, packID := range []string{"pack-a", "pack-b"} {
		_, err := WritePack(root, CreatePackInput{
			PackID:            packID,
			Name:              strings.ToUpper(packID),
			Goal:              "Exercise program generation.",
			CreatedBy:         "tests",
			MaxAllowedFiles:   2,
			RequireValidation: true,
			RequireBranch:     true,
			StopOnFailure:     true,
			Tasks: []PackTaskInput{
				{
					ID:           packID + "-task",
					Title:        "Task for " + packID,
					Summary:      "Keep the test pack valid.",
					AllowedFiles: []string{"internal/packstudio"},
				},
			},
		})
		if err != nil {
			t.Fatalf("WritePack(%s) returned error: %v", packID, err)
		}
	}

	definition, err := WriteProgram(root, CreateProgramInput{
		ProgramID:   "demo-program",
		Name:        "Demo Program",
		Description: "Group multiple packs into one sequential run.",
		Packs: []ProgramPackRef{
			{PackID: "pack-a"},
			{PackID: "pack-b", DependsOn: []string{"pack-a"}},
		},
	})
	if err != nil {
		t.Fatalf("WriteProgram returned error: %v", err)
	}
	if definition.ID != "demo-program" {
		t.Fatalf("expected program id demo-program, got %q", definition.ID)
	}

	loaded, validationErrs, err := GetProgram(root, "demo-program")
	if err != nil {
		t.Fatalf("GetProgram returned error: %v", err)
	}
	if len(validationErrs) != 0 {
		t.Fatalf("expected no validation errors, got %v", validationErrs)
	}
	if got := len(loaded.Packs); got != 2 {
		t.Fatalf("expected 2 packs in program, got %d", got)
	}
	if got := loaded.Packs[1].DependsOn; len(got) != 1 || got[0] != "pack-a" {
		t.Fatalf("expected second pack to depend on pack-a, got %+v", got)
	}

	programs, err := ListPrograms(root)
	if err != nil {
		t.Fatalf("ListPrograms returned error: %v", err)
	}
	if len(programs) != 1 || !programs[0].Valid {
		t.Fatalf("expected valid generated program in catalog, got %+v", programs)
	}
}
