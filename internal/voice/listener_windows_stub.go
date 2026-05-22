//go:build windows && !vosk

package voice

import "errors"

type stubListener struct{}

func NewListener() (Listener, error) {
	return nil, errors.New("vosk listener requires build tag 'vosk' and Vosk/PortAudio dependencies")
}
