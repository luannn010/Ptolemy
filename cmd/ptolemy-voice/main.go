//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/luannn010/ptolemy/internal/config"
	"github.com/luannn010/ptolemy/internal/voice"
)

const commandWindow = 30 * time.Second

type runtimeEvent struct {
	Time       string         `json:"time"`
	Type       string         `json:"type"`
	Heard      string         `json:"heard,omitempty"`
	Normalized string         `json:"normalized,omitempty"`
	Active     bool           `json:"active"`
	Command    *voice.Command `json:"command,omitempty"`
	Shell      string         `json:"shell,omitempty"`
	Message    string         `json:"message,omitempty"`
}

func workerBaseURL() string {
	if v := strings.TrimSpace(os.Getenv("WORKER_BASE_URL")); v != "" {
		return v
	}
	return config.DefaultWorkerBaseURL
}

func main() {
	realtimeJSON := flag.Bool("realtime-json", false, "emit runtime events as JSON lines")
	listenOnly := flag.Bool("listen-only", false, "only stream recognized phrases; do not run wake/command state machine")
	noActions := flag.Bool("no-actions", false, "parse and acknowledge commands without executing system actions")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	scheduler := voice.NewScheduler()
	defer scheduler.Stop()

	listener, err := voice.NewListener()
	if err != nil {
		fmt.Printf("failed to initialize voice listener: %v\n", err)
		os.Exit(1)
	}

	phrases, err := listener.Listen(ctx)
	if err != nil {
		fmt.Printf("failed to start listening: %v\n", err)
		os.Exit(1)
	}

	execClient := voice.NewHTTPExecutorClient(workerBaseURL())
	fmt.Printf("Voice catcher started. Executor: %s\n", workerBaseURL())
	fmt.Println("Say 'Hey Ptolemy' to activate.")

	activeUntil := time.Time{}
	pendingShell := ""
	sessionID := ""

	for {
		select {
		case <-ctx.Done():
			fmt.Println("voice catcher stopped")
			return
		case phrase, ok := <-phrases:
			if !ok {
				fmt.Println("listener stopped")
				return
			}
			normalized := strings.TrimSpace(strings.ToLower(phrase))
			emitEvent(*realtimeJSON, runtimeEvent{
				Time:       time.Now().Format(time.RFC3339Nano),
				Type:       "heard",
				Heard:      phrase,
				Normalized: normalized,
				Active:     !activeUntil.IsZero(),
			})
			if normalized == "" {
				continue
			}
			if *listenOnly {
				continue
			}
			if activeUntil.IsZero() {
				if voice.IsWakePhrase(normalized) {
					activeUntil = time.Now().Add(commandWindow)
					fmt.Println("Wake phrase detected. Listening for command for 30 seconds...")
					emitEvent(*realtimeJSON, runtimeEvent{
						Time:    time.Now().Format(time.RFC3339Nano),
						Type:    "wake_detected",
						Heard:   phrase,
						Active:  true,
						Message: "wake phrase detected",
					})
				}
				continue
			}
			if voice.IsCommandWindowExpired(activeUntil, time.Now()) {
				msg := "command window expired"
				if pendingShell != "" {
					msg = "confirmation window expired; command not run"
				}
				fmt.Println("No command received in 30 seconds. Returning to wake mode.")
				emitEvent(*realtimeJSON, runtimeEvent{
					Time:    time.Now().Format(time.RFC3339Nano),
					Type:    "command_window_timeout",
					Active:  false,
					Message: msg,
				})
				activeUntil = time.Time{}
				pendingShell = ""
				continue
			}
			// Awaiting confirmation for a spoken shell command.
			if pendingShell != "" {
				if voice.IsConfirmPhrase(normalized) {
					id, runErr := runShellViaExecutor(ctx, execClient, &sessionID, pendingShell, *realtimeJSON)
					sessionID = id
					if runErr != nil {
						fmt.Printf("Command failed: %v\n", runErr)
					}
				} else {
					fmt.Printf("Cancelled. Did not run: %s\n", pendingShell)
					emitEvent(*realtimeJSON, runtimeEvent{
						Time:    time.Now().Format(time.RFC3339Nano),
						Type:    "command_cancelled",
						Heard:   phrase,
						Shell:   pendingShell,
						Active:  false,
						Message: "not confirmed",
					})
				}
				pendingShell = ""
				activeUntil = time.Time{}
				fmt.Println("Returning to wake mode.")
				continue
			}
			cmd, ok := voice.ParseCommand(normalized, time.Now())
			if !ok {
				fmt.Println("I didn't catch that.")
				emitEvent(*realtimeJSON, runtimeEvent{
					Time:    time.Now().Format(time.RFC3339Nano),
					Type:    "command_unrecognized",
					Heard:   phrase,
					Active:  true,
					Message: "not parsed as supported command",
				})
				continue
			}
			emitEvent(*realtimeJSON, runtimeEvent{
				Time:    time.Now().Format(time.RFC3339Nano),
				Type:    "command_recognized",
				Heard:   phrase,
				Active:  true,
				Command: &cmd,
			})
			// Shell commands always require a verbal confirmation before they run,
			// so a misheard command never executes on its own.
			if cmd.Type == voice.CommandRunShell {
				if *noActions {
					fmt.Printf("Would run (dry-run, -no-actions set, will NOT execute): %s\n", cmd.Shell)
					activeUntil = time.Time{}
					fmt.Println("Returning to wake mode.")
					continue
				}
				pendingShell = cmd.Shell
				activeUntil = time.Now().Add(commandWindow)
				fmt.Printf("Would run: %s\nSay \"confirm\" within 30 seconds to execute, or anything else to cancel.\n", cmd.Shell)
				emitEvent(*realtimeJSON, runtimeEvent{
					Time:    time.Now().Format(time.RFC3339Nano),
					Type:    "command_pending",
					Heard:   phrase,
					Shell:   cmd.Shell,
					Active:  true,
					Message: "awaiting confirmation",
				})
				continue
			}
			if *noActions {
				fmt.Printf("Recognized command (dry-run): %s\n", cmd.Type)
				activeUntil = time.Time{}
				fmt.Println("Returning to wake mode.")
				continue
			}
			if err := executeCommand(ctx, scheduler, cmd); err != nil {
				fmt.Printf("Command failed: %v\n", err)
			}
			activeUntil = time.Time{}
			fmt.Println("Returning to wake mode.")
		}
	}
}

func emitEvent(enabled bool, event runtimeEvent) {
	if !enabled {
		return
	}
	b, err := json.Marshal(event)
	if err != nil {
		return
	}
	fmt.Println(string(b))
}

// runShellViaExecutor sends a confirmed shell command to the executor, opening a
// session lazily on first use and reusing it afterward. It returns the (possibly
// newly opened) session ID so the caller can cache it.
func runShellViaExecutor(ctx context.Context, client voice.ExecutorClient, sessionID *string, shell string, jsonEvents bool) (string, error) {
	id := *sessionID
	if id == "" {
		opened, err := client.OpenSession(ctx)
		if err != nil {
			return id, fmt.Errorf("open executor session: %w", err)
		}
		id = opened
	}

	emitEvent(jsonEvents, runtimeEvent{
		Time:    time.Now().Format(time.RFC3339Nano),
		Type:    "command_confirmed",
		Shell:   shell,
		Active:  true,
		Message: "confirmed; executing",
	})
	fmt.Printf("Running: %s\n", shell)

	res, err := client.Execute(ctx, id, shell)
	if err != nil {
		return id, err
	}

	fmt.Printf("exit %d: %s\n", res.ExitCode, res.Summary)
	emitEvent(jsonEvents, runtimeEvent{
		Time:    time.Now().Format(time.RFC3339Nano),
		Type:    "command_executed",
		Shell:   shell,
		Active:  false,
		Message: fmt.Sprintf("exit %d", res.ExitCode),
	})
	return id, nil
}

func executeCommand(ctx context.Context, scheduler *voice.Scheduler, cmd voice.Command) error {
	switch cmd.Type {
	case voice.CommandSleepPC:
		fmt.Println("Executing: sleep pc")
		return voice.PutPCToSleep(ctx)
	case voice.CommandSetAlarm:
		if cmd.When.IsZero() {
			return errors.New("alarm time is required")
		}
		scheduler.Schedule("alarm", cmd.When, "Alarm")
		fmt.Printf("Alarm set for %s\n", cmd.When.Format(time.RFC1123))
		return nil
	case voice.CommandSetReminder:
		if cmd.When.IsZero() {
			return errors.New("reminder time is required")
		}
		message := strings.TrimSpace(cmd.Message)
		if message == "" {
			message = "Reminder"
		}
		scheduler.Schedule("reminder", cmd.When, message)
		fmt.Printf("Reminder set for %s: %s\n", cmd.When.Format(time.RFC1123), message)
		return nil
	default:
		return fmt.Errorf("unsupported command type: %s", cmd.Type)
	}
}
