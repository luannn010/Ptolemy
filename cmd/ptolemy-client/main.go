package main

import (
	"flag"
	"fmt"
	"os"

	clientinit "github.com/luannn010/ptolemy/internal/client/init"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ptolemy-client <command>")
	}

	switch args[0] {
	case "init":
		return runInit(args[1:])
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	force := fs.Bool("force", false, "overwrite managed files")
	serverURL := fs.String("server-url", "", "server URL")
	projectName := fs.String("project-name", "", "project name")
	workspace := fs.String("workspace", ".", "workspace directory")
	if err := fs.Parse(args); err != nil {
		return err
	}

	result, err := clientinit.Initialize(clientinit.Options{
		Workspace:   *workspace,
		ServerURL:   *serverURL,
		ProjectName: *projectName,
		Force:       *force,
	})
	if err != nil {
		return err
	}

	fmt.Printf("created: %d\n", len(result.Created))
	fmt.Printf("skipped: %d\n", len(result.Skipped))
	return nil
}
