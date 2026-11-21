package discord

import (
	"encoding/json"
	"io"
	"net"
)

// FakeForTest, Discord'un yerine geçen bir sahte başlatır: el sıkışmayı
// karşılar, gelen her SET_ACTIVITY'nin etkinliğini (nil: temizleme) kanala
// yazar. Geri yükleme işlevini döner.
func FakeForTest() (activities <-chan map[string]any, restore func()) {
	ch := make(chan map[string]any, 32)
	old := dial
	dial = func() (io.ReadWriteCloser, error) {
		server, client := net.Pipe()
		go func() {
			if _, _, err := read(server); err != nil {
				return
			}
			_ = send(server, opFrame, map[string]any{"evt": "READY"})
			for {
				op, body, err := read(server)
				if err != nil {
					return
				}
				if op != opFrame {
					continue
				}
				var m struct {
					Args struct {
						Activity map[string]any `json:"activity"`
					} `json:"args"`
				}
				_ = json.Unmarshal(body, &m)
				select {
				case ch <- m.Args.Activity:
				default:
				}
				_ = send(server, opFrame, map[string]any{})
			}
		}()
		return client, nil
	}
	return ch, func() { dial = old }
}
