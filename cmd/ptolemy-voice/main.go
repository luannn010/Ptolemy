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

// useColor reports whether the dimmed observation block should use ANSI codes.
// Honors the NO_COLOR convention (https://no-color.org/).
func useColor() bool {
	return strings.TrimSpace(os.Getenv("NO_COLOR")) == ""
}

// printObservation prints the dimmed multi-line diagnostic block under the
// normal output. It is always on, narrating what was heard, what it was caught
// as, and (for executed commands) the request payload, tools used, and timing.
func printObservation(obs voice.Observation) {
	block := voice.FormatObservation(obs, useColor())
	if block != "" {
		fmt.Println(block)
	}
}

func main() {
	realtimeJSON := flag.Bool("realtime-json", false, "emit runtime events as JSON lines")
	listenOnly := flag.Bool("listen-only", false, "only stream recognized phrases; do not run wake/command state machine")
	noActions := flag.Bool("no-actions", false, "parse and acknowledge commands without executing system actions")
	enroll := flag.Bool("enroll", false, "(re)run wake-phrase enrollment, then exit")
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

	profilePath := voice.WakeProfilePath()
	if *enroll || !fileExists(profilePath) {
		samples := runEnrollment(ctx, phrases)
		profile, warnings := voice.BuildProfileFromSamples(samples)
		if err := profile.Save(profilePath); err != nil {
			fmt.Printf("could not save wake profile: %v\n", err)
		} else {
			fmt.Printf("Saved wake profile to %s\n", profilePath)
		}
		fmt.Printf("Wake variants: %s\n", strings.Join(profile.Variants, " | "))
		for _, w := range warnings {
			fmt.Printf("  note: %s\n", w)
		}
		if *enroll {
			fmt.Println("Enrollment complete.")
			return
		}
	}
	profile := voice.LoadWakeProfile(profilePath)

	execClient := voice.NewHTTPExecutorClient(workerBaseURL())
	fmt.Printf("Voice catcher started. Executor: %s\n", workerBaseURL())
	fmt.Println("Say 'Hey Ptolemy' to activate.")
	fmt.Println("Every phrase the speech engine recognizes is printed below as `heard: \"...\"`.")
	fmt.Println("If you speak and see no `heard:` line, the microphone/speech engine is not picking up audio.")

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
			// Always echo what was recognized so you can see exactly what the
			// speech engine transcribed, even when it isn't the wake phrase or a
			// known command. This is the key diagnostic for "nothing caught".
			state := "asleep — waiting for wake phrase"
			if !activeUntil.IsZero() {
				state = "awake — listening for command"
			}
			fmt.Printf("heard: %q  [%s]\n", phrase, state)
			if *listenOnly {
				continue
			}
			if activeUntil.IsZero() {
				if profile.Matches(normalized) {
					activeUntil = time.Now().Add(commandWindow)
					fmt.Println("Wake phrase detected. Listening for command for 30 seconds...")
					printObservation(voice.Observation{Heard: phrase, Caught: "wake"})
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
					freshSession := sessionID == ""
					id, res, runErr := runShellViaExecutor(ctx, execClient, &sessionID, pendingShell, *realtimeJSON)
					sessionID = id
					if runErr != nil {
						fmt.Printf("Command failed: %v\n", runErr)
					}
					tools := []string{"execute"}
					if freshSession {
						tools = []string{"open_session", "execute"}
					}
					printObservation(voice.Observation{
						Heard:    phrase,
						Caught:   string(voice.CommandRunShell),
						Request:  trimForObservation(res.RequestPayload),
						Tools:    tools,
						TimingMS: res.DurationMS,
					})
				} else {
					fmt.Printf("Cancelled. Did not run: %s\n", pendingShell)
					printObservation(voice.Observation{Heard: phrase, Caught: "cancel"})
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
				printObservation(voice.Observation{
					Heard:  phrase,
					Caught: string(voice.CommandRunShell) + " (awaiting confirm)",
				})
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
				printObservation(voice.Observation{Heard: phrase, Caught: string(cmd.Type) + " (dry-run)"})
				activeUntil = time.Time{}
				fmt.Println("Returning to wake mode.")
				continue
			}
			start := time.Now()
			if err := executeCommand(ctx, scheduler, cmd); err != nil {
				fmt.Printf("Command failed: %v\n", err)
			}
			printObservation(voice.Observation{
				Heard:    phrase,
				Caught:   string(cmd.Type),
				Tools:    []string{"local:" + string(cmd.Type)},
				TimingMS: time.Since(start).Milliseconds(),
			})
			activeUntil = time.Time{}
			fmt.Println("Returning to wake mode.")
		}
	}
}

// enrollmentSamples is how many times the user is asked to say the wake phrase.
const enrollmentSamples = 4

// perSampleTimeout is how long to wait for one spoken sample before re-prompting.
const perSampleTimeout = 10 * time.Second

// maxSampleAttempts caps re-prompts per sample when the user is silent.
const maxSampleAttempts = 3

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// runEnrollment guides the user through saying the wake phrase several times and
// returns the raw transcripts Vosk produced. Silence on a sample is re-prompted
// up to maxSampleAttempts times.
func runEnrollment(ctx context.Context, phrases <-chan string) []string {
	fmt.Println("--- Wake-phrase setup ---")
	fmt.Printf("Say \"hey ptolemy\" %d times so Ptolemy learns how you say it.\n", enrollmentSamples)
	samples := make([]string, 0, enrollmentSamples)
	for i := 1; i <= enrollmentSamples; i++ {
		captured := ""
		for attempt := 1; attempt <= maxSampleAttempts && captured == ""; attempt++ {
			fmt.Printf("  (%d/%d) Say \"hey ptolemy\" now...\n", i, enrollmentSamples)
			timer := time.NewTimer(perSampleTimeout)
			select {
			case <-ctx.Done():
				timer.Stop()
				return samples
			case phrase, ok := <-phrases:
				timer.Stop()
				if !ok {
					return samples
				}
				captured = strings.TrimSpace(phrase)
				if captured == "" {
					continue
				}
				fmt.Printf("      heard: %q\n", captured)
			case <-timer.C:
				fmt.Println("      (didn't catch that — try again)")
			}
		}
		if captured != "" {
			samples = append(samples, captured)
		}
	}
	fmt.Println("--- setup done ---")
	return samples
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

// trimForObservation shortens a request payload so the dimmed observation block
// stays to a single readable line.
func trimForObservation(payload string) string {
	const maxObservationPayload = 120
	if len(payload) <= maxObservationPayload {
		return payload
	}
	return payload[:maxObservationPayload] + "…"
}

// runShellViaExecutor sends a confirmed shell command to the executor, opening a
// session lazily on first use and reusing it afterward. It returns the (possibly
// newly opened) session ID and the ExecResult (which carries the request payload
// and round-trip timing for the observation block).
func runShellViaExecutor(ctx context.Context, client voice.ExecutorClient, sessionID *string, shell string, jsonEvents bool) (string, voice.ExecResult, error) {
	id := *sessionID
	if id == "" {
		opened, err := client.OpenSession(ctx)
		if err != nil {
			return id, voice.ExecResult{}, fmt.Errorf("open executor session: %w", err)
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
		return id, res, err
	}

	fmt.Printf("exit %d: %s\n", res.ExitCode, res.Summary)
	emitEvent(jsonEvents, runtimeEvent{
		Time:    time.Now().Format(time.RFC3339Nano),
		Type:    "command_executed",
		Shell:   shell,
		Active:  false,
		Message: fmt.Sprintf("exit %d", res.ExitCode),
	})
	return id, res, nil
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
