package ui

import (
	"testing"

	"github.com/Ehtisam/sotto/internal/spotify"
)

func sampleResult() spotify.SearchResult {
	var r spotify.SearchResult
	for _, n := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		r.Tracks.Items = append(r.Tracks.Items, &spotify.Track{Name: n, URI: "spotify:track:" + n})
	}
	r.Tracks.Total = 7
	r.Artists.Items = []*spotify.FullArtist{{Artist: spotify.Artist{Name: "x", URI: "spotify:artist:x"}}, nil}
	r.Playlists.Items = []*spotify.Playlist{nil, {ID: "p", Name: "list", URI: "spotify:playlist:p"}}
	return r
}

func TestSearchItemsAll(t *testing.T) {
	items := searchItems(sampleResult(), filterAll)
	// başlık + 5 şarkı, başlık + 1 sanatçı, başlık + 1 liste
	if got, want := len(items), 1+5+1+1+1+1; got != want {
		t.Fatalf("len = %d, want %d", got, want)
	}
	if items[0].kind != "header" || items[6].kind != "header" {
		t.Fatalf("section headers missing: %+v", items)
	}
	if items[len(items)-1].kind != "playlist" {
		t.Fatalf("last item = %q, want playlist", items[len(items)-1].kind)
	}
}

func TestSearchItemsSingleFilterKeepsAll(t *testing.T) {
	r := sampleResult()
	items := searchItems(spotify.SearchResult{Tracks: r.Tracks}, filterSongs)
	if len(items) != 7 {
		t.Fatalf("len = %d, want 7 (no headers, no cap)", len(items))
	}
	if searchTotal(r, filterSongs) != 7 {
		t.Fatal("total")
	}
}

func TestNextSelectableSkipsHeaders(t *testing.T) {
	items := searchItems(sampleResult(), filterAll)
	if got := nextSelectable(items, 0, 1); got != 1 {
		t.Fatalf("down from header = %d, want 1", got)
	}
	if got := nextSelectable(items, 6, 1); got != 7 {
		t.Fatalf("down onto header = %d, want 7", got)
	}
	if got := nextSelectable(items, 6, -1); got != 5 {
		t.Fatalf("up onto header = %d, want 5", got)
	}
}

func TestLeaderPartsFits(t *testing.T) {
	for _, w := range []int{20, 40, 80} {
		l, dots, r := leaderParts("A rather long song title here", "Some Artist  3:41", w)
		if got := textWidth(l) + 1 + textWidth(dots) + 1 + textWidth(r); got > w {
			t.Fatalf("w=%d: row is %d wide", w, got)
		}
	}
}

func TestFitScroll(t *testing.T) {
	c, s := fitScroll(25, 0, 100, 10)
	if c != 25 || s != 16 {
		t.Fatalf("got cursor %d scroll %d", c, s)
	}
	c, s = fitScroll(-5, 3, 100, 10)
	if c != 0 || s != 0 {
		t.Fatalf("got cursor %d scroll %d", c, s)
	}
}
