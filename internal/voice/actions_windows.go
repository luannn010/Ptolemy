//go:build windows

package voice

import (
	"context"
	"os/exec"
)

func PutPCToSleep(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", `Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.Application]::SetSuspendState('Suspend', $false, $false)`)
	return cmd.Run()
}
