//go:build linux

package proc

import (
	"os/exec"
	"syscall"
)

// Detach, programı ayrı bir süreç grubunda başlatır ve sotto ölürse onun da
// ölmesini çekirdekten ister (Pdeathsig).
func Detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
}

func BindToParent(int) error { return nil }
