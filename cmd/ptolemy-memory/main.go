// Command ptolemy-memory is a thin alias over the memory recall/capture
// subcommands for use by Claude Code / Codex hooks (SessionStart recall, Stop
// capture). Hooks are shell commands, not MCP callers, so they need a stable
// binary entry point. The same logic is also reachable as `ptolemy memory
// recall|capture`; this alias preserves the original CLI surface so existing
// hooks keep working unchanged.
//
// Usage:
//
//	ptolemy-memory recall  [--query Q] [--subject S] [--project P] [--k N] [--generate] [--verbose] [--quiet]
//	ptolemy-memory capture --user TEXT --assistant TEXT [--subject S] [--project P] [--session S] [--verbose] [--quiet]
package main

import (
	"context"
	"fmt"
	"os"

	// autoload reads .env at startup so memory.LoadConfig() picks up
	// DATABASE_URL, EMBEDDING_*, BRAIN_* without the hook shell exporting them.
	_ "github.com/joho/godotenv/autoload"

	climemory "github.com/luannn010/ptolemy/internal/cli/memory"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: ptolemy-memory <recall|capture> [flags]")
		os.Exit(2)
	}
	ctx := context.Background()
	var err error
	switch os.Args[1] {
	case "recall":
		err = climemory.RunRecall(ctx, os.Args[2:], os.Stdout, os.Stderr)
	case "capture":
		err = climemory.RunCapture(ctx, os.Args[2:], os.Stdout, os.Stderr)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q (want recall|capture)\n", os.Args[1])
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
