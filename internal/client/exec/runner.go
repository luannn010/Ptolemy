package clientexec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	clientworkspace "github.com/luannn010/ptolemy/internal/client/workspace"
	"github.com/luannn010/ptolemy/internal/shellcmd"
)

var ErrDeniedByPolicy = errors.New("command denied by policy")

type PolicyFunc func(command string) error

type Options struct {
	Workspace      string
	Shell          string
	TimeoutSeconds int
	Policy         PolicyFunc
}

type Runner struct {
	guard          clientworkspace.Guard
	shell          string
	timeoutSeconds int
	policy         PolicyFunc
}

type Result struct {
	Command  string
	Stdout   string
	Stderr   string
	ExitCode int
	TimedOut bool
	Duration time.Duration
}

func New(opts Options) (Runner, error) {
	guard, err := clientworkspace.New(opts.Workspace)
	if err != nil {
		return Runner{}, err
	}

	shell := opts.Shell
	if shell == "" {
		shell = shellcmd.DefaultProgram(runtime.GOOS)
	}

	timeoutSeconds := opts.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 120
	}

	policy := opts.Policy
	if policy == nil {
		policy = AllowAllPolicy
	}

	return Runner{
		guard:          guard,
		shell:          shell,
		timeoutSeconds: timeoutSeconds,
		policy:         policy,
	}, nil
}

func (r Runner) Run(command string) (Result, error) {
	if err := r.policy(command); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrDeniedByPolicy, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(r.timeoutSeconds)*time.Second)
	defer cancel()

	cmd := shellcmd.CommandForProgram(ctx, runtime.GOOS, r.shell, command)
	cmd.Dir = r.guard.Root()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	runErr := cmd.Run()
	duration := time.Since(start)

	result := Result{
		Command:  command,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
		TimedOut: ctx.Err() == context.DeadlineExceeded,
		Duration: duration,
	}

	if runErr == nil {
		return result, nil
	}

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}

	return result, runErr
}

func AllowAllPolicy(string) error {
	return nil
}
