package skilltools

import (
	"encoding/json"

	"github.com/luannn010/ptolemy/internal/mcp"
	"github.com/luannn010/ptolemy/internal/mcp/executortools"
	"github.com/luannn010/ptolemy/internal/mcp/filetools"
	"github.com/luannn010/ptolemy/internal/mcp/gittools"
	"github.com/luannn010/ptolemy/internal/mcp/navigatortools"
	"github.com/luannn010/ptolemy/internal/mcp/sessiontools"
	"github.com/luannn010/ptolemy/internal/mcp/worktreetools"
)

type listedSkill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

func Tools() []mcp.Tool {
	return []mcp.Tool{
		mcp.NewTool("ptolemy_list_skills", "List all available Ptolemy MCP skills/tools grouped by category.", map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}),
	}
}

func Handle(name string, _ map[string]any, _ *mcp.WorkerClient) (map[string]any, bool, error) {
	if name != "ptolemy_list_skills" {
		return nil, false, nil
	}

	skills := []listedSkill{}
	skills = appendSkills(skills, "session", sessiontools.Tools())
	skills = appendSkills(skills, "executor", executortools.Tools())
	skills = appendSkills(skills, "file", filetools.Tools())
	skills = appendSkills(skills, "navigator", navigatortools.Tools())
	skills = appendSkills(skills, "git", gittools.Tools())
	skills = appendSkills(skills, "worktree", worktreetools.Tools())

	body, err := json.Marshal(map[string]any{
		"skills": skills,
		"count":  len(skills),
	})
	return mcp.TextResult(body), true, err
}

func appendSkills(existing []listedSkill, category string, tools []mcp.Tool) []listedSkill {
	for _, tool := range tools {
		existing = append(existing, listedSkill{
			Name:        tool.Name,
			Description: tool.Description,
			Category:    category,
		})
	}
	return existing
}
