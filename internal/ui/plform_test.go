package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/spotify"
)

func playlistModel(t *testing.T) Model {
	t.Helper()
	m := queueModel(t)
	m.me.ID = "me"
	m.lib.entries = []libEntry{
		{kind: kindPlaylist, key: "p1", uri: "spotify:playlist:p1", name: "Mine", desc: "old", ownerID: "me", public: true},
		{kind: kindPlaylist, key: "p2", uri: "spotify:playlist:p2", name: "Theirs", ownerID: "x"},
	}
	return m
}

func typeText(m Model, s string) Model {
	for _, r := range s {
		m, _ = m.formKey(tea.KeyPressMsg{Text: string(r), Code: r})
	}
	return m
}

func formPress(m Model, k string) (Model, tea.Cmd) {
	var msg tea.KeyPressMsg
	switch k {
	case "enter":
		msg = tea.KeyPressMsg{Code: tea.KeyEnter}
	case "down":
		msg = tea.KeyPressMsg{Code: tea.KeyDown}
	case "right":
		msg = tea.KeyPressMsg{Code: tea.KeyRight}
	case "backspace":
		msg = tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "esc":
		msg = tea.KeyPressMsg{Code: tea.KeyEscape}
	}
	out, cmd := m.update(msg) // Update'in gözlemci komutları (son çalınan kaydı) burada ölçülmüyor
	return out.(Model), cmd
}

func TestEditOnlyOwnPlaylists(t *testing.T) {
	m := playlistModel(t)
	has := func(e libEntry) bool {
		m.actions = actionMenu{open: true, target: entryItem(e), saved: 1}
		for _, it := range m.menuItems() {
			if it.id == actEdit {
				return true
			}
		}
		return false
	}
	if !has(m.lib.entries[0]) || has(m.lib.entries[1]) {
		t.Fatal("edit should show only on your own playlist")
	}
}

func TestNewPlaylistFromPicker(t *testing.T) {
	m := playlistModel(t)
	m.actions = actionMenu{open: true, target: trackItem(spotify.Track{URI: "spotify:track:x", Name: "X"}), saved: 0}
	m.actions.items = m.menuItems()
	m, _ = m.runAction(actAddList)
	if !m.actions.picking || len(m.actions.lists) != 1 {
		t.Fatalf("picker: %+v", m.actions)
	}
	if v := strings.Join(m.actionsView(60), "\n"); !strings.Contains(v, "New playlist") || !strings.Contains(v, "Mine") {
		t.Fatalf("picker view:\n%s", v)
	}
	m, _ = m.actionsKey("enter") // ilk satır: yeni liste
	if !m.form.on || !m.form.create || m.form.addURI != "spotify:track:x" || m.actions.open {
		t.Fatalf("form = %+v", m.form)
	}

	// Ad boşken kaydedilmez.
	m.form.row = len(m.form.rows()) - 1
	m, cmd := formPress(m, "enter")
	if cmd != nil || m.form.err == "" || m.form.row != 0 {
		t.Fatalf("empty name: err %q row %d", m.form.err, m.form.row)
	}
	m = typeText(m, "Yolda ş")
	m, _ = formPress(m, "backspace")
	if m.form.name != "Yolda " {
		t.Fatalf("name = %q", m.form.name)
	}
	m, _ = formPress(m, "down")
	m, _ = formPress(m, "down")
	m, _ = formPress(m, "right") // gizli → herkese açık
	if m.form.vis != visPublic {
		t.Fatalf("vis = %d", m.form.vis)
	}
	m, _ = formPress(m, "down") // kaydet
	m, cmd = formPress(m, "enter")
	if cmd == nil || !m.form.busy {
		t.Fatal("save did not send")
	}

	// Cevap Update'ten geçer; kitaplık yeniden istenir.
	out, cmd := m.Update(plSavedMsg{create: true, key: "new", name: "Yolda"})
	m = out.(Model)
	if m.form.on || cmd == nil || !strings.Contains(m.flash, "Yolda") {
		t.Fatalf("after create: form %v flash %q", m.form.on, m.flash)
	}
}

func TestEditDetails(t *testing.T) {
	m := playlistModel(t)
	e := m.lib.entries[0]
	m = m.openPlaylistForm(&e, "")
	if m.form.vis != visPublic || len(m.form.rows()) != 5 {
		t.Fatalf("form %+v", m.form)
	}
	// Değişiklik yoksa sadece kapanır.
	m.form.row = len(m.form.rows()) - 1
	if mm, cmd := m.submitPlaylistForm(); cmd != nil || mm.form.on {
		t.Fatal("nothing changed but it saved")
	}
	m.form.desc = "new words"
	mm, cmd := m.submitPlaylistForm()
	if cmd == nil {
		t.Fatal("no request")
	}
	desc := "new words"
	out, _ := mm.Update(plSavedMsg{key: "p1", name: "Mine", saved: true, details: spotify.PlaylistDetails{Description: &desc}})
	mm = out.(Model)
	if mm.lib.entries[0].desc != "new words" || mm.form.on {
		t.Fatalf("entry %+v form %v", mm.lib.entries[0], mm.form.on)
	}

	// Kapak izni yoksa form açık kalır, ayrıntılar yine de işlenir, ctrl+l teklif edilir.
	mm = m
	mm.form.busy = true
	out, _ = mm.Update(plSavedMsg{key: "p1", name: "Mine", saved: true, scope: true, err: errors.New("403"),
		details: spotify.PlaylistDetails{Description: &desc}})
	mm = out.(Model)
	if !mm.form.on || !mm.form.auth || mm.lib.entries[0].desc != "new words" {
		t.Fatalf("scope: form %+v", mm.form)
	}
	if !strings.Contains(strings.Join(mm.playlistFormView(80), "\n"), "ctrl+l") {
		t.Fatal("relogin hint missing")
	}
}

func TestMoveInList(t *testing.T) {
	m := playlistModel(t)
	tl := &trackList{entry: m.lib.entries[0], tracks: []spotify.Track{{URI: "a"}, {URI: "b"}, {URI: "c"}}, total: 3}
	m.lib.open = tl
	m.page, m.focus = pageLibrary, focusContent
	cmd, handled, _ := m.listKey(tl, "J")
	if !handled || cmd == nil || tl.tracks[1].URI != "a" || tl.cursor != 1 {
		t.Fatalf("move down: %v cursor %d", tl.tracks, tl.cursor)
	}
	if cmd, _, _ := m.listKey(tl, "J"); cmd != nil {
		t.Fatal("moved again while busy")
	}
	out, cmd := m.Update(plMovedMsg{key: "p1", err: errors.New("nope")})
	m = out.(Model)
	if tl.moving || cmd == nil || !tl.loading || m.flash != "nope" {
		t.Fatalf("after error: moving %v loading %v flash %q", tl.moving, tl.loading, m.flash)
	}
	// Başkasının listesinde taşınmaz.
	other := &trackList{entry: m.lib.entries[1], tracks: []spotify.Track{{URI: "a"}, {URI: "b"}}}
	if cmd, _, _ := m.listKey(other, "J"); cmd != nil || other.tracks[0].URI != "a" {
		t.Fatal("moved someone else's playlist")
	}
}
