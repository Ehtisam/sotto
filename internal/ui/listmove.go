package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Kendi (ya da ortak) listende şarkıyı shift+J / shift+K ile bir yukarı/aşağı taşımak.
// Ekran hemen güncellenir; Spotify reddederse liste yeniden yüklenir.

type plMovedMsg struct {
	key  string
	err  error
	note string // başarılıysa gösterilecek not
}

func (m Model) moveInList(tl *trackList, d int) tea.Cmd {
	if tl.entry.kind != kindPlaylist || !m.editable(tl.entry) || tl.moving {
		return nil
	}
	i, j := tl.cursor, tl.cursor+d
	if i < 0 || j < 0 || i >= len(tl.tracks) || j >= len(tl.tracks) {
		return nil
	}
	tl.tracks[i], tl.tracks[j] = tl.tracks[j], tl.tracks[i]
	tl.changed()
	tl.cursor = j
	tl.clamp(m.listHeight())
	tl.moving = true
	// Spotify'da "önüne koy": aşağı taşırken bir sonrakinin ardına, yukarı
	// taşırken bir öncekinin önüne.
	before := j
	if d > 0 {
		before = j + 1
	}
	sp, key := m.sp, tl.entry.key
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return plMovedMsg{key: key, err: sp.MovePlaylistItem(ctx, key, i, before)}
	}
}

func (m Model) updateListMove(msg plMovedMsg) (Model, tea.Cmd) {
	var cmds []tea.Cmd
	for _, tl := range m.openLists() {
		if tl == nil || tl.entry.key != msg.key {
			continue
		}
		tl.moving = false
		if msg.err != nil {
			// Gerçek sırayı göster.
			tl.tracks, tl.loading = nil, true
			cmds = append(cmds, m.loadTracks(tl.entry, 0))
		}
	}
	switch {
	case msg.err != nil:
		m = m.say(msg.err.Error())
	case msg.note != "":
		m = m.note(msg.note)
	}
	return m, tea.Batch(cmds...)
}

// removeFromList, kendi (ya da ortak) listende seçili şarkıyı çıkarır (x).
// Ekran hemen güncellenir; Spotify reddederse liste yeniden yüklenir.
func (m Model) removeFromList(tl *trackList) tea.Cmd {
	if tl.entry.kind != kindPlaylist || !m.editable(tl.entry) || tl.moving || len(tl.tracks) == 0 {
		return nil
	}
	t := tl.tracks[tl.cursor]
	tl.remove(t.URI)
	tl.clamp(m.listHeight())
	tl.moving = true
	sp, key, list := m.sp, tl.entry.key, tl.entry.name
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		err := sp.RemoveFromPlaylist(ctx, key, []string{t.URI})
		return plMovedMsg{key: key, err: err, note: F("act.removed", list)}
	}
}
