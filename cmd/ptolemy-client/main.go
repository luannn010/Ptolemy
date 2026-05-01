package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	clientexec "github.com/luannn010/ptolemy/internal/client/exec"
	clientfileops "github.com/luannn010/ptolemy/internal/client/fileops"
	clientinit "github.com/luannn010/ptolemy/internal/client/init"
	clientscan "github.com/luannn010/ptolemy/internal/client/scan"
	clientskillsync "github.com/luannn010/ptolemy/internal/client/skillsync"
	"github.com/luannn010/ptolemy/internal/shellcmd"
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
	case "file":
		return runFile(args[1:])
	case "exec":
		return runExec(args[1:])
	case "skills":
		return runSkills(args[1:])
	case "sync-skills":
		return runSyncSkills(args[1:])
	case "scan":
		return runScan(args[1:])
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

func runFile(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ptolemy-client file <read|write|insert-after|replace-between|delete>")
	}

	switch args[0] {
	case "read":
		return runFileRead(args[1:])
	case "write":
		return runFileWrite(args[1:])
	case "insert-after":
		return runFileInsertAfter(args[1:])
	case "replace-between":
		return runFileReplaceBetween(args[1:])
	case "delete":
		return runFileDelete(args[1:])
	default:
		return fmt.Errorf("unknown file command: %s", args[0])
	}
}

func runFileRead(args []string) error {
	fs := flag.NewFlagSet("file read", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	workspace := fs.String("workspace", ".", "workspace directory")
	path := fs.String("path", "", "relative path to read")
	if err := fs.Parse(args); err != nil {
		return err
	}
	client, err := clientfileops.New(clientfileops.Options{Workspace: *workspace})
	if err != nil {
		return err
	}
	content, err := client.Read(*path)
	if err != nil {
		return err
	}
	fmt.Print(content)
	return nil
}

func runFileWrite(args []string) error {
	fs := flag.NewFlagSet("file write", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	workspace := fs.String("workspace", ".", "workspace directory")
	path := fs.String("path", "", "relative path to write")
	content := fs.String("content", "", "content to write")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *content == "" {
		bytes, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		*content = string(bytes)
	}
	client, err := clientfileops.New(clientfileops.Options{Workspace: *workspace})
	if err != nil {
		return err
	}
	return client.Write(*path, *content)
}

func runFileInsertAfter(args []string) error {
	fs := flag.NewFlagSet("file insert-after", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	workspace := fs.String("workspace", ".", "workspace directory")
	path := fs.String("path", "", "relative path to update")
	marker := fs.String("marker", "", "marker text")
	content := fs.String("content", "", "snippet to insert")
	if err := fs.Parse(args); err != nil {
		return err
	}
	client, err := clientfileops.New(clientfileops.Options{Workspace: *workspace})
	if err != nil {
		return err
	}
	return client.InsertAfter(*path, *marker, *content)
}

func runFileReplaceBetween(args []string) error {
	fs := flag.NewFlagSet("file replace-between", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	workspace := fs.String("workspace", ".", "workspace directory")
	path := fs.String("path", "", "relative path to update")
	start := fs.String("start", "", "start marker")
	end := fs.String("end", "", "end marker")
	replacement := fs.String("replacement", "", "replacement text")
	if err := fs.Parse(args); err != nil {
		return err
	}
	client, err := clientfileops.New(clientfileops.Options{Workspace: *workspace})
	if err != nil {
		return err
	}
	return client.ReplaceBetween(*path, *start, *end, *replacement)
}

func runFileDelete(args []string) error {
	fs := flag.NewFlagSet("file delete", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	workspace := fs.String("workspace", ".", "workspace directory")
	path := fs.String("path", "", "relative path to delete")
	allowDelete := fs.Bool("allow-delete", false, "allow deletes by config")
	confirm := fs.Bool("confirm", false, "confirm delete action")
	if err := fs.Parse(args); err != nil {
		return err
	}
	client, err := clientfileops.New(clientfileops.Options{
		Workspace:   *workspace,
		AllowDelete: *allowDelete,
	})
	if err != nil {
		return err
	}
	return client.Delete(*path, *confirm)
}

func runExec(args []string) error {
	fs := flag.NewFlagSet("exec", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	workspace := fs.String("workspace", ".", "workspace directory")
	shell := fs.String("shell", shellcmd.DefaultProgram(runtime.GOOS), "shell to execute command")
	timeoutSeconds := fs.Int("timeout-seconds", 120, "command timeout in seconds")
	command := fs.String("command", "", "shell command to run")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *command == "" {
		return fmt.Errorf("command is required")
	}

	runner, err := clientexec.New(clientexec.Options{
		Workspace:      *workspace,
		Shell:          *shell,
		TimeoutSeconds: *timeoutSeconds,
		Policy:         clientexec.AllowAllPolicy,
	})
	if err != nil {
		return err
	}

	result, err := runner.Run(*command)
	if err != nil {
		return err
	}

	fmt.Printf("exit_code: %d\n", result.ExitCode)
	fmt.Printf("timed_out: %t\n", result.TimedOut)
	if result.Stdout != "" {
		fmt.Printf("stdout:\n%s", result.Stdout)
		if result.Stdout[len(result.Stdout)-1] != '\n' {
			fmt.Print("\n")
		}
	}
	if result.Stderr != "" {
		fmt.Printf("stderr:\n%s", result.Stderr)
		if result.Stderr[len(result.Stderr)-1] != '\n' {
			fmt.Print("\n")
		}
	}

	return nil
}

func runSkills(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ptolemy-client skills list [--workspace .]")
	}
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("skills list", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		workspace := fs.String("workspace", ".", "workspace directory")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		skills, err := clientskillsync.ListSkills(*workspace)
		if err != nil {
			return err
		}
		for _, skill := range skills {
			if strings.TrimSpace(skill.Name) != "" {
				fmt.Printf("%s\t%s\n", skill.ID, skill.Name)
			} else {
				fmt.Println(skill.ID)
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown skills command: %s", args[0])
	}
}

func runSyncSkills(args []string) error {
	fs := flag.NewFlagSet("sync-skills", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	workspace := fs.String("workspace", ".", "workspace directory")
	skillsArg := fs.String("skills", "", "comma-separated skill IDs to sync (default all)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	selected := []string{}
	for _, part := range strings.Split(*skillsArg, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			selected = append(selected, trimmed)
		}
	}

	result, err := clientskillsync.SyncSkills(*workspace, selected)
	if err != nil {
		return err
	}
	fmt.Printf("downloaded: %d\n", len(result.Downloaded))
	fmt.Printf("skipped: %d\n", len(result.Skipped))
	return nil
}

func runScan(args []string) error {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	workspace := fs.String("workspace", ".", "workspace directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return clientscan.Run(*workspace)
}
