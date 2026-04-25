# Agent Task: Add task-file support and multi-step loop

## Goal
Update `cmd/ptolemy-agent/main.go` so the agent can:
- accept `--task-file`
- accept `--max-steps`
- read a markdown task file
- execute a loop instead of only one action

## File to edit
cmd/ptolemy-agent/main.go

## Required changes

1. Add this import:

```go
"flag"```
2. In main(), replace the current task parsing logic:
```task := strings.Join(os.Args[1:], " ")
if task == "" {
	fmt.Println("usage: ptolemy-agent <task>")
	os.Exit(1)
}
```
with:

``` taskFile := flag.String("task-file", "", "markdown task file to execute")
maxSteps := flag.Int("max-steps", 8, "max agent steps")
flag.Parse()

task := strings.Join(flag.Args(), " ")

if *taskFile != "" {
	data, err := os.ReadFile(*taskFile)
	if err != nil {
		fmt.Printf("failed to read task file: %v\n", err)
		os.Exit(1)
	}
	task = string(data)
}

if strings.TrimSpace(task) == "" {
	fmt.Println("usage: ptolemy-agent [--task-file path] [--max-steps 8] <task>")
	os.Exit(1)
}
```
3. Replace the single brain call with a loop using maxSteps.

The loop should:

- send task + observations to Gemma
- parse JSON action
- execute one tool action
- append result to observations
- stop when action is explain
4. Supported actions must include:
- run_command
- read_file
- write_file
- explain
- ask_approval
