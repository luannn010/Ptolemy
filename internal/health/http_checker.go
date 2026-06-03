package health

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// httpChecker probes an OpenAI-compatible or MCP HTTP endpoint with a GET and
// treats any 2xx as healthy. An empty baseURL means "not configured": a required
// checker reports down, an optional one reports disabled.
type httpChecker struct {
	name     string
	baseURL  string
	path     string
	required bool
	client   *http.Client
}

// NewHTTPChecker builds a GET-based liveness checker. Used for Brain and Embedder
// (path "/v1/models") and MCP (path "/health").
func NewHTTPChecker(name, baseURL, path string, required bool) Checker {
	return &httpChecker{
		name:     name,
		baseURL:  baseURL,
		path:     path,
		required: required,
		client:   &http.Client{},
	}
}

func (h *httpChecker) Name() string { return h.name }

func (h *httpChecker) Check(ctx context.Context) Check {
	c := Check{Name: h.name, Required: h.required}
	if h.baseURL == "" {
		if h.required {
			c.Status = StatusDown
			c.Error = "not configured"
		} else {
			c.Status = StatusDisabled
		}
		return c
	}
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.baseURL+h.path, nil)
	if err != nil {
		c.Status = StatusDown
		c.Error = err.Error()
		return c
	}
	resp, err := h.client.Do(req)
	c.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		c.Status = StatusDown
		c.Error = err.Error()
		return c
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.Status = StatusDown
		c.Error = fmt.Sprintf("status %d", resp.StatusCode)
		return c
	}
	c.Status = StatusUp
	return c
}
