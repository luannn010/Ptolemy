// Package policy holds the `ptolemy policy` CLI subcommands. `check` exercises
// the policy engine against a handful of sample commands and prints each
// verdict — a quick way to eyeball the loaded ruleset.
package policy

import (
	"context"
	"fmt"
	"io"

	"github.com/luannn010/ptolemy/internal/domain"
	"github.com/luannn010/ptolemy/internal/policy"
)

// policyPath is the ruleset the check command authorizes against. The binary
// runs from the repo root, so the relative path resolves there.
const policyPath = "./.ptolemy/policy.json"

// sampleCommands are representative intents spanning allow/ask/deny so the
// printed table shows each verdict class.
var sampleCommands = []string{
	"go test ./...",
	"git push origin main",
	"git push --force origin main",
	"cat ./.env",
}

// RunCheck loads the policy ruleset from disk and prints the verdict for each
// sample command. It takes no flags; args are ignored beyond a usage guard.
func RunCheck(_ context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) > 0 {
		fmt.Fprintln(stderr, "usage: ptolemy policy check")
		return fmt.Errorf("policy check takes no arguments, got %d", len(args))
	}
	eng := policy.NewEngine(policy.LoadRuleset(policyPath))
	return runCheck(stdout, eng, sampleCommands)
}

// runCheck authorizes each sample against the engine and writes one verdict
// line per command to out. Split from RunCheck so it can be tested with an
// injected engine, independent of the working directory.
func runCheck(out io.Writer, eng *policy.Engine, samples []string) error {
	for _, cmd := range samples {
		decision := eng.Authorize(domain.Intent{
			Kind:    "command.exec",
			Program: "shell",
			Args:    []string{cmd},
		})
		fmt.Fprintf(out, "%-28s => %s", cmd, decision.Effect)
		if decision.Channel != "" {
			fmt.Fprintf(out, " (%s)", decision.Channel)
		}
		fmt.Fprintf(out, " [%s]\n", decision.RuleID)
	}
	return nil
}
