package ui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Kaydırılan her sayfada sağda kaydırma çubuğu olmalı ve hiçbir satır
// sayfa genişliğini aşmamalı (çubuk satırı kaydırmasın).
func TestScrollbarOnEveryScrollingPage(t *testing.T) {
	_, m := newFakeWeb(t)
	m.height = 20
	const w = 80
	knob := "┃"
	check := func(name string, rows []string, want bool) {
		t.Helper()
		s := ansi.Strip(strings.Join(rows, "\n"))
		if got := strings.Contains(s, knob); got != want {
			t.Errorf("%s: scrollbar %v, want %v:\n%s", name, got, want, s)
		}
		for _, r := range rows {
			if lipgloss.Width(r) > w {
				t.Errorf("%s: row wider than %d: %q", name, w, ansi.Strip(r))
			}
		}
	}

	m.page, m.focus = pageSettings, focusContent
	check("settings", m.settingsView(w, 14), true)
	// Tema ve vurgu renginin örnekleri (■■■) çubuk yüzünden kırpılmamalı.
	if s := ansi.Strip(strings.Join(m.settingsView(w, 14), "\n")); strings.Count(s, "■■■") != 2 {
		t.Errorf("swatches cut:\n%s", s)
	}
	check("settings tall", m.settingsView(w, 400), false)
	check("help", m.helpView(w, 12), true)

	var notes []string
	for i := 0; i < 40; i++ {
		notes = append(notes, fmt.Sprintf("- note %d", i))
	}
	m.whatsnew = whatsNew{open: true, version: "0.2.0", notes: []versionNotes{{version: "0.2.0", lines: notes}}}
	check("what's new", m.whatsNewView(w, 14), true)
	m.whatsnew = whatsNew{}

	var items []searchItem
	for i := 0; i < 40; i++ {
		items = append(items, searchItem{kind: "track", name: fmt.Sprintf("Song %d", i), sub: "Artist"})
	}
	check("profile panel", strings.Split(m.profilePanel("Top songs", items, 0, 0, 38, 6, true), "\n"), true)
	check("profile panel short", strings.Split(m.profilePanel("Top songs", items[:3], 0, 0, 38, 6, true), "\n"), false)

	m.extra.page = extraLog
	for i := 0; i < 60; i++ {
		m.extra.items = append(m.extra.items, searchItem{kind: "log", name: fmt.Sprintf("line %d", i), sub: "12:00:00"})
	}
	check("log", m.extraView(w), true)
	m.extra = extraState{}

	m.actions.picking = true
	for i := 0; i < 60; i++ {
		m.actions.lists = append(m.actions.lists, libEntry{name: fmt.Sprintf("List %d", i), total: i})
	}
	check("add to playlist", m.actionsView(w), true)
	m.actions.picking = false

	for i := 0; i < 60; i++ {
		m.actions.folders = append(m.actions.folders, folderChoice{id: fmt.Sprint(i), name: fmt.Sprintf("Folder %d", i), depth: 1})
	}
	check("move to folder", m.folderPickView(w), true)
}
