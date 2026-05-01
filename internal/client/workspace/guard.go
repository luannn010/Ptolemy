package clientworkspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ErrPathOutsideWorkspace = errors.New("path escapes workspace")

type Guard struct {
	root string
}

func New(root string) (Guard, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Guard{}, fmt.Errorf("resolve workspace root: %w", err)
	}

	cleanRoot := filepath.Clean(absRoot)
	return Guard{root: cleanRoot}, nil
}

func (g Guard) Root() string {
	return g.root
}

func (g Guard) ResolvePath(requestedPath string) (string, error) {
	if strings.TrimSpace(requestedPath) == "" {
		return "", fmt.Errorf("path is required")
	}

	var candidate string
	if filepath.IsAbs(requestedPath) {
		candidate = filepath.Clean(requestedPath)
	} else {
		candidate = filepath.Clean(filepath.Join(g.root, requestedPath))
	}

	if !isWithinRoot(g.root, candidate) {
		return "", fmt.Errorf("%w: %s", ErrPathOutsideWorkspace, requestedPath)
	}

	// Evaluate symlinks for practical escape detection.
	evaluated, err := filepath.EvalSymlinks(candidate)
	if err == nil {
		if !isWithinRoot(g.root, evaluated) {
			return "", fmt.Errorf("%w: %s", ErrPathOutsideWorkspace, requestedPath)
		}
		return evaluated, nil
	}

	if os.IsNotExist(err) {
		parent := filepath.Dir(candidate)
		evaluatedParent, parentErr := filepath.EvalSymlinks(parent)
		if parentErr == nil && !isWithinRoot(g.root, evaluatedParent) {
			return "", fmt.Errorf("%w: %s", ErrPathOutsideWorkspace, requestedPath)
		}
		return candidate, nil
	}

	return "", fmt.Errorf("evaluate path symlinks: %w", err)
}

func isWithinRoot(root string, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}

	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
