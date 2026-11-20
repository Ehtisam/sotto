//go:build !windows

package ui

import (
	"context"
	"os/exec"

	"github.com/Ehtisam/sotto/internal/proc"
)

// shellCommand, komutu sh ile çalıştırır.
func shellCommand(ctx context.Context, command string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	proc.Detach(cmd)
	return cmd
}
