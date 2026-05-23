//go:build windows && !vosk

package voice

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

type windowsSpeechListener struct{}

func NewListener() (Listener, error) {
	return &windowsSpeechListener{}, nil
}

func (l *windowsSpeechListener) Listen(ctx context.Context) (<-chan string, error) {
	_ = l
	script := `
Add-Type -AssemblyName System.Speech
$engine = New-Object System.Speech.Recognition.SpeechRecognitionEngine
$engine.SetInputToDefaultAudioDevice()
$grammar = New-Object System.Speech.Recognition.DictationGrammar
$engine.LoadGrammar($grammar)
$engine.InitialSilenceTimeout = [TimeSpan]::FromSeconds(5)
$engine.BabbleTimeout = [TimeSpan]::FromSeconds(2)
while ($true) {
  $result = $engine.Recognize([TimeSpan]::FromSeconds(2))
  if ($null -ne $result -and -not [string]::IsNullOrWhiteSpace($result.Text)) {
    Write-Output $result.Text
  }
}
`
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", script)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("create stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start listener process: %w", err)
	}

	out := make(chan string, 16)
	go readSpeechLines(out, stdout)
	go drain(stderr)
	go func() {
		<-ctx.Done()
		_ = cmd.Process.Kill()
	}()
	return out, nil
}

func readSpeechLines(out chan<- string, r io.Reader) {
	defer close(out)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			out <- line
		}
	}
}

func drain(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
	}
}
