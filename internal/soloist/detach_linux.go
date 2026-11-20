//go:build linux

package soloist

import (
	"os/exec"
	"syscall"
)

// detach, Soloist'i ayrı bir süreç grubunda başlatır ve sotto ölürse onun da
// ölmesini çekirdekten ister (Pdeathsig).
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
}

func bindToParent(int) error { return nil }
