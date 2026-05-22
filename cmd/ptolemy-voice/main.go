//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/luannn010/ptolemy/internal/voice"
)

const commandWindow = 30 * time.Second

func main() {
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

	fmt.Println("Voice catcher started. Say 'Hey Ptolemy' to activate.")

	activeUntil := time.Time{}

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
			if normalized == "" {
				continue
			}
			if activeUntil.IsZero() {
				if voice.IsWakePhrase(normalized) {
					activeUntil = time.Now().Add(commandWindow)
					fmt.Println("Wake phrase detected. Listening for command for 30 seconds...")
				}
				continue
			}
			if voice.IsCommandWindowExpired(activeUntil, time.Now()) {
				fmt.Println("No command received in 30 seconds. Returning to wake mode.")
				activeUntil = time.Time{}
				continue
			}
			cmd, ok := voice.ParseCommand(normalized, time.Now())
			if !ok {
				fmt.Println("I didn't catch that.")
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
