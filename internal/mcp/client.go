package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type WorkerClient struct {
	BaseURL string
}

func NewWorkerClient(baseURL string) *WorkerClient {
	return &WorkerClient{BaseURL: baseURL}
}

func (c *WorkerClient) Post(path string, payload any) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(c.BaseURL+path, "application/json", bytes.NewReader(data))
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
	resp, err := http.Get(c.BaseURL + path)
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
