package fileops

import (
	"errors"
	"fmt"
	"os"
	"strings"

	clientworkspace "github.com/luannn010/ptolemy/internal/client/workspace"
)

var (
	ErrMarkerNotFound     = errors.New("marker not found")
	ErrDeleteNotPermitted = errors.New("delete not permitted")
)

type Options struct {
	Workspace   string
	AllowDelete bool
}

type Client struct {
	guard       clientworkspace.Guard
	allowDelete bool
}

func New(opts Options) (Client, error) {
	guard, err := clientworkspace.New(opts.Workspace)
	if err != nil {
		return Client{}, err
	}
	return Client{
		guard:       guard,
		allowDelete: opts.AllowDelete,
	}, nil
}

func (c Client) Read(path string) (string, error) {
	resolvedPath, err := c.guard.ResolvePath(path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (c Client) Write(path string, content string) error {
	resolvedPath, err := c.guard.ResolvePath(path)
	if err != nil {
		return err
	}
	return os.WriteFile(resolvedPath, []byte(content), 0o644)
}

func (c Client) InsertAfter(path string, marker string, snippet string) error {
	if strings.TrimSpace(marker) == "" {
		return fmt.Errorf("marker is required")
	}
	if strings.TrimSpace(snippet) == "" {
		return fmt.Errorf("snippet is required")
	}

	resolvedPath, err := c.guard.ResolvePath(path)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return err
	}

	content := string(data)
	idx := strings.Index(content, marker)
	if idx == -1 {
		return ErrMarkerNotFound
	}
	insertAt := idx + len(marker)
	insertText := snippet
	if !strings.HasPrefix(insertText, "\n") {
		insertText = "\n" + insertText
	}

	updated := content[:insertAt] + insertText + content[insertAt:]
	return os.WriteFile(resolvedPath, []byte(updated), 0o644)
}

func (c Client) ReplaceBetween(path string, startMarker string, endMarker string, replacement string) error {
	if strings.TrimSpace(startMarker) == "" || strings.TrimSpace(endMarker) == "" {
		return fmt.Errorf("start and end markers are required")
	}

	resolvedPath, err := c.guard.ResolvePath(path)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return err
	}
	content := string(data)

	startIdx := strings.Index(content, startMarker)
	if startIdx == -1 {
		return ErrMarkerNotFound
	}
	searchFrom := startIdx + len(startMarker)
	relativeEndIdx := strings.Index(content[searchFrom:], endMarker)
	if relativeEndIdx == -1 {
		return ErrMarkerNotFound
	}
	endIdx := searchFrom + relativeEndIdx

	updated := content[:searchFrom] + replacement + content[endIdx:]
	return os.WriteFile(resolvedPath, []byte(updated), 0o644)
}

func (c Client) Delete(path string, confirm bool) error {
	if !c.allowDelete && !confirm {
		return ErrDeleteNotPermitted
	}

	resolvedPath, err := c.guard.ResolvePath(path)
	if err != nil {
		return err
	}
	return os.Remove(resolvedPath)
}
