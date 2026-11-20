//go:build !windows && !linux

package soloist

import (
	"os/exec"
	"syscall"
)

// detach, Soloist'i ayrı bir süreç grubunda başlatır; terminaldeki Ctrl+C
// doğrudan ona gitmez, sotto onu kendisi kapatır.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// bindToParent: macOS'ta karşılığı yok; sotto düzgün kapanınca Soloist'i kendisi kapatır.
func bindToParent(int) error { return nil }
