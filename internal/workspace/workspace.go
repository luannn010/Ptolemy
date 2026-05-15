package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var ErrNoActiveSession = errors.New("no active session selected")

type ActiveSession struct {
	SessionID string    `json:"session_id"`
	UpdatedAt time.Time `json:"updated_at"`
}

func Resolve(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		path = wd
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspace is not a directory: %s", path)
	}
	return filepath.Clean(abs), nil
}

func SessionFilePath(workspace string) string {
	return filepath.Join(workspace, ".ptolemy", "session.json")
}

func ReadActiveSession(workspace string) (ActiveSession, error) {
	path := SessionFilePath(workspace)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ActiveSession{}, ErrNoActiveSession
		}
		return ActiveSession{}, err
	}

	var session ActiveSession
	if err := json.Unmarshal(data, &session); err != nil {
		return ActiveSession{}, err
	}
	if strings.TrimSpace(session.SessionID) == "" {
		return ActiveSession{}, ErrNoActiveSession
	}
	return session, nil
}

func WriteActiveSession(workspace string, sessionID string) (ActiveSession, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ActiveSession{}, fmt.Errorf("session ID is required")
	}
	if err := os.MkdirAll(filepath.Join(workspace, ".ptolemy"), 0o755); err != nil {
		return ActiveSession{}, err
	}

	session := ActiveSession{
		SessionID: sessionID,
		UpdatedAt: time.Now().UTC(),
	}
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return ActiveSession{}, err
	}
	data = append(data, '\n')
	if err := os.WriteFile(SessionFilePath(workspace), data, 0o644); err != nil {
		return ActiveSession{}, err
	}
	return session, nil
}
