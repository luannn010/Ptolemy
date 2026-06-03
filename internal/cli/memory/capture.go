package memory

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/luannn010/ptolemy/internal/memory"
)

type capturer interface {
	Capture(ctx context.Context, ex memory.Exchange) error
}

// RunCapture is the `ptolemy memory capture` (and `ptolemy-memory capture`)
// subcommand. It opens the memory module and captures one user/assistant
// exchange synchronously.
func RunCapture(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("capture", flag.ContinueOnError)
	fs.SetOutput(stderr)
	user := fs.String("user", "", "user message text")
	assistant := fs.String("assistant", "", "assistant reply text")
	subject := fs.String("subject", "", "owner scope")
	project := fs.String("project", "", "project scope")
	session := fs.String("session", "", "session id")
	verbose := fs.Bool("verbose", false, "extra debug detail")
	quiet := fs.Bool("quiet", false, "errors only")
	if err := fs.Parse(args); err != nil {
		return err
	}

	setupLogging(*verbose, *quiet)

	hook, conn, err := openCapture(ctx)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	sub, prj := scope(*subject, *project)
	ex := memory.Exchange{
		UserText: *user, AssistantText: *assistant,
		SubjectID: sub, ProjectID: prj, SessionID: *session,
	}
	return runCapture(ctx, hook, ex)
}

// runCapture validates the exchange has text and captures it synchronously.
func runCapture(ctx context.Context, c capturer, ex memory.Exchange) error {
	if strings.TrimSpace(ex.UserText) == "" && strings.TrimSpace(ex.AssistantText) == "" {
		return errors.New("provide at least one of --user or --assistant")
	}
	return c.Capture(ctx, ex)
}

func openCapture(ctx context.Context) (*memory.PerTurnCaptureHook, dbConn, error) {
	cfg, err := memory.LoadConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("config: %w", err)
	}
	orch, conn, err := memory.NewModule(ctx, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("module: %w", err)
	}
	hook := memory.NewCaptureHookFromConfig(cfg, orch.Store, orch.Embedder)
	return hook, conn, nil
}
