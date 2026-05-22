//go:build !windows

package voice

import "errors"

func NewListener() (Listener, error) {
	return nil, errors.New("voice catcher MVP is windows-only")
}
