package connect

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ehtisam/sotto/internal/bridge"
)

// TestLiveQueueMove, çalan sırada iki şarkının yerini değiştirir, sonucu
// okur ve eski sırayı geri koyar. Çalan bir şey yoksa atlanır.
// SOTTO_LIVE=1 go test ./internal/connect -run LiveQueueMove -v
func TestLiveQueueMove(t *testing.T) {
	if os.Getenv("SOTTO_LIVE") == "" {
		t.Skip("SOTTO_LIVE not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	tok, err := bridge.New(filepath.Join(os.Getenv("LOCALAPPDATA"), "sotto", "bridge")).Token(ctx)
	if err != nil {
		t.Fatal(err)
	}
	names := func(ts []Track) []string {
		var out []string
		for i, t := range ts {
			if i >= 6 {
				break
			}
			out = append(out, t.URI[len(t.URI)-6:]+"/"+t.Provider)
		}
		return out
	}
	s, err := Open(ctx, tok)
	if err != nil {
		t.Fatal(err)
	}
	c := s.Cluster()
	t.Logf("active %q, devices %v", c.ActiveDeviceID, c.ActiveDeviceID != "")
	before := c.PlayerState.NextTracks
	t.Logf("before: %v (revision %s)", names(before), c.PlayerState.QueueRevision)
	if c.ActiveDeviceID == "" || len(before) < 3 {
		s.Close()
		t.Skip("nothing playing")
	}
	k := Locate(before, 0, before[0].URI, before[0].UID)
	moved, ok := Move(before, k, 1)
	if !ok {
		s.Close()
		t.Fatalf("can't move row %d", k)
	}
	if err := s.SetQueue(ctx, moved); err != nil {
		s.Close()
		t.Fatalf("set_queue: %v", err)
	}
	s.Close()
	time.Sleep(1500 * time.Millisecond)

	s2, err := Open(ctx, tok)
	if err != nil {
		t.Fatal(err)
	}
	after := s2.Cluster().PlayerState.NextTracks
	t.Logf("after:  %v (revision %s)", names(after), s2.Cluster().PlayerState.QueueRevision)
	swapped := len(after) > 1 && after[0].URI == before[1].URI && after[1].URI == before[0].URI
	// Eski sırayı geri koy (değiştiyse).
	if swapped {
		back, _ := Move(after, 0, 1)
		t.Logf("restore: %v", s2.SetQueue(ctx, back))
	}
	s2.Close()
	if !swapped {
		t.Errorf("the player did not keep the new order")
	}
}
