package discord

import (
	"fmt"
	"io"
	"os"
)

// dialIPC, Discord'un adlandırılmış borusuna bağlanır (discord-ipc-0 … 9).
func dialIPC() (io.ReadWriteCloser, error) {
	for i := 0; i < 10; i++ {
		f, err := os.OpenFile(fmt.Sprintf(`\\.\pipe\discord-ipc-%d`, i), os.O_RDWR, 0)
		if err == nil {
			return f, nil
		}
	}
	return nil, os.ErrNotExist
}
