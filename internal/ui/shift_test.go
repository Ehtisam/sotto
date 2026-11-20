package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestShiftHintAndModifiers(t *testing.T) {
	m := playlistModel(t)
	m.width, m.height = 110, 32
	m.page, m.focus = pageLibrary, focusContent
	m.lib.open = &trackList{entry: m.lib.entries[0], tracks: nil}

	out, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyLeftShift})
	m = out.(Model)
	h := m.hints(110)
	if !strings.Contains(h, "shift held") || !strings.Contains(h, "move song down") {
		t.Fatalf("hint while shift held: %q", h)
	}
	out, _ = m.Update(tea.KeyReleaseMsg{Code: tea.KeyLeftShift})
	m = out.(Model)
	if strings.Contains(m.hints(110), "shift held") {
		t.Fatal("hint stayed after release")
	}

	// Silme sorusu açıkken ctrl'e tek başına basmak soruyu kapatmamalı.
	m.lib.open = nil
	m.actions = actionMenu{open: true, target: entryItem(m.lib.entries[0]), confirm: true}
	out, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyLeftCtrl})
	if a := out.(Model).actions; !a.open || !a.confirm {
		t.Fatal("a lone ctrl press cancelled the question")
	}
}
