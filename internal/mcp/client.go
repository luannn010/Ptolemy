package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type WorkerClient struct {
	BaseURL        string
	HTTPClient     *http.Client
	DefaultTimeout time.Duration
	HealthTimeout  time.Duration
}

func NewWorkerClient(baseURL string) *WorkerClient {
	return &WorkerClient{
		BaseURL:        baseURL,
		HTTPClient:     &http.Client{},
		DefaultTimeout: 30 * time.Second,
		HealthTimeout:  10 * time.Second,
	}
}

func (c *WorkerClient) Post(path string, payload any) ([]byte, error) {
	return c.PostWithTimeout(path, payload, c.DefaultTimeout)
}

func (c *WorkerClient) PostWithTimeout(path string, payload any, timeout time.Duration) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("worker error %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func (c *WorkerClient) Get(path string) ([]byte, error) {
	return c.GetWithTimeout(path, c.DefaultTimeout)
}

func (c *WorkerClient) GetWithTimeout(path string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("worker error %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}
