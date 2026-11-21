package discord

import (
	"encoding/json"
	"io"
	"net"
	"testing"
	"time"
)

// Sahte Discord: el sıkışmayı karşılar, gelen SET_ACTIVITY'leri kanala yazar.
func TestPresenceSendsActivity(t *testing.T) {
	got := make(chan map[string]any, 8)
	server, client := net.Pipe()
	old := dial
	used := false
	dial = func() (io.ReadWriteCloser, error) {
		if used {
			return nil, io.EOF
		}
		used = true
		return client, nil
	}
	defer func() { dial = old }()
	go func() {
		op, body, err := read(server)
		if err != nil || op != opHandshake {
			t.Errorf("handshake op %d %v", op, err)
			return
		}
		var h map[string]any
		_ = json.Unmarshal(body, &h)
		if h["client_id"] != "123" {
			t.Errorf("client id %v", h)
		}
		_ = send(server, opFrame, map[string]any{"evt": "READY"})
		for {
			op, body, err := read(server)
			if err != nil {
				return
			}
			var m map[string]any
			_ = json.Unmarshal(body, &m)
			if op == opFrame {
				got <- m
				_ = send(server, opFrame, map[string]any{"evt": nil})
			}
		}
	}()
	p := Start("123")
	start := time.Now()
	p.Set(&Activity{Title: "Song", Artist: "Band", Album: "Record", Cover: "https://i.scdn.co/image/x", Start: start, End: start.Add(3 * time.Minute)})
	select {
	case m := <-got:
		args := m["args"].(map[string]any)
		act := args["activity"].(map[string]any)
		if m["cmd"] != "SET_ACTIVITY" || act["details"] != "Song" || act["state"] != "Band" || act["type"].(float64) != 2 {
			t.Fatalf("activity %v", m)
		}
		if act["assets"].(map[string]any)["large_image"] != "https://i.scdn.co/image/x" {
			t.Fatalf("assets %v", act["assets"])
		}
	case <-time.After(3 * time.Second):
		t.Fatal("nothing sent")
	}
	p.Set(nil)
	select {
	case m := <-got:
		if m["args"].(map[string]any)["activity"] != nil {
			t.Fatalf("clear sent %v", m)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("clear not sent")
	}
	go func() { // kapanırken gelen temizleme ve kapanış
		for range got {
		}
	}()
	p.Close()
}
