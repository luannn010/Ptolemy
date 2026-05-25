package agentloop

import (
	"fmt"
	"strings"

	"github.com/luannn010/ptolemy/internal/brain"
	"github.com/luannn010/ptolemy/internal/navigator"
	"github.com/luannn010/ptolemy/internal/tasks"
)

const (
	MaxContextTokens                   = 24576
	TargetMaxInputTokens               = 8192
	HardMaxInputTokens                 = 9216
	ReserveForReasoningAndOutputTokens = 15360
	DefaultMaxCompletionTokens         = 4096
	MaxCompletionTokens                = 6144
	SafetyBufferTokens                 = 2048
	defaultSummaryContextTokens        = 1600
	defaultMinimumContextBudgetTokens  = 768
	defaultPerContextFileBudgetTokens  = 1024
)

type PromptContext struct {
	Manifest     *tasks.ProcessManifest
	ChildTask    *tasks.ProcessChildTask
	Context      navigator.ContextSelection
	Observations []Observation
}

type PromptAssembly struct {
	Messages             []brain.Message
	EstimatedInputTokens int
	IncludedContext      []string
	TrimmedContext       []string
	PreviousSummaries    []string
}

func assemblePrompt(task tasks.Task, promptContext PromptContext) PromptAssembly {
	messages := BuildMessages(task, promptContext)
	estimated := 0
	for _, message := range messages {
		estimated += tasks.EstimateTokens(message.Content)
	}
	return PromptAssembly{
		Messages:             messages,
		EstimatedInputTokens: estimated,
		IncludedContext:      append([]string{}, promptContext.Context.IncludedPaths...),
		TrimmedContext:       append([]string{}, promptContext.Context.TrimmedPaths...),
	}
}

func buildPromptAssembly(workspace string, task tasks.Task, manifest *tasks.ProcessManifest, child *tasks.ProcessChildTask, summaries []navigator.ContextFile, observations []Observation) (PromptAssembly, error) {
	taskForPrompt := task
	if child != nil {
		taskForPrompt.ID = child.ID
		taskForPrompt.Body = child.Body
		taskForPrompt.AllowedFiles = append([]string{}, child.AllowedFiles...)
		taskForPrompt.Validation = append([]string{}, child.ValidationCommands...)
	}

	basePromptTokens := tasks.EstimateTokens(taskForPrompt.ID) +
		tasks.EstimateTokens(taskForPrompt.Branch) +
		tasks.EstimateTokens(taskForPrompt.Body) +
		tasks.EstimateTokens(strings.Join(taskForPrompt.AllowedFiles, "\n")) +
		tasks.EstimateTokens(renderObservationSummaries(observations)) +
		512

	trimmedSummaries := trimSummaryContext(summaries, defaultSummaryContextTokens)
	summaryTokens := 0
	for _, summary := range trimmedSummaries {
		summaryTokens += tasks.EstimateTokens(summary.Content)
	}

	contextBudget := TargetMaxInputTokens - basePromptTokens - summaryTokens
	if contextBudget < defaultMinimumContextBudgetTokens {
		trimmedSummaries = trimSummaryContext(summaries, 800)
		summaryTokens = 0
		for _, summary := range trimmedSummaries {
			summaryTokens += tasks.EstimateTokens(summary.Content)
		}
		contextBudget = TargetMaxInputTokens - basePromptTokens - summaryTokens
	}
	if contextBudget < defaultMinimumContextBudgetTokens {
		contextBudget = defaultMinimumContextBudgetTokens
	}

	contextSelection, err := navigator.AssembleContext(navigator.ContextRequest{
		Workspace:         workspace,
		TaskBody:          taskForPrompt.Body,
		AllowedFiles:      taskForPrompt.AllowedFiles,
		ReferencedPaths:   referencedContextPaths(taskForPrompt),
		PreviousSummaries: trimmedSummaries,
		BudgetTokens:      contextBudget,
		MaxFileTokens:     defaultPerContextFileBudgetTokens,
	})
	if err != nil {
		return PromptAssembly{}, err
	}

	promptContext := PromptContext{
		Manifest:     manifest,
		ChildTask:    child,
		Context:      contextSelection,
		Observations: observations,
	}

	assembly := assemblePrompt(taskForPrompt, promptContext)
	assembly.PreviousSummaries = contextFilePaths(trimmedSummaries)
	for assembly.EstimatedInputTokens > HardMaxInputTokens && len(promptContext.Context.Files) > 0 {
		last := promptContext.Context.Files[len(promptContext.Context.Files)-1]
		promptContext.Context.Files = promptContext.Context.Files[:len(promptContext.Context.Files)-1]
		promptContext.Context.IncludedPaths = promptContext.Context.IncludedPaths[:len(promptContext.Context.IncludedPaths)-1]
		promptContext.Context.TrimmedPaths = append(promptContext.Context.TrimmedPaths, last.Path)
		assembly = assemblePrompt(taskForPrompt, promptContext)
	}

	if assembly.EstimatedInputTokens > HardMaxInputTokens {
		return PromptAssembly{}, fmt.Errorf(
			"context budget exceeded: estimated input tokens %d exceed hard max %d",
			assembly.EstimatedInputTokens,
			HardMaxInputTokens,
		)
	}

	return assembly, nil
}

func contextFilePaths(items []navigator.ContextFile) []string {
	paths := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item.Path)
		if trimmed == "" {
			continue
		}
		paths = append(paths, trimmed)
	}
	return dedupeReferencedPaths(paths)
}

func trimSummaryContext(items []navigator.ContextFile, tokenBudget int) []navigator.ContextFile {
	if tokenBudget <= 0 || len(items) == 0 {
		return nil
	}
	selected := make([]navigator.ContextFile, 0, len(items))
	used := 0
	for i := len(items) - 1; i >= 0; i-- {
		tokens := tasks.EstimateTokens(items[i].Content)
		if used+tokens > tokenBudget {
			continue
		}
		selected = append(selected, items[i])
		used += tokens
	}
	for i, j := 0, len(selected)-1; i < j; i, j = i+1, j-1 {
		selected[i], selected[j] = selected[j], selected[i]
	}
	return selected
}

func renderObservationSummaries(observations []Observation) string {
	if len(observations) == 0 {
		return ""
	}
	var builder strings.Builder
	for _, observation := range observations {
		builder.WriteString(fmt.Sprintf("- step %d [%s] %s\n", observation.Step, observation.Source, observation.Summary))
	}
	return builder.String()
}

func referencedContextPaths(task tasks.Task) []string {
	paths := append([]string{}, task.Snippets...)
	for _, token := range strings.Fields(task.Body) {
		if strings.HasPrefix(token, ".ptolemy/") && strings.HasSuffix(token, ".md") {
			paths = append(paths, strings.TrimSpace(strings.Trim(token, "`'\"")))
		}
	}
	return dedupeReferencedPaths(paths)
}

func dedupeReferencedPaths(paths []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	return out
}
