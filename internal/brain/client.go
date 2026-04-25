package brain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature"`
}

type ChatResponse struct {
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func NewClient(baseURL, model string) *Client {
	return &Client{
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (c *Client) Chat(ctx context.Context, messages []Message) (string, error) {
	body := ChatRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: 0.2,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/v1/chat/completions",
		bytes.NewReader(data),
	)
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("brain request failed with status %s", resp.Status)
	}

	var parsed ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", err
	}

	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("brain returned no choices")
	}

	return parsed.Choices[0].Message.Content, nil
}
