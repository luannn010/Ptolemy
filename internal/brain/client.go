package brain

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	model      string
	timeout    time.Duration
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
	timeout := loadBrainTimeout()
	return &Client{
		baseURL: baseURL,
		model:   model,
		timeout: timeout,
		httpClient: &http.Client{
			Timeout: timeout,
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
		if isTimeoutError(err) {
			return "", fmt.Errorf(
				"brain request timed out after %s; reduce prompt size, increase timeout, or use a faster model",
				formatTimeout(c.effectiveTimeout(ctx)),
			)
		}
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

func (c *Client) Timeout() string {
	return formatTimeout(c.timeout)
}

func loadBrainTimeout() time.Duration {
	value := strings.TrimSpace(os.Getenv("PTOLEMY_BRAIN_TIMEOUT_SECONDS"))
	if value == "" {
		return 180 * time.Second
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return 180 * time.Second
	}
	return time.Duration(seconds) * time.Second
}

func (c *Client) effectiveTimeout(ctx context.Context) time.Duration {
	timeout := c.timeout
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 && remaining < timeout {
			return remaining
		}
	}
	return timeout
}

func isTimeoutError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func formatTimeout(timeout time.Duration) string {
	if timeout%time.Second == 0 {
		return fmt.Sprintf("%ds", int(timeout/time.Second))
	}
	return timeout.String()
}
