// Command ptolemy is the unified developer CLI. It groups the memory and policy
// subcommands behind one binary:
//
//	ptolemy memory recall|capture|demo|eval|synth-eval
//	ptolemy policy check
//
// Each leaf is implemented in internal/cli/{memory,policy} as a
// Run*(ctx, args, stdout, stderr) error function; this dispatcher owns process
// exit so deferred cleanup (e.g. closing the DB connection) runs. The
// ptolemy-memory binary is a thin alias over the same memory recall/capture
// leaves so existing Claude Code hooks keep working.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	// autoload reads .env once for every subcommand so memory.LoadConfig()
	// picks up DATABASE_URL, EMBEDDING_*, BRAIN_* without the caller having to
	// export them. Kept at the entrypoint; internal/cli/* stay library-clean.
	_ "github.com/joho/godotenv/autoload"

	climemory "github.com/luannn010/ptolemy/internal/cli/memory"
	clipolicy "github.com/luannn010/ptolemy/internal/cli/policy"
)

type leaf func(ctx context.Context, args []string, stdout, stderr io.Writer) error

var groups = map[string]map[string]leaf{
	"memory": {
		"recall":     climemory.RunRecall,
		"capture":    climemory.RunCapture,
		"demo":       climemory.RunDemo,
		"eval":       climemory.RunEval,
		"synth-eval": climemory.RunSynthEval,
	},
	"policy": {
		"check": clipolicy.RunCheck,
	},
}

func main() {
	os.Exit(dispatch(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

// dispatch routes `ptolemy <group> <subcommand> [args...]` to the matching
// leaf and maps the result to an exit code: 0 on success, 1 when an eval
// reports failures (ErrEvalFailed), 2 for usage/runtime errors.
func dispatch(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		usage(stderr)
		return 2
	}
	subs, ok := groups[args[0]]
	if !ok {
		fmt.Fprintf(stderr, "unknown group %q\n", args[0])
		usage(stderr)
		return 2
	}
	if len(args) < 2 {
		fmt.Fprintf(stderr, "group %q needs a subcommand\n", args[0])
		usageGroup(stderr, args[0], subs)
		return 2
	}
	run, ok := subs[args[1]]
	if !ok {
		fmt.Fprintf(stderr, "unknown %s subcommand %q\n", args[0], args[1])
		usageGroup(stderr, args[0], subs)
		return 2
	}
	if err := run(ctx, args[2:], stdout, stderr); err != nil {
		fmt.Fprintln(stderr, err)
		if errors.Is(err, climemory.ErrEvalFailed) {
			return 1
		}
		return 2
	}
	return 0
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "usage: ptolemy <group> <subcommand> [args...]")
	fmt.Fprintln(w, "  memory recall|capture|demo|eval|synth-eval")
	fmt.Fprintln(w, "  policy check")
}

func usageGroup(w io.Writer, group string, subs map[string]leaf) {
	fmt.Fprintf(w, "usage: ptolemy %s <subcommand>\n", group)
	for name := range subs {
		fmt.Fprintf(w, "  %s\n", name)
	}
}
