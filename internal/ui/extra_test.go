package ui

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestTopPageLoadsRangesAndPlays(t *testing.T) {
	f, m := playingModel(t)
	f.on("GET me/top/tracks", `{"items":[{"name":"One","uri":"spotify:track:1","album":{"uri":"spotify:album:a"}},{"name":"Two","uri":"spotify:track:2"}]}`)
	f.on("GET me/top/artists", `{"items":[{"name":"Ada","uri":"spotify:artist:ada"}]}`)
	f.on("GET me/player/devices", `{"devices":[]}`)
	m.page, m.focus = pageQueue, focusContent
	m = m.pressKey("g")
	m = m.pressKey("t")
	if !m.extraOn() || m.extra.page != extraTop || len(m.extra.items) != 6 {
		t.Fatalf("top page: on %v items %d", m.extraOn(), len(m.extra.items))
	}
	screen := ansi.Strip(strings.Join(m.extraView(90), "\n"))
	for _, want := range []string{"last 4 weeks", " 1  One", " 1  Ada", "Artists"} {
		if !strings.Contains(screen, want) {
			t.Fatalf("missing %q:\n%s", want, screen)
		}
	}
	m = m.pressKey("]")
	if got := f.sent("GET me/top/tracks?"); len(got) != 2 || !strings.Contains(got[1], "medium_term") {
		t.Fatalf("range: %v", got)
	}
	m = m.pressKey("enter") // ilk şarkı (One) albümünde çalar
	play := f.sent("PUT me/player/play")
	if len(play) != 1 || !strings.Contains(f.bodyOf(play[0]), "spotify:track:1") {
		t.Fatalf("play %v", play)
	}
	// a: eylem menüsü seçili şarkı için.
	m = m.pressKey("a")
	if !m.actions.open || m.actions.target.uri != "spotify:track:1" {
		t.Fatalf("actions target %+v", m.actions.target)
	}
	m = m.pressKey("esc")
	m = m.pressKey("esc") // ek sayfa kapanır, Sıra'ya dönülür
	if m.extraOn() || m.page != pageQueue {
		t.Fatalf("esc: extra %v page %v", m.extraOn(), m.page)
	}
}

func TestExtraClosesOnPageChange(t *testing.T) {
	f, m := playingModel(t)
	at := time.Now().Add(-2 * time.Hour)
	if at.Day() != time.Now().Day() { // gece yarısından hemen sonra "Today" kalsın
		at = time.Now()
	}
	f.on("GET me/player/recently-played", `{"items":[{"track":{"name":"Old","uri":"spotify:track:o"},"played_at":"`+at.UTC().Format(time.RFC3339)+`"}]}`)
	m.page, m.focus = pageHome, focusContent
	m = m.pressKey("g")
	m = m.pressKey("r")
	if m.extra.page != extraRecent || !strings.Contains(ansi.Strip(strings.Join(m.extraView(80), "\n")), "Today") {
		t.Fatalf("recent: %+v", m.extra)
	}
	m.km.Counts = false
	m = m.pressKey("4")
	if m.extraOn() || m.extra.page != extraNone {
		t.Fatal("extra page should close when the page changes")
	}
}

func TestLogPageShowsNotesAndWritesFile(t *testing.T) {
	_, m := playingModel(t)
	m = m.say("something broke")
	m = m.note("all good")
	m.page, m.focus = pageNow, focusMenu
	m = m.pressKey("g")
	m = m.pressKey("o")
	if m.extra.page != extraLog || len(m.extra.items) < 2 {
		t.Fatalf("log: %+v", m.extra.items)
	}
	if m.extra.items[0].name != "all good" { // en yeni üstte
		t.Fatalf("order: %v", m.extra.items[0].name)
	}
	b, err := os.ReadFile(logFile(m.cfg.LocalDir))
	if err != nil || !strings.Contains(string(b), "error  something broke") {
		t.Fatalf("file: %q %v", b, err)
	}
}

func TestHomeHasSeeAllRows(t *testing.T) {
	f, m := playingModel(t)
	f.on("GET me/top/tracks", `{"items":[]}`)
	f.on("GET me/top/artists", `{"items":[]}`)
	m.home.recent = nil
	m.page, m.focus = pageHome, focusContent
	m.home.topTracks = append(m.home.topTracks, m.home.topTracks...)
	out, _ := m.updateHome(homeMsg{})
	m = out
	m.home.items = append(m.home.items, searchItem{kind: "more", name: "→ x", section: "top"})
	m.home.cursor = len(m.home.items) - 1
	m = m.pressKey("enter")
	if m.extra.page != extraTop {
		t.Fatal("see-all row did not open the top page")
	}
}
