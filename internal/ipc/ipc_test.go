package ipc

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.sock")
	if _, err := Call(path, Request{Cmd: "status"}, time.Second); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("no server: %v", err)
	}
	srv, err := Listen(path, func(_ context.Context, r Request) Response {
		return Done("got "+r.Cmd+" "+r.Args[0], map[string]int{"n": len(r.Args)})
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	resp, err := Call(path, Request{Cmd: "seek", Args: []string{"+10"}}, time.Second)
	if err != nil || !resp.OK || resp.Text != "got seek +10" || string(resp.Data) != `{"n":1}` {
		t.Fatalf("%+v %v", resp, err)
	}
	// İkinci dinleyici açılmaz.
	if _, err := Listen(path, nil); !errors.Is(err, ErrInUse) {
		t.Fatalf("second listener: %v", err)
	}
}
