package main

import (
	"os"
	"strings"

	"github.com/luannn010/ptolemy/internal/mcp"
	"github.com/luannn010/ptolemy/internal/mcp/remotetools"
)

func main() {
	baseURL := strings.TrimSpace(os.Getenv("PTOLEMY_BASE_URL"))
	if baseURL == "" {
		baseURL = "http://127.0.0.1:8080"
	}

	client := mcp.NewWorkerClient(baseURL)

	server := mcp.NewServer(
		client,
		remotetools.Tools(),
	)

	server.RegisterHandler(remotetools.Handle)

	server.Run(os.Stdin, os.Stdout)
}
