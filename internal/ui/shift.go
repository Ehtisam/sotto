package ui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Shift basılıyken alt satır, o an bulunulan yerde shift'le çalışan tuşları
// gösterir. Terminal shift'e tek başına basılmasını yalnızca kitty klavye
// protokolüyle bildirir (View bunu ister; Windows Terminal destekliyor).

// shiftTimeout: bırakma bilgisi kaybolursa (pencere odağı gitti) ipucu yine kalkar.
const shiftTimeout = 15 * time.Second

func isShiftKey(s string) bool { return s == "leftshift" || s == "rightshift" }

// Tek başına basılan değiştirici tuşlar (kitty modunda ayrı tuş olarak gelir);
// hiçbir şey yapmamalı (örn. silme sorusunu iptal etmemeli).
var modifierKeys = map[string]bool{
	"leftctrl": true, "rightctrl": true, "leftalt": true, "rightalt": true,
	"leftsuper": true, "rightsuper": true, "lefthyper": true, "righthyper": true,
	"leftmeta": true, "rightmeta": true, "capslock": true, "numlock": true,
	"scrolllock": true, "isolevel3shift": true, "isolevel5shift": true,
}

// shiftEvent, shift'e basma/bırakmayı işler. ok=false: başka bir tuş.
func (m Model) shiftEvent(msg tea.Msg) (Model, bool) {
	switch k := msg.(type) {
	case tea.KeyPressMsg:
		if isShiftKey(k.String()) {
			m.shiftAt = m.now
			if m.shiftAt.IsZero() {
				m.shiftAt = time.Now()
			}
			return m, true
		}
		if modifierKeys[k.String()] {
			return m, true
		}
	case tea.KeyReleaseMsg:
		if isShiftKey(k.String()) {
			m.shiftAt = time.Time{}
		}
		return m, true // diğer bırakmalar önemsiz
	}
	return m, false
}

func (m Model) shiftHeld() bool {
	return !m.shiftAt.IsZero() && m.now.Sub(m.shiftAt) < shiftTimeout
}

// shiftHints, bulunulan yerde shift'le çalışan tuşlar.
func (m Model) shiftHints() []hint {
	var hs []hint
	switch {
	case m.form.on, m.actions.open, m.wiz.on:
		return nil
	case m.page == pageSearch && m.focus == focusContent && m.search.typing && m.search.open == nil:
		return nil // yazarken büyük harf yazıya gider
	}
	tl := m.shownList()
	switch {
	case tl != nil && tl.entry.kind == kindPlaylist && m.editable(tl.entry) && m.focus == focusContent:
		hs = append(hs, hint{"J", T("shift.song.down")}, hint{"K", T("shift.song.up")})
	case m.page == pageQueue && m.focus == focusContent && m.cfg.Extras.Enabled:
		hs = append(hs, hint{"J", T("shift.song.down")}, hint{"K", T("shift.song.up")})
	case m.page == pageLibrary && m.focus == focusContent && tl == nil && m.foldersOn() && m.lib.filter == libAll:
		hs = append(hs, hint{"J", T("shift.item.down")}, hint{"K", T("shift.item.up")})
	}
	if m.focus == focusContent && (tl != nil || m.page == pageLibrary || m.page == pageQueue) {
		hs = append(hs, hint{"G", T("shift.end")})
	}
	if m.focus == focusContent && tl == nil && (m.page == pageQueue || m.page == pageHome) {
		hs = append(hs, hint{"R", T("shift.refresh")})
	}
	hs = append(hs, hint{"N", T("hint.newlist")}, hint{"L", T("shift.lyrics")})
	return hs
}

// shownList, bulunulan sayfada açık olan liste (yoksa nil).
func (m Model) shownList() *trackList {
	switch m.page {
	case pageLibrary:
		return m.lib.open
	case pageSearch:
		return m.search.open
	case pageHome:
		return m.home.open
	case pageProfile:
		return m.prof.open
	}
	return nil
}

// shiftHintRow, alt satırın shift basılıyken hali.
func (m Model) shiftHintRow(render func(hint) string) string {
	hs := m.shiftHints()
	if len(hs) == 0 {
		return ""
	}
	parts := []string{T("shift.title")}
	for _, h := range hs {
		parts = append(parts, render(hint{"⇧" + h.key, h.verb}))
	}
	return strings.Join(parts, "  ")
}
