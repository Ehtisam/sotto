package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Ehtisam/sotto/internal/keys"
	"github.com/Ehtisam/sotto/internal/spotify"
)

func ctrl(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl} }

func longList(t *testing.T) (*fakeWeb, Model) {
	f, m := playingModel(t)
	e := libEntry{kind: kindPlaylist, key: "pl", uri: "spotify:playlist:pl", name: "List", ownerID: "me", total: 30}
	m.lib.entries = []libEntry{e}
	var ts []spotify.Track
	for i := 0; i < 30; i++ {
		ts = append(ts, spotify.Track{Name: fmt.Sprintf("Song %02d", i), URI: fmt.Sprintf("spotify:track:t%d", i)})
	}
	m.page, m.focus = pageLibrary, focusContent
	m.lib.open = &trackList{entry: e, tracks: ts, total: 30}
	return f, m
}

func TestSequenceGoesToPage(t *testing.T) {
	_, m := longList(t)
	m = m.pressKey("g")
	if m.seq != "g" || m.page != pageLibrary {
		t.Fatalf("g alone: seq %q page %v", m.seq, m.page)
	}
	if !strings.Contains(ansi.Strip(m.hints(120)), "g…") {
		t.Fatal("pending g not shown")
	}
	m = m.pressKey("q")
	if m.page != pageQueue || m.seq != "" {
		t.Fatalf("g q: page %v seq %q", m.page, m.seq)
	}
}

func TestGGGoesToTopAndBrokenSequenceIsDropped(t *testing.T) {
	_, m := longList(t)
	m.lib.open.cursor = 20
	m = m.pressKey("g")
	m = m.pressKey("g")
	if m.lib.open.cursor != 0 {
		t.Fatalf("gg cursor %d", m.lib.open.cursor)
	}
	m.lib.open.cursor = 5
	m = m.pressKey("g")
	m = m.pressKey("x") // g x diye bir şey yok: ikisi de yok sayılır (x silmez)
	if m.seq != "" || len(m.lib.open.tracks) != 30 {
		t.Fatalf("broken sequence did something: seq %q tracks %d", m.seq, len(m.lib.open.tracks))
	}
	// Zaman aşımı yarım diziyi siler.
	m = m.pressKey("g")
	out, _ := m.Update(seqTimeoutMsg{at: m.seqAt})
	if out.(Model).seq != "" {
		t.Fatal("timeout did not clear the sequence")
	}
}

func TestCountsRepeatMoves(t *testing.T) {
	_, m := longList(t)
	m = m.pressKey("1")
	m = m.pressKey("2")
	if m.count != "12" || m.page != pageLibrary {
		t.Fatalf("count %q page %v", m.count, m.page)
	}
	m = m.pressKey("j")
	if m.lib.open.cursor != 12 || m.count != "" {
		t.Fatalf("12j cursor %d", m.lib.open.cursor)
	}
	m = m.pressKey("5")
	m = m.pressKey("k")
	if m.lib.open.cursor != 7 {
		t.Fatalf("5k cursor %d", m.lib.open.cursor)
	}
}

func TestLoneDigitStillGoesToPage(t *testing.T) {
	_, m := longList(t)
	m = m.pressKey("5")
	if m.page != pageLibrary {
		t.Fatal("digit jumped before the wait")
	}
	out, cmd := m.Update(countTimeoutMsg{at: m.countAt})
	m = settle(out.(Model), cmd)
	if m.page != pageQueue {
		t.Fatalf("5 then wait → page %v", m.page)
	}
	// Rakamdan sonra sayılamayan bir tuş: önce sayfa, sonra tuş.
	_, m = longList(t)
	m = m.pressKey("6")
	m = m.pressKey("?")
	if m.page != pageDevices || !m.help {
		t.Fatalf("6? → page %v help %v", m.page, m.help)
	}
	// Menüdeyken rakam hemen gider.
	_, m = longList(t)
	m.focus = focusMenu
	m = m.pressKey("5")
	if m.page != pageQueue {
		t.Fatal("digit in the menu should jump at once")
	}
}

func TestRemappedKeys(t *testing.T) {
	f, m := longList(t)
	m.km = keys.Parse([]byte("[keys]\nnext = \"ctrl+n\"\ndown = [\"e\", \"down\"]\n"))
	m.page, m.focus = pageNow, focusMenu // Kitaplık'ta n yeni liste açar; burada sonraki şarkı
	out, cmd := m.Update(ctrl('n'))
	m = settle(out.(Model), cmd)
	if len(f.sent("POST me/player/next")) != 1 {
		t.Fatalf("ctrl+n did not skip: %v", f.sent(""))
	}
	m = m.pressKey("n") // artık boşta
	if len(f.sent("POST me/player/next")) != 1 {
		t.Fatal("n still skips")
	}
	m.page, m.focus = pageLibrary, focusContent
	m = m.pressKey("e")
	if m.lib.open.cursor != 1 {
		t.Fatalf("e should move down, cursor %d", m.lib.open.cursor)
	}
	m = m.pressKey("j") // j artık boşta
	if m.lib.open.cursor != 1 {
		t.Fatal("j still moves")
	}
	if !strings.Contains(ansi.Strip(m.hints(200)), "ctrl+n") {
		t.Fatalf("hints show old key: %s", ansi.Strip(m.hints(200)))
	}
	help := ansi.Strip(strings.Join(m.helpView(120, 200), "\n"))
	if !strings.Contains(help, "e  ↓") || !strings.Contains(help, "ctrl+n") {
		t.Fatalf("help:\n%s", help)
	}
}

func TestMenusKeepTheirOwnLetters(t *testing.T) {
	_, m := longList(t)
	m.km = keys.Parse([]byte("[keys]\nnext = \"q\"\n"))
	m = m.pressKey("O") // sıralama menüsü
	m = m.pressKey("q") // menüde q: kapat (next'e çevrilmez)
	if m.sortm.open {
		t.Fatal("q should close the sort menu")
	}
	// Yazarken tuşlar çevrilmez.
	m = m.pressKey("/")
	m = typeFind(m, "qn")
	if m.lib.open.find.query != "qn" {
		t.Fatalf("query %q", m.lib.open.find.query)
	}
}

func TestHelpScrolls(t *testing.T) {
	_, m := longList(t)
	m.width, m.height = 80, 20
	m = m.pressKey("?")
	if !m.help {
		t.Fatal("? did not open help")
	}
	first := ansi.Strip(strings.Join(m.helpView(m.contentSize()), "\n"))
	m = m.pressKey("j")
	m = m.pressKey("j")
	if m.helpScroll != 2 {
		t.Fatalf("scroll %d", m.helpScroll)
	}
	if ansi.Strip(strings.Join(m.helpView(m.contentSize()), "\n")) == first {
		t.Fatal("help did not move")
	}
	for i := 0; i < 200; i++ {
		m = m.pressKey("j")
	}
	end := m.helpScroll
	m = m.pressKey("k")
	if m.helpScroll != end-1 {
		t.Fatalf("scroll should stop at the end: %d then %d", end, m.helpScroll)
	}
}

// Kullanıcı buldu: ? listesi açıkken g n sonraki şarkıya geçiyordu.
func TestSequencesWorkInHelp(t *testing.T) {
	f, m := longList(t)
	m = m.pressKey("?")
	m = m.pressKey("g")
	if m.seq != "g" {
		t.Fatalf("g in help: seq %q", m.seq)
	}
	m = m.pressKey("n")
	if m.page != pageNow || m.help {
		t.Fatalf("g n in help → page %v help %v", m.page, m.help)
	}
	if len(f.sent("POST me/player/next")) != 0 {
		t.Fatal("g n skipped the song")
	}
	// g t de ? listesinden açılır; g g listede en üste kaydırır.
	m = m.pressKey("?")
	for i := 0; i < 5; i++ {
		m = m.pressKey("j")
	}
	m = m.pressKey("g")
	m = m.pressKey("g")
	if !m.help || m.helpScroll != 0 {
		t.Fatalf("g g in help: help %v scroll %d", m.help, m.helpScroll)
	}
}

// Eylem menüsünde g radyodur, dizi başlatmaz.
func TestActionMenuLettersAreNotSequences(t *testing.T) {
	_, m := longList(t)
	m.cfg.Extras.Enabled = true
	m = m.pressKey("a")
	if !m.actions.open {
		t.Fatal("a did not open the menu")
	}
	m = m.pressKey("g")
	if m.seq != "" || m.actions.open || m.flash == "" { // radyo başladı (testte go-librespot yok, onun notu da olur)
		t.Fatalf("g in the action menu: seq %q open %v flash %q", m.seq, m.actions.open, m.flash)
	}
}
