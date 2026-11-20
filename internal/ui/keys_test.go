package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/config"
	"github.com/Ehtisam/sotto/internal/soloist"
)

func typingModel(t *testing.T) Model {
	t.Helper()
	dir := t.TempDir()
	m := New(config.Config{ConfigDir: dir, LocalDir: dir, Spotify: config.Spotify{ClientID: "x"}})
	m.spState = spReady
	m.page, m.focus = pageSearch, focusContent
	m.search.typing = true
	return m
}

func press(m Model, text string, code rune) Model {
	out, _ := m.Update(tea.KeyPressMsg{Text: text, Code: code})
	return out.(Model)
}

func TestTypingDoesNotTriggerShortcuts(t *testing.T) {
	m := typingModel(t)
	for _, r := range "nsöq" {
		m = press(m, string(r), r)
	}
	m = press(m, " ", tea.KeySpace)
	if m.search.query != "nsöq " {
		t.Fatalf("query = %q", m.search.query)
	}
	if m.page != pageSearch || m.focus != focusContent {
		t.Fatal("a letter left the search page")
	}
}

func TestBackspaceRemovesWholeRune(t *testing.T) {
	m := typingModel(t)
	m = press(m, "ş", 'ş')
	m = press(m, "", tea.KeyBackspace)
	if m.search.query != "" {
		t.Fatalf("query = %q", m.search.query)
	}
}

func TestEscClearsThenLeaves(t *testing.T) {
	m := typingModel(t)
	m = press(m, "a", 'a')
	m = press(m, "", tea.KeyEscape)
	if m.search.query != "" || m.focus != focusContent {
		t.Fatal("first esc should only clear the query")
	}
	m = press(m, "", tea.KeyEscape)
	if m.focus != focusMenu {
		t.Fatal("second esc should go back to the menu")
	}
}

func TestSlashOpensSearch(t *testing.T) {
	dir := t.TempDir()
	m := New(config.Config{ConfigDir: dir, LocalDir: dir})
	m.wiz.on = false
	m = press(m, "/", '/')
	if m.page != pageSearch || m.focus != focusContent || !m.search.typing {
		t.Fatal("/ should open search with the input focused")
	}
}

func TestLyricsKeyGoesToNowPlaying(t *testing.T) {
	dir := t.TempDir()
	m := New(config.Config{ConfigDir: dir, LocalDir: dir})
	m.wiz.on = false
	m.page, m.focus = pageLibrary, focusContent
	m.lyr.showQueue = true
	m = press(m, "l", 'l')
	if m.page != pageNow || m.lyr.showQueue {
		t.Fatal("l should open the lyrics on Now playing")
	}
	m = press(m, "l", 'l')
	if !m.lyr.showQueue {
		t.Fatal("second l should switch to up next")
	}
}

// Alt şeritteki simge Spotify'daki düğme gibi: çalarken ❚❚, dururken ▶.
func TestStripIconIsTheButton(t *testing.T) {
	m := homeModel(t)
	m.state.Item = entity("song")
	m.state.Status = soloist.StatusPlaying
	if s := m.strip(100); !strings.Contains(s, "❚❚") || strings.Contains(s, "▶") {
		t.Fatalf("playing strip = %q", s)
	}
	m.state.Status = soloist.StatusPaused
	if s := m.strip(100); !strings.Contains(s, "▶") || strings.Contains(s, "❚❚") {
		t.Fatalf("paused strip = %q", s)
	}
}
