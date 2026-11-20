package connect

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ehtisam/sotto/internal/bridge"
)

// TestLiveRead, gerçek hesapta yalnızca okur: gözlemci olarak kaydolur,
// sırayı yazdırır, kaydı siler. Sırayı değiştirmez.
// Çalıştırmak için: SOTTO_LIVE=1 go test ./internal/connect -run Live -v
func TestLiveRead(t *testing.T) {
	if os.Getenv("SOTTO_LIVE") == "" {
		t.Skip("SOTTO_LIVE not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	br := bridge.New(filepath.Join(os.Getenv("LOCALAPPDATA"), "sotto", "bridge"))
	tok, err := br.Token(ctx)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(ctx, tok)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := s.Cluster()
	ps := c.PlayerState
	t.Logf("active=%q context=%q revision=%s prev=%d next=%d", c.ActiveDeviceID, ps.ContextURI, ps.QueueRevision, len(ps.PrevTracks), len(ps.NextTracks))
	if ps.Track != nil {
		t.Logf("now: %s uid=%q provider=%q", ps.Track.URI, ps.Track.UID, ps.Track.Provider)
	}
	for i, tr := range ps.NextTracks {
		if i >= 15 {
			break
		}
		keys := make([]string, 0, len(tr.Metadata))
		for k := range tr.Metadata {
			keys = append(keys, k)
		}
		t.Logf("%2d %s uid=%q provider=%q queued=%v meta=%v", i, tr.URI, tr.UID, tr.Provider, tr.Queued(), keys)
	}
	if len(ps.NextTracks) > 0 {
		t.Logf("raw row 0: %s", ps.NextTracks[0].raw)
	}
}
