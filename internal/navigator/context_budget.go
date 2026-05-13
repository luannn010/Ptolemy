package navigator

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type ContextRequest struct {
	Workspace         string
	TaskBody          string
	AllowedFiles      []string
	ReferencedPaths   []string
	PreviousSummaries []ContextFile
	BudgetTokens      int
	MaxFileTokens     int
}

type ContextSelection struct {
	Files           []ContextFile
	IncludedPaths   []string
	TrimmedPaths    []string
	EstimatedTokens int
}

type contextCandidate struct {
	Path     string
	Content  string
	Priority int
	Score    int
	Required bool
}

func AssembleContext(req ContextRequest) (ContextSelection, error) {
	root, err := cleanWorkspace(req.Workspace)
	if err != nil {
		return ContextSelection{}, err
	}

	budgetTokens := req.BudgetTokens
	if budgetTokens <= 0 {
		budgetTokens = 4096
	}

	maxFileTokens := req.MaxFileTokens
	if maxFileTokens <= 0 {
		maxFileTokens = 1024
	}

	keywords := contextKeywords(req.TaskBody, req.AllowedFiles, req.ReferencedPaths)
	candidates, err := gatherContextCandidates(root, keywords, req)
	if err != nil {
		return ContextSelection{}, err
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Priority != candidates[j].Priority {
			return candidates[i].Priority < candidates[j].Priority
		}
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		return candidates[i].Path < candidates[j].Path
	})

	selection := ContextSelection{
		Files:         []ContextFile{},
		IncludedPaths: []string{},
		TrimmedPaths:  []string{},
	}
	remaining := budgetTokens

	for _, candidate := range candidates {
		if remaining <= 64 {
			selection.TrimmedPaths = append(selection.TrimmedPaths, candidate.Path)
			continue
		}

		perFileBudget := maxFileTokens
		if candidate.Required && remaining < perFileBudget {
			perFileBudget = remaining
		}
		if perFileBudget > remaining {
			perFileBudget = remaining
		}

		content, truncated := formatContextCandidate(candidate.Path, candidate.Content, keywords, perFileBudget)
		tokens := estimateContextTokens(content)
		if tokens > remaining && !candidate.Required {
			selection.TrimmedPaths = append(selection.TrimmedPaths, candidate.Path)
			continue
		}
		if tokens > remaining {
			content, truncated = formatContextCandidate(candidate.Path, candidate.Content, keywords, remaining)
			tokens = estimateContextTokens(content)
		}
		if tokens == 0 || tokens > remaining {
			selection.TrimmedPaths = append(selection.TrimmedPaths, candidate.Path)
			continue
		}

		selection.Files = append(selection.Files, ContextFile{
			Path:    candidate.Path,
			Content: content,
		})
		selection.IncludedPaths = append(selection.IncludedPaths, candidate.Path)
		selection.EstimatedTokens += tokens
		remaining -= tokens
		if truncated {
			selection.TrimmedPaths = append(selection.TrimmedPaths, candidate.Path)
		}
	}

	selection.IncludedPaths = dedupeContextStrings(selection.IncludedPaths)
	selection.TrimmedPaths = dedupeContextStrings(selection.TrimmedPaths)
	return selection, nil
}

func gatherContextCandidates(root string, keywords []string, req ContextRequest) ([]contextCandidate, error) {
	candidates := []contextCandidate{}
	seen := map[string]bool{}

	addCandidate := func(path string, content string, priority int, score int, required bool) {
		cleanPath := filepath.ToSlash(filepath.Clean(path))
		if seen[cleanPath] || strings.TrimSpace(content) == "" {
			return
		}
		seen[cleanPath] = true
		candidates = append(candidates, contextCandidate{
			Path:     cleanPath,
			Content:  content,
			Priority: priority,
			Score:    score,
			Required: required,
		})
	}

	ptolemyPath := filepath.Join(root, ".ptolemy", "PTOLEMY.md")
	if data, err := os.ReadFile(ptolemyPath); err == nil {
		addCandidate(".ptolemy/PTOLEMY.md", string(data), 1, 1000, true)
	}

	for _, ref := range dedupeContextStrings(req.ReferencedPaths) {
		relPath, data, err := readContextPath(root, ref)
		if err != nil {
			continue
		}
		addCandidate(relPath, data, 2, scoreContextCandidate(relPath, data, keywords)+500, true)
	}

	markdownFiles, err := listKnowledgeBaseMarkdown(root)
	if err != nil {
		return nil, err
	}
	for _, relPath := range markdownFiles {
		if relPath == ".ptolemy/PTOLEMY.md" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relPath)))
		if err != nil {
			return nil, err
		}
		score := scoreContextCandidate(relPath, string(data), keywords)
		priority := 5
		if score > 0 {
			priority = 3
		}
		addCandidate(relPath, string(data), priority, score, false)
	}

	for _, summary := range req.PreviousSummaries {
		addCandidate(summary.Path, summary.Content, 4, scoreContextCandidate(summary.Path, summary.Content, keywords), false)
	}

	return candidates, nil
}

func listKnowledgeBaseMarkdown(root string) ([]string, error) {
	var paths []string
	for _, dir := range []string{
		filepath.Join(root, ".ptolemy", "kb"),
		filepath.Join(root, ".ptolemy", "context"),
	} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
				continue
			}
			rel, err := filepath.Rel(root, filepath.Join(dir, entry.Name()))
			if err != nil {
				return nil, err
			}
			paths = append(paths, filepath.ToSlash(rel))
		}
	}
	sort.Strings(paths)
	return dedupeContextStrings(paths), nil
}

func readContextPath(root string, ref string) (string, string, error) {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return "", "", fmt.Errorf("path is required")
	}
	cleanRef := filepath.Clean(trimmed)
	fullPath := cleanRef
	if !filepath.IsAbs(cleanRef) {
		fullPath = filepath.Join(root, filepath.FromSlash(cleanRef))
	}
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", "", err
	}
	rel, err := filepath.Rel(root, fullPath)
	if err != nil {
		return "", "", err
	}
	return filepath.ToSlash(rel), string(data), nil
}

func scoreContextCandidate(path string, content string, keywords []string) int {
	lowerPath := strings.ToLower(path)
	lowerContent := strings.ToLower(content)
	score := 0
	for _, keyword := range keywords {
		if strings.Contains(lowerPath, keyword) {
			score += 4
		}
		if strings.Contains(lowerContent, keyword) {
			score += 1
		}
	}
	return score
}

func contextKeywords(taskBody string, allowedFiles []string, referencedPaths []string) []string {
	seen := map[string]bool{}
	keywords := []string{}

	add := func(value string) {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || len(value) < 3 || seen[value] {
			return
		}
		seen[value] = true
		keywords = append(keywords, value)
	}

	for _, token := range strings.FieldsFunc(strings.ToLower(taskBody), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '-' && r != '_' && r != '/'
	}) {
		add(token)
	}
	for _, path := range allowedFiles {
		add(filepath.Base(path))
		add(filepath.ToSlash(path))
	}
	for _, path := range referencedPaths {
		add(filepath.Base(path))
		add(filepath.ToSlash(path))
	}

	return keywords
}

func formatContextCandidate(path string, content string, keywords []string, maxTokens int) (string, bool) {
	header := fmt.Sprintf("--- context: %s ---\n", path)
	if maxTokens <= estimateContextTokens(header)+16 {
		return header + "[TRUNCATED: exceeded context budget]\n", true
	}

	if estimateContextTokens(content) <= maxTokens-estimateContextTokens(header) {
		return header + strings.TrimSpace(content) + "\n", false
	}

	var builder strings.Builder
	builder.WriteString(header)

	if outline := headingOutline(content); outline != "" {
		builder.WriteString("Outline:\n")
		builder.WriteString(outline)
		builder.WriteString("\n\n")
	}

	excerpt := relevantContextExcerpt(content, keywords)
	remainingChars := max(64, (maxTokens-estimateContextTokens(builder.String())-8)*4)
	if len(excerpt) > remainingChars {
		excerpt = excerpt[:remainingChars]
	}
	builder.WriteString(strings.TrimSpace(excerpt))
	if !strings.HasSuffix(builder.String(), "\n") {
		builder.WriteString("\n")
	}
	builder.WriteString("[TRUNCATED: exceeded context budget]\n")
	return builder.String(), true
}

func headingOutline(content string) string {
	lines := strings.Split(content, "\n")
	headings := make([]string, 0)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			headings = append(headings, "- "+strings.TrimSpace(strings.TrimLeft(trimmed, "#")))
		}
		if len(headings) >= 12 {
			break
		}
	}
	return strings.Join(headings, "\n")
}

func relevantContextExcerpt(content string, keywords []string) string {
	blocks := strings.Split(content, "\n\n")
	matches := make([]string, 0, len(blocks))
	for _, block := range blocks {
		lower := strings.ToLower(block)
		for _, keyword := range keywords {
			if strings.Contains(lower, keyword) {
				matches = append(matches, strings.TrimSpace(block))
				break
			}
		}
		if len(matches) >= 3 {
			break
		}
	}
	if len(matches) > 0 {
		return strings.Join(matches, "\n\n")
	}
	if len(content) > 1200 {
		return content[:1200]
	}
	return content
}

func estimateContextTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	return (len(text) + 3) / 4
}

func dedupeContextStrings(items []string) []string {
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

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
