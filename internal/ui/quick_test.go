package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Ehtisam/sotto/internal/cli"
	"github.com/Ehtisam/sotto/internal/soloist"
	"github.com/Ehtisam/sotto/internal/spotify"
)

func TestSpotifyLinkFindsLinksInText(t *testing.T) {
	cases := map[string]string{
		"https://open.spotify.com/intl-tr/track/abc?si=x":         "spotify:track:abc",
		"listen: <https://open.spotify.com/album/al1> !":          "spotify:album:al1",
		"spotify:playlist:pl9":                                    "spotify:playlist:pl9",
		"no link here":                                            "",
		"https://example.com/track/abc":                           "",
		"open.spotify.com/show/sh1 and open.spotify.com/track/t2": "spotify:show:sh1",
	}
	for in, want := range cases {
		if got := spotifyLink(in); got != want {
			t.Errorf("spotifyLink(%q) = %q, want %q", in, got, want)
		}
	}
}

func playingModel(t *testing.T) (*fakeWeb, Model) {
	f, m := newFakeWeb(t)
	m.state.Item = &soloist.Entity{URI: "spotify:track:now", EntityType: "track"}
	m.state.Volume = 40
	m.state.Status = soloist.StatusPlaying
	m.remote.active, m.remote.device = true, "Phone"
	return f, m
}

func TestMuteRemembersAndRestoresVolume(t *testing.T) {
	f, m := playingModel(t)
	m = m.pressKey("m")
	if got := f.sent("PUT me/player/volume"); len(got) != 1 || !strings.Contains(got[0], "volume_percent=0") {
		t.Fatalf("mute sent %v", got)
	}
	if cli.MutedVolume(m.cfg.LocalDir) != 40 {
		t.Fatal("old volume not saved")
	}
	m.state.Volume = 0 // Spotify'dan gelen yeni durum
	if !m.muted() {
		t.Fatal("muted() false after mute")
	}
	m = m.pressKey("m")
	if got := f.sent("PUT me/player/volume"); len(got) != 2 || !strings.Contains(got[1], "volume_percent=40") {
		t.Fatalf("unmute sent %v", got)
	}
	if cli.MutedVolume(m.cfg.LocalDir) != 0 {
		t.Fatal("saved volume not cleared")
	}
}

func TestZeroSeeksToStart(t *testing.T) {
	f, m := playingModel(t)
	m.pressKey("0")
	if got := f.sent("PUT me/player/seek"); len(got) != 1 || !strings.Contains(got[0], "position_ms=0") {
		t.Fatalf("0 sent %v", got)
	}
}

func TestLikeTogglesSelectedTrack(t *testing.T) {
	f, m := playingModel(t)
	m.page, m.focus = pageLibrary, focusContent
	m.lib.entries = []libEntry{{kind: kindPlaylist, key: "pl", uri: "spotify:playlist:pl", name: "List", ownerID: "me"}}
	m.lib.open = &trackList{entry: m.lib.entries[0], tracks: []spotify.Track{{Name: "Sel", URI: "spotify:track:sel"}}, total: 1}

	f.on("GET me/library/contains", `[false]`)
	m = m.pressKey("h")
	if got := f.sent("PUT me/library"); len(got) != 1 || !strings.Contains(got[0], "spotify%3Atrack%3Asel") {
		t.Fatalf("like sent %v", got)
	}
	if !strings.Contains(m.flash, "Sel") {
		t.Fatalf("flash = %q", m.flash)
	}
	f.on("GET me/library/contains", `[true]`)
	m.pressKey("h")
	if got := f.sent("DELETE me/library"); len(got) != 1 {
		t.Fatalf("unlike sent %v", got)
	}
}

func TestLikeWithoutSelectionUsesPlayingTrack(t *testing.T) {
	f, m := playingModel(t)
	m.page, m.focus = pageNow, focusMenu
	f.on("GET me/library/contains", `[false]`)
	m.pressKey("h")
	if got := f.sent("PUT me/library"); len(got) != 1 || !strings.Contains(got[0], "spotify%3Atrack%3Anow") {
		t.Fatalf("like sent %v", got)
	}
}

func TestQueueSelectedTrackAndAlbum(t *testing.T) {
	f, m := playingModel(t)
	m.page, m.focus = pageLibrary, focusContent
	m.lib.entries = []libEntry{{kind: kindPlaylist, key: "pl", uri: "spotify:playlist:pl", name: "List", ownerID: "me", total: 3}}
	m.lib.open = &trackList{entry: m.lib.entries[0], tracks: []spotify.Track{{Name: "A", URI: "spotify:track:a"}, {Name: "B", URI: "spotify:track:b"}}, total: 2}
	m.lib.open.cursor = 1
	m = m.pressKey("u")
	if got := f.sent("POST me/player/queue"); len(got) != 1 || !strings.Contains(got[0], "spotify%3Atrack%3Ab") {
		t.Fatalf("queue sent %v", got)
	}

	// Liste düzeyinde: listenin şarkıları (Spotify'dan) sıraya.
	m.lib.open = nil
	f.on("GET playlists/pl/items", `{"total":3,"items":[{"item":{"uri":"spotify:track:x"}},{"item":{"uri":"spotify:track:y"}},{"item":{"uri":"spotify:track:z"}}]}`)
	m = m.pressKey("u")
	if got := f.sent("POST me/player/queue"); len(got) != 4 {
		t.Fatalf("queue sent %v", got)
	}
	if !strings.Contains(m.flash, "3") {
		t.Fatalf("flash = %q", m.flash)
	}
}

func TestQueueNeedsSelection(t *testing.T) {
	f, m := playingModel(t)
	m.page, m.focus = pageNow, focusMenu
	m = m.pressKey("u")
	if len(f.sent("POST")) != 0 || m.flash != T("queue.select") {
		t.Fatalf("u without selection: %v %q", f.sent("POST"), m.flash)
	}
}

func TestShufflePlayTurnsShuffleOnAndStartsSomewhere(t *testing.T) {
	f, m := playingModel(t)
	m.page, m.focus = pageLibrary, focusContent
	m.lib.entries = []libEntry{{kind: kindPlaylist, key: "pl", uri: "spotify:playlist:pl", name: "List", total: 30}}
	f.on("GET me/player/devices", `{"devices":[]}`)
	m.pressKey("S")
	sh := f.sent("PUT me/player/shuffle")
	play := f.sent("PUT me/player/play")
	if len(sh) != 1 || !strings.Contains(sh[0], "state=true") || len(play) != 1 {
		t.Fatalf("shuffle %v play %v", sh, play)
	}
	if b := f.bodyOf(play[0]); !strings.Contains(b, `"context_uri":"spotify:playlist:pl"`) {
		t.Fatalf("play body %s", b)
	}
	// Karıştırma önce açılır, sonra çalar.
	order := strings.Join(f.sent("PUT me/player/"), " | ")
	if strings.Index(order, "player/shuffle") > strings.Index(order, "player/play") {
		t.Fatalf("order: %s", order)
	}
}

func TestPastedOrClipboardLinkOpensList(t *testing.T) {
	f, m := playingModel(t)
	f.on("GET playlists/abc", `{"id":"abc","name":"Pasted","uri":"spotify:playlist:abc","owner":{"id":"o","display_name":"O"},"items":{"total":1}}`)
	f.on("GET playlists/abc/items", `{"total":1,"items":[{"item":{"uri":"spotify:track:p1","name":"P1"}}]}`)
	old := readClipboard
	readClipboard = func() (string, error) { return "https://open.spotify.com/playlist/abc?si=1", nil }
	t.Cleanup(func() { readClipboard = old })
	m.page = pageNow
	m = m.pressKey("v")
	if m.page != pageSearch || m.search.open == nil || m.search.open.entry.name != "Pasted" {
		t.Fatalf("page %v open %+v", m.page, m.search.open)
	}
	if len(m.search.open.tracks) != 1 {
		t.Fatal("tracks not loaded")
	}

	// Şarkı bağlantısı yapıştırılınca çalınır.
	out, cmd := m.Update(teaPaste("https://open.spotify.com/track/t1"))
	settle(out.(Model), cmd)
	play := f.sent("PUT me/player/play")
	if len(play) != 1 || !strings.Contains(f.bodyOf(play[0]), "spotify:track:t1") {
		t.Fatalf("paste play %v", play)
	}
}

func TestFindPlayingLoadsPagesUntilFound(t *testing.T) {
	f, m := playingModel(t)
	m.state.Item = &soloist.Entity{URI: "spotify:track:t70", EntityType: "track"}
	m.state.Context = &soloist.Entity{URI: "spotify:playlist:big", EntityType: "playlist"}
	e := libEntry{kind: kindPlaylist, key: "big", uri: "spotify:playlist:big", name: "Big", total: 100}
	m.lib.entries = []libEntry{e}
	var first []spotify.Track
	for i := 0; i < 50; i++ {
		first = append(first, spotify.Track{URI: fmt.Sprintf("spotify:track:t%d", i)})
	}
	var items []string
	for i := 50; i < 100; i++ {
		items = append(items, fmt.Sprintf(`{"item":{"uri":"spotify:track:t%d"}}`, i))
	}
	f.on("GET playlists/big/items", `{"total":100,"items":[`+strings.Join(items, ",")+`]}`)
	m.page, m.focus = pageLibrary, focusContent
	m.lib.open = &trackList{entry: e, tracks: first, total: 100}
	m = m.pressKey("c")
	if tl := m.lib.open; tl.cursor != 70 || tl.seek != "" {
		t.Fatalf("cursor %d seek %q tracks %d", tl.cursor, tl.seek, len(tl.tracks))
	}
}

func TestFindPlayingOpensContextFromAnywhere(t *testing.T) {
	f, m := playingModel(t)
	m.state.Context = &soloist.Entity{URI: "spotify:playlist:pl", EntityType: "playlist"}
	m.state.Item = &soloist.Entity{URI: "spotify:track:b", EntityType: "track"}
	m.lib.entries = []libEntry{{kind: kindPlaylist, key: "pl", uri: "spotify:playlist:pl", name: "List", total: 2}}
	f.on("GET playlists/pl/items", `{"total":2,"items":[{"item":{"uri":"spotify:track:a"}},{"item":{"uri":"spotify:track:b"}}]}`)
	m.page, m.focus = pageHome, focusMenu
	m = m.pressKey("c")
	if m.page != pageLibrary || m.lib.open == nil || m.lib.open.cursor != 1 {
		t.Fatalf("page %v open %+v", m.page, m.lib.open)
	}
}

func TestIPCMute(t *testing.T) {
	f, m := playingModel(t)
	m, r := ask(t, m, "mute")
	if !r.OK || r.Text != "Muted." || len(f.sent("PUT me/player/volume?volume_percent=0")) != 1 {
		t.Fatalf("%+v %v", r, f.sent("PUT"))
	}
	m.state.Volume = 0
	_, r = ask(t, m, "mute", "off")
	if !r.OK || !strings.Contains(r.Text, "40") || len(f.sent("PUT me/player/volume?volume_percent=40")) != 1 {
		t.Fatalf("%+v %v", r, f.sent("PUT"))
	}
	if _, r = ask(t, m, "mute", "loud"); r.OK {
		t.Fatal("bad arg accepted")
	}
}
