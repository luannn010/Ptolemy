//go:build windows && vosk

package voice

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	vosk "github.com/alphacep/vosk-api/go"
	"github.com/gordonklaus/portaudio"
)

const sampleRate = 16000

type voskListener struct {
	modelPath string
}

type voskResult struct {
	Text string `json:"text"`
}

func NewListener() (Listener, error) {
	modelPath := strings.TrimSpace(os.Getenv("VOSK_MODEL_PATH"))
	if modelPath == "" {
		modelPath = ".state/vosk-model"
	}
	return &voskListener{modelPath: modelPath}, nil
}

func (l *voskListener) Listen(ctx context.Context) (<-chan string, error) {
	if _, err := os.Stat(l.modelPath); err != nil {
		return nil, fmt.Errorf("vosk model path not found (%s): %w", l.modelPath, err)
	}
	model, err := vosk.NewModel(l.modelPath)
	if err != nil {
		return nil, fmt.Errorf("load vosk model: %w", err)
	}
	rec, err := vosk.NewRecognizer(model, sampleRate)
	if err != nil {
		model.Free()
		return nil, fmt.Errorf("create recognizer: %w", err)
	}
	if err := portaudio.Initialize(); err != nil {
		rec.Free()
		model.Free()
		return nil, fmt.Errorf("initialize portaudio: %w", err)
	}

	out := make(chan string, 16)
	go func() {
		defer close(out)
		defer rec.Free()
		defer model.Free()
		defer portaudio.Terminate()

		buffer := make([]int16, 2048)
		stream, err := portaudio.OpenDefaultStream(1, 0, float64(sampleRate), len(buffer), &buffer)
		if err != nil {
			return
		}
		defer stream.Close()

		if err := stream.Start(); err != nil {
			return
		}
		defer stream.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if err := stream.Read(); err != nil {
				continue
			}
			if !rec.AcceptWaveform(buffer) {
				continue
			}
			var parsed voskResult
			if err := json.Unmarshal([]byte(rec.Result()), &parsed); err != nil {
				continue
			}
			text := strings.TrimSpace(parsed.Text)
			if text != "" {
				out <- text
			}
		}
	}()
	return out, nil
}
