//go:build !windows

package voice

import (
	"context"
	"errors"
)

func PutPCToSleep(ctx context.Context) error {
	_ = ctx
	return errors.New("sleep is only supported on windows for this MVP")
}
