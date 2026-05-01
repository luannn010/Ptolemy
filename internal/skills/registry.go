package skills

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var ErrSkillNotFound = errors.New("skill not found")

type Skill struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

type SkillDocument struct {
	Skill
	Content string `json:"content"`
}

type Registry struct {
	root string
}

func NewRegistry(baseDir string, configuredDir string) (*Registry, error) {
	root, err := resolveSkillDir(baseDir, configuredDir)
	if err != nil {
		return nil, err
	}
	return &Registry{root: root}, nil
}

func (r *Registry) List() ([]Skill, error) {
	if _, err := os.Stat(r.root); err != nil {
		if os.IsNotExist(err) {
			return []Skill{}, nil
		}
		return nil, err
	}

	skills := make([]Skill, 0)
	err := filepath.WalkDir(r.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(r.root, path)
		if err != nil {
			return err
		}
		normalized := filepath.ToSlash(rel)
		id := strings.TrimSuffix(normalized, filepath.Ext(normalized))
		skills = append(skills, Skill{
			ID:   id,
			Name: filepath.Base(id),
			Path: normalized,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(skills, func(i, j int) bool {
		return skills[i].ID < skills[j].ID
	})
	return skills, nil
}

func (r *Registry) Get(id string) (SkillDocument, error) {
	id = strings.TrimSpace(id)
	if id == "" || hasParentTraversal(id) {
		return SkillDocument{}, ErrSkillNotFound
	}

	skills, err := r.List()
	if err != nil {
		return SkillDocument{}, err
	}

	var match *Skill
	for i := range skills {
		skill := skills[i]
		if skill.ID == id || skill.Name == id || skill.Path == id {
			if match != nil && match.ID != skill.ID {
				return SkillDocument{}, ErrSkillNotFound
			}
			match = &skill
		}
	}
	if match == nil {
		return SkillDocument{}, ErrSkillNotFound
	}

	content, err := os.ReadFile(filepath.Join(r.root, filepath.FromSlash(match.Path)))
	if err != nil {
		if os.IsNotExist(err) {
			return SkillDocument{}, ErrSkillNotFound
		}
		return SkillDocument{}, err
	}

	return SkillDocument{
		Skill:   *match,
		Content: string(content),
	}, nil
}

func resolveSkillDir(baseDir string, configuredDir string) (string, error) {
	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		baseDir = "."
	}

	dir := strings.TrimSpace(configuredDir)
	if dir == "" {
		dir = defaultSkillDir(baseDir)
	}

	if filepath.IsAbs(dir) {
		return filepath.Clean(dir), nil
	}
	if hasParentTraversal(dir) {
		return "", errors.New("skill registry path must stay within the workspace")
	}
	return filepath.Join(baseDir, filepath.Clean(dir)), nil
}

func defaultSkillDir(baseDir string) string {
	primary := filepath.Join(baseDir, "docs", "skills")
	if info, err := os.Stat(primary); err == nil && info.IsDir() {
		return primary
	}
	return filepath.Join(baseDir, ".ptolemy", "server", "skills")
}

func hasParentTraversal(path string) bool {
	cleaned := filepath.ToSlash(filepath.Clean(path))
	for _, part := range strings.Split(cleaned, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}
