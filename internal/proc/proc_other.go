//go:build !windows && !linux

package proc

import (
	"os/exec"
	"syscall"
)

// Detach, programı ayrı bir süreç grubunda başlatır; terminaldeki Ctrl+C
// doğrudan ona gitmez, sotto onu kendisi kapatır.
func Detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// BindToParent: macOS'ta karşılığı yok; sotto düzgün kapanınca programı kendisi kapatır.
func BindToParent(int) error { return nil }
