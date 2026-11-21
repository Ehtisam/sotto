//go:build !windows

package discord

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
)

// dialIPC, Discord'un unix soketine bağlanır ($XDG_RUNTIME_DIR, TMPDIR, /tmp;
// Flatpak ve Snap kurulumlarının alt klasörleri de).
func dialIPC() (io.ReadWriteCloser, error) {
	var dirs []string
	for _, k := range []string{"XDG_RUNTIME_DIR", "TMPDIR", "TMP", "TEMP"} {
		if v := os.Getenv(k); v != "" {
			dirs = append(dirs, v)
		}
	}
	dirs = append(dirs, "/tmp")
	for _, d := range dirs {
		for _, sub := range []string{"", "app/com.discordapp.Discord", "snap.discord"} {
			for i := 0; i < 10; i++ {
				c, err := net.Dial("unix", filepath.Join(d, sub, fmt.Sprintf("discord-ipc-%d", i)))
				if err == nil {
					return c, nil
				}
			}
		}
	}
	return nil, os.ErrNotExist
}
