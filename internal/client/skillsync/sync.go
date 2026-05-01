package skillsync

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var ErrServerUnavailable = errors.New("skill server unavailable")

type ClientConfig struct {
	ServerURL   string
	SkillsCache string
}

type Skill struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Content string `json:"content"`
}

type SyncResult struct {
	Downloaded []string
	Skipped    []string
}

func LoadClientConfig(workspace string) (ClientConfig, error) {
	path := filepath.Join(workspace, ".ptolemy", "client.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return ClientConfig{}, err
	}

	cfg := ClientConfig{
		ServerURL:   "http://localhost:8080",
		SkillsCache: ".ptolemy/cache/skills",
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), "\"'")

		switch key {
		case "server_url":
			if value != "" {
				cfg.ServerURL = value
			}
		case "skills_cache":
			if value != "" {
				cfg.SkillsCache = value
			}
		}
	}

	return cfg, nil
}

func ListSkills(workspace string) ([]Skill, error) {
	cfg, err := LoadClientConfig(workspace)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	url := strings.TrimRight(cfg.ServerURL, "/") + "/skills"
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrServerUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("skills list request failed: %s %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var skills []Skill
	if err := json.NewDecoder(resp.Body).Decode(&skills); err != nil {
		return nil, err
	}
	return skills, nil
}

func SyncSkills(workspace string, selected []string) (SyncResult, error) {
	cfg, err := LoadClientConfig(workspace)
	if err != nil {
		return SyncResult{}, err
	}

	list, err := ListSkills(workspace)
	if err != nil {
		return SyncResult{}, err
	}

	want := map[string]bool{}
	for _, id := range selected {
		trimmed := strings.TrimSpace(id)
		if trimmed != "" {
			want[trimmed] = true
		}
	}

	cacheDir := cfg.SkillsCache
	if !filepath.IsAbs(cacheDir) {
		cacheDir = filepath.Join(workspace, cacheDir)
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return SyncResult{}, err
	}

	result := SyncResult{
		Downloaded: []string{},
		Skipped:    []string{},
	}

	for _, skill := range list {
		if len(want) > 0 && !want[skill.ID] {
			result.Skipped = append(result.Skipped, skill.ID)
			continue
		}

		if strings.TrimSpace(skill.Content) == "" {
			full, err := fetchSkill(cfg.ServerURL, skill.ID)
			if err != nil {
				return result, err
			}
			skill = full
		}

		target := filepath.Join(cacheDir, sanitizeSkillID(skill.ID)+".md")
		if err := os.WriteFile(target, []byte(skill.Content), 0o644); err != nil {
			return result, err
		}
		result.Downloaded = append(result.Downloaded, skill.ID)
	}

	return result, nil
}

func fetchSkill(serverURL string, skillID string) (Skill, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	url := strings.TrimRight(serverURL, "/") + "/skills/" + skillID
	resp, err := client.Get(url)
	if err != nil {
		return Skill{}, fmt.Errorf("%w: %v", ErrServerUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return Skill{}, fmt.Errorf("skill fetch failed for %s: %s %s", skillID, resp.Status, strings.TrimSpace(string(body)))
	}

	var skill Skill
	if err := json.NewDecoder(resp.Body).Decode(&skill); err != nil {
		return Skill{}, err
	}
	return skill, nil
}

func sanitizeSkillID(id string) string {
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-")
	return replacer.Replace(strings.TrimSpace(id))
}
