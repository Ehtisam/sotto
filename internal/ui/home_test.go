package ui

import (
	"testing"
	"time"

	"github.com/Ehtisam/sotto/internal/config"
	"github.com/Ehtisam/sotto/internal/spotify"
)

type ctxRef = struct {
	URI  string `json:"uri"`
	Type string `json:"type"`
}

func play(name, ctxURI, ctxType string, at time.Time) spotify.PlayHistory {
	h := spotify.PlayHistory{
		Track: spotify.Track{Name: name, URI: "spotify:track:" + name,
			Album:   spotify.Album{ID: "al", URI: "spotify:album:al", Name: "Album"},
			Artists: []spotify.Artist{{Name: "Art", URI: "spotify:artist:ar"}}},
		PlayedAt: at,
	}
	if ctxURI != "" {
		h.Context = &ctxRef{URI: ctxURI, Type: ctxType}
	}
	return h
}

func homeModel(t *testing.T) Model {
	t.Helper()
	dir := t.TempDir()
	m := New(config.Config{ConfigDir: dir, LocalDir: dir})
	m.wiz.on = false // testler kurulumu bitmiş bir sotto ile çalışır
	m.width, m.height = 120, 40
	m.now = time.Date(2026, 11, 20, 20, 0, 0, 0, time.UTC)
	m.lib.entries = []libEntry{
		{kind: kindLiked, key: likedKey, name: "Liked Songs", total: 3},
		{kind: kindPlaylist, key: "pl", uri: "spotify:playlist:pl", name: "KaBoom.2"},
	}
	return m
}

func TestHomeSections(t *testing.T) {
	m := homeModel(t)
	at := m.now.Add(-90 * time.Minute)
	m.home.recent = []spotify.PlayHistory{
		play("a", "spotify:playlist:pl", "playlist", at),
		play("a", "spotify:playlist:pl", "playlist", at), // art arda aynı şarkı
		play("b", "spotify:album:al", "album", at),
		play("c", "spotify:user:me:collection", "collection", at),
		play("d", "spotify:playlist:unknown", "playlist", at), // kitaplıkta olmayan liste atlanır
	}
	m.rebuildHome()

	var kinds []string
	for _, it := range m.home.items {
		kinds = append(kinds, it.kind)
	}
	// başlık, liste, albüm, beğenilenler, başlık, 4 farklı şarkı, "hepsini gör"
	want := []string{"header", "playlist", "album", "liked", "space", "header", "track", "track", "track", "track", "more"}
	if len(kinds) != len(want) {
		t.Fatalf("kinds = %v", kinds)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("kinds = %v, want %v", kinds, want)
		}
	}
	if m.home.items[1].name != "KaBoom.2" {
		t.Fatalf("playlist name = %q", m.home.items[1].name)
	}
	if m.home.cursor != 1 {
		t.Fatalf("cursor = %d, want first selectable row", m.home.cursor)
	}
	// Son çalınan şarkı, çalındığı listede başlatılır; beğenilenler bağlam sayılmaz.
	if m.home.items[6].ctx != "spotify:playlist:pl" || m.home.items[8].ctx != "" {
		t.Fatalf("track contexts: %q %q", m.home.items[6].ctx, m.home.items[8].ctx)
	}
}

func TestAgo(t *testing.T) {
	now := time.Date(2026, 11, 20, 20, 0, 0, 0, time.UTC)
	cases := map[time.Duration]string{
		30 * time.Second: "just now",
		5 * time.Minute:  "5 min ago",
		3 * time.Hour:    "3 h ago",
		30 * time.Hour:   "yesterday",
		72 * time.Hour:   "3 days ago",
	}
	for d, want := range cases {
		if got := ago(now, now.Add(-d)); got != want {
			t.Errorf("ago(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestRecentRowsAreMarkedHistory(t *testing.T) {
	m := homeModel(t)
	m.home.recent = []spotify.PlayHistory{play("x", "spotify:playlist:pl", "playlist", m.now.Add(-time.Hour))}
	m.rebuildHome()
	found := false
	for _, it := range m.homeItems() {
		if it.kind == "track" && it.ctx != "" {
			found = true
			if !it.history {
				t.Fatalf("recent row %q not marked as history", it.name)
			}
		}
	}
	if !found {
		t.Fatal("no recent row with a context in the test data")
	}
}
