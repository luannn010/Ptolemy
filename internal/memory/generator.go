package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type Generator interface {
	Generate(ctx context.Context, q Query, ctxBody PromptContext) (Answer, error)
}

type OpenAIGenerator struct {
	BaseURL string
	Model   string
	APIKey  string
	Client  *http.Client
}

func NewOpenAIGenerator(baseURL, model, apiKey string) *OpenAIGenerator {
	return &OpenAIGenerator{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Model:   model,
		APIKey:  apiKey,
		// A bounded timeout so a hung/overloaded BRAIN fails the call instead of
		// blocking the caller indefinitely (http.DefaultClient has no timeout).
		// Generous (180s) to tolerate a cold model load on the first call.
		Client: &http.Client{Timeout: 180 * time.Second},
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Grammar  string        `json:"grammar,omitempty"`
	// MaxTokens caps generation. Extraction/consolidation/answer all produce
	// short, bounded output; without a cap a runaway model can generate for
	// minutes. Omitted from JSON when zero.
	MaxTokens int `json:"max_tokens,omitempty"`
	// ChatTemplateKwargs carries enable_thinking=false. Qwen3.x is a reasoning
	// model that, left on, emits a hidden <think> block before the answer —
	// pure latency for the mechanical JSON tasks here (measured ~10x slower).
	// llama.cpp / vLLM forward this into the chat template.
	ChatTemplateKwargs map[string]any `json:"chat_template_kwargs,omitempty"`
}

// noThinking is the kwargs map disabling the reasoning pass for every memory LLM
// call. Centralized so Generate and Complete stay in sync.
func noThinking() map[string]any { return map[string]any{"enable_thinking": false} }

// Generation token caps. Memory LLM outputs are small: extraction is a short
// JSON array, an answer is a few sentences. These are headroom, not targets.
const (
	maxAnswerTokens  = 1024
	maxExtractTokens = 1024
)

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

var citationRe = regexp.MustCompile(`\[source:([^\]]+)\]`)

func (g *OpenAIGenerator) Generate(ctx context.Context, q Query, pc PromptContext) (Answer, error) {
	reqBody := chatRequest{
		Model: g.Model,
		Messages: []chatMessage{
			{Role: "system", Content: pc.System},
			{Role: "user", Content: pc.User},
		},
		MaxTokens:          maxAnswerTokens,
		ChatTemplateKwargs: noThinking(),
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return Answer{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Answer{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if g.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+g.APIKey)
	}
	resp, err := g.Client.Do(req)
	if err != nil {
		return Answer{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		msg, _ := io.ReadAll(resp.Body)
		return Answer{}, fmt.Errorf("llm server %d: %s", resp.StatusCode, string(msg))
	}
	var parsed chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return Answer{}, err
	}
	if len(parsed.Choices) == 0 {
		return Answer{}, fmt.Errorf("llm returned no choices")
	}
	text := parsed.Choices[0].Message.Content
	matches := citationRe.FindAllStringSubmatch(text, -1)
	allowed := map[string]bool{}
	for _, id := range pc.SourceIDs {
		allowed[id] = true
	}
	var cites []string
	seen := map[string]bool{}
	for _, m := range matches {
		id := m[1]
		if !allowed[id] || seen[id] {
			continue
		}
		seen[id] = true
		cites = append(cites, id)
	}
	return Answer{Text: text, Citations: cites}, nil
}

// OpenAIGenerator is the production ChatClient (compile-time guarantee).
var _ ChatClient = (*OpenAIGenerator)(nil)

// Complete is a raw chat completion (no citation parsing). It satisfies the
// ChatClient interface the Extractor and Consolidator depend on.
func (g *OpenAIGenerator) Complete(ctx context.Context, system, user string, opts CompleteOptions) (string, error) {
	reqBody := chatRequest{
		Model: g.Model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Grammar:            opts.Grammar,
		MaxTokens:          maxExtractTokens,
		ChatTemplateKwargs: noThinking(),
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if g.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+g.APIKey)
	}
	resp, err := g.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		msg, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("llm server %d: %s", resp.StatusCode, string(msg))
	}
	var parsed chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("llm returned no choices")
	}
	return parsed.Choices[0].Message.Content, nil
}
