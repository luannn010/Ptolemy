package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/luannn010/ptolemy/internal/navigator"
	"github.com/luannn010/ptolemy/internal/worker"
	"github.com/luannn010/ptolemy/internal/workspace"
)

type Config struct {
	Args    []string
	Version string
	Stdout  io.Writer
	Stderr  io.Writer
	Getenv  func(string) string
	Workdir string
}

func Run(ctx context.Context, cfg Config) error {
	if cfg.Stdout == nil {
		cfg.Stdout = os.Stdout
	}
	if cfg.Stderr == nil {
		cfg.Stderr = os.Stderr
	}
	if cfg.Getenv == nil {
		cfg.Getenv = os.Getenv
	}
	if cfg.Args == nil {
		cfg.Args = os.Args[1:]
	}
	if cfg.Version == "" {
		cfg.Version = "dev"
	}

	root, err := workspace.Resolve(cfg.Workdir)
	if err != nil {
		return err
	}

	if len(cfg.Args) == 0 {
		printUsage(cfg.Stdout)
		return nil
	}

	switch cfg.Args[0] {
	case "--version", "-version", "version":
		fmt.Fprintln(cfg.Stdout, cfg.Version)
	case "--workspace", "-workspace", "workspace":
		fmt.Fprintln(cfg.Stdout, root)
	case "kb-build":
		return runKBBuild(cfg.Stdout, root)
	case "kb-update":
		return runKBUpdate(cfg.Stdout, root)
	case "kb-reset":
		return runKBReset(cfg.Stdout, root)
	case "health":
		return runHealth(ctx, cfg.Stdout, cfg.Getenv)
	case "session":
		return runSession(ctx, cfg.Stdout, cfg.Getenv, root, cfg.Args[1:])
	case "help", "--help", "-h":
		printUsage(cfg.Stdout)
	default:
		return fmt.Errorf("unknown command %q", cfg.Args[0])
	}
	return nil
}

func runKBBuild(stdout io.Writer, root string) error {
	result, err := navigator.IndexWorkspace(root)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "KB built for %s\n", result.Workspace)
	fmt.Fprintf(stdout, "files indexed: %d\n", result.FileCount)
	return nil
}

func runKBUpdate(stdout io.Writer, root string) error {
	result, err := navigator.UpdateKnowledgeBase(root, nil, "", nil, "")
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "KB updated for %s\n", result.Workspace)
	fmt.Fprintf(stdout, "updated files: %d\n", len(result.UpdatedFiles))
	fmt.Fprintf(stdout, "removed files: %d\n", len(result.RemovedFiles))
	return nil
}

func runKBReset(stdout io.Writer, root string) error {
	result, err := navigator.ResetKnowledgeBase(root)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "KB reset for %s\n", result.Workspace)
	fmt.Fprintf(stdout, "updated files: %d\n", len(result.UpdatedFiles))
	return nil
}

func runHealth(ctx context.Context, stdout io.Writer, getenv func(string) string) error {
	baseURL := worker.WorkerURLFromEnv(getenv)
	client := worker.NewClient(baseURL)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	result, err := client.Health(ctx)
	if err != nil {
		return fmt.Errorf("worker health check failed at %s: %w", baseURL, err)
	}
	fmt.Fprintf(stdout, "worker: %s\n", cleanStatus(result.Status))
	if strings.TrimSpace(result.Service) != "" {
		fmt.Fprintf(stdout, "service: %s\n", result.Service)
	}
	fmt.Fprintf(stdout, "url: %s\n", baseURL)
	return nil
}

func runSession(ctx context.Context, stdout io.Writer, getenv func(string) string, root string, args []string) error {
	if len(args) == 0 {
		active, err := workspace.ReadActiveSession(root)
		if err != nil {
			if errors.Is(err, workspace.ErrNoActiveSession) {
				return fmt.Errorf("no active session selected in %s; set one with `ptolemy session <sessionID>`", workspace.SessionFilePath(root))
			}
			return err
		}
		fmt.Fprintln(stdout, active.SessionID)
		return nil
	}

	if len(args) == 1 && args[0] == "ps" {
		return listSessions(ctx, stdout, getenv)
	}

	if len(args) != 1 {
		return fmt.Errorf("usage: ptolemy session [ps|<sessionID>]")
	}

	active, err := workspace.WriteActiveSession(root, args[0])
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "active session: %s\n", active.SessionID)
	fmt.Fprintf(stdout, "stored: %s\n", workspace.SessionFilePath(root))
	return nil
}

func listSessions(ctx context.Context, stdout io.Writer, getenv func(string) string) error {
	baseURL := worker.WorkerURLFromEnv(getenv)
	client := worker.NewClient(baseURL)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	sessions, err := client.ListSessions(ctx)
	if err != nil {
		return fmt.Errorf("could not list sessions from %s: %w", baseURL, err)
	}
	if len(sessions) == 0 {
		fmt.Fprintln(stdout, "no sessions found")
		return nil
	}

	table := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "ID\tNAME\tCREATED\tSTATUS\tWORKSPACE")
	for _, session := range sessions {
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\n",
			session.ID,
			session.Name,
			session.CreatedAt,
			session.Status,
			session.Workspace,
		)
	}
	return table.Flush()
}

func cleanStatus(status string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		return "unknown"
	}
	return status
}

func printUsage(stdout io.Writer) {
	fmt.Fprintln(stdout, `Usage:
  ptolemy --version
  ptolemy --workspace
  ptolemy kb-build
  ptolemy kb-update
  ptolemy kb-reset
  ptolemy health
  ptolemy session
  ptolemy session <sessionID>
  ptolemy session ps`)
}
