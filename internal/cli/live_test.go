package cli

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/Ehtisam/sotto/internal/config"
	"github.com/Ehtisam/sotto/internal/spotify"
)

// SOTTO_LIVE=1 go test ./internal/cli -run LiveFind -v
// "sotto play <sözcükler>" aramasını gerçek hesapta dener (bir şey çalmaz).
func TestLiveFind(t *testing.T) {
	if os.Getenv("SOTTO_LIVE") == "" {
		t.Skip("SOTTO_LIVE=1 ile çalışır")
	}
	cfg, _ := config.Load()
	sp := spotify.NewClient(cfg.Spotify.ClientID, cfg.TokenFile())
	ctx := context.Background()
	for _, c := range []struct{ words, kind, want string }{
		{"daft punk one more time", "", "One More Time"},
		{"seksendört neyin var", "", "Neyin Var"},
		{"random access memories", "album", "Random Access Memories"},
		{"radiohead", "artist", "Radiohead"},
		{"kaboom", "playlist", ""},
	} {
		uri, label, err := Find(ctx, sp, c.words, c.kind)
		t.Logf("%q (%s) → %s %s %v", c.words, c.kind, label, uri, err)
		if err != nil || !strings.Contains(label, c.want) {
			t.Errorf("%q: %q %v", c.words, label, err)
		}
	}
	if _, _, err := Find(ctx, sp, "zzqqxx nonexistent qqzz 12345", ""); err == nil {
		t.Error("nonsense found something")
	} else {
		t.Logf("nonsense: %v", err)
	}
}
