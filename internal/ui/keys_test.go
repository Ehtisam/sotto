package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/config"
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
	m = press(m, "/", '/')
	if m.page != pageSearch || m.focus != focusContent || !m.search.typing {
		t.Fatal("/ should open search with the input focused")
	}
}
