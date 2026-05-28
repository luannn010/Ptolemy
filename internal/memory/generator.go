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
		Client:  http.DefaultClient,
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}
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
func (g *OpenAIGenerator) Complete(ctx context.Context, system, user string) (string, error) {
	reqBody := chatRequest{
		Model: g.Model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
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
