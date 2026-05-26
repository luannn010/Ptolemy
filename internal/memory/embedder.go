package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

type OpenAIEmbedder struct {
	BaseURL string
	Model   string
	APIKey  string
	Client  *http.Client
}

func NewOpenAIEmbedder(baseURL, model, apiKey string) *OpenAIEmbedder {
	return &OpenAIEmbedder{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Model:   model,
		APIKey:  apiKey,
		Client:  http.DefaultClient,
	}
}

type embedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}
type embedResponseItem struct {
	Embedding []float32 `json:"embedding"`
}
type embedResponse struct {
	Data []embedResponseItem `json:"data"`
}

func (e *OpenAIEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	body, err := json.Marshal(embedRequest{Model: e.Model, Input: texts})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.BaseURL+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if e.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.APIKey)
	}
	resp, err := e.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		msg, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embedding server %d: %s", resp.StatusCode, string(msg))
	}
	var parsed embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	out := make([][]float32, len(parsed.Data))
	for i, item := range parsed.Data {
		out[i] = item.Embedding
	}
	return out, nil
}
