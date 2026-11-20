package ui

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/spotify"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// IV · Kitaplık: listeler ve beğenilenler; birine girilince şarkıları.

type library struct {
	loading bool
	loaded  bool
	err     error
	entries []libEntry
	cursor  int
	scroll  int

	open *trackList // nil: liste düzeyi
}

type playlistsMsg struct {
	entries []libEntry
	err     error
}

func (m Model) loadLibrary() tea.Cmd {
	sp := m.sp
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		liked, err := sp.LikedTracks(ctx, 0, 1)
		if err != nil {
			return playlistsMsg{err: err}
		}
		entries := []libEntry{{kind: kindLiked, key: likedKey, name: T("lib.liked"), total: liked.Total}}
		for offset := 0; offset < 2000; offset += pageSize {
			p, err := sp.MyPlaylists(ctx, offset, pageSize)
			if err != nil {
				return playlistsMsg{entries: entries, err: err}
			}
			for _, pl := range p.Items {
				if pl.ID == "" {
					continue
				}
				entries = append(entries, playlistEntry(pl))
			}
			if p.Next == "" || len(p.Items) == 0 {
				break
			}
		}
		return playlistsMsg{entries: entries}
	}
}

func playlistEntry(pl spotify.Playlist) libEntry {
	return libEntry{
		kind: kindPlaylist, key: pl.ID, uri: pl.URI, name: pl.Name,
		owner: pl.Owner.DisplayName, desc: cleanDesc(pl.Description),
		image: pickImage(pl.Images), total: pl.Total(),
	}
}

func (m Model) updateLibrary(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case playlistsMsg:
		m.lib.loading = false
		if msg.err != nil {
			m.lib.err = msg.err
			m = m.spotifyFailed(msg.err)
		}
		if len(msg.entries) > 0 {
			m.lib.entries, m.lib.loaded = msg.entries, true
		}
		m.lib.clamp(m.listHeight())
	case tracksPageMsg:
		for _, tl := range []*trackList{m.lib.open, m.search.open} {
			if tl.receive(msg) {
				if msg.err != nil {
					return m.spotifyFailed(msg.err), nil
				}
				tl.clamp(m.listHeight())
			}
		}
		for _, t := range msg.tracks {
			m.rt.known[t.URI] = t
		}
	case playedMsg:
		return m.played(msg)
	}
	return m, nil
}

// loginKeys, Spotify hesabı bağlı değilken sayfaların ortak tuşları.
func (m Model) loginKeys(k string) (Model, tea.Cmd, bool) {
	switch k {
	case "enter":
		if m.spState == spLoggedOut {
			m.spState = spLoggingIn
			return m, m.login(), true
		}
		return m, nil, true
	case "esc", "left", "backspace":
		m.focus = focusMenu
		return m, nil, true
	}
	return m, nil, false
}

func (m Model) libraryKey(k string) (Model, tea.Cmd, bool) {
	if m.spState != spReady {
		return m.loginKeys(k)
	}
	l := &m.lib
	if l.open != nil {
		cmd, handled, back := m.listKey(l.open, k)
		if back {
			l.open = nil
		}
		return m, cmd, handled
	}

	h := m.listHeight()
	move := func(d int) {
		l.cursor = max(0, min(len(l.entries)-1, l.cursor+d))
		l.clamp(h)
	}
	switch k {
	case "up", "k":
		move(-1)
	case "down", "j":
		move(1)
	case "pgup":
		move(-h)
	case "pgdown":
		move(h)
	case "home", "g":
		move(-1 << 30)
	case "end", "G":
		move(1 << 30)
	case "enter", "right":
		if len(l.entries) == 0 {
			if !l.loading {
				l.loading = true
				return m, m.loadLibrary(), true
			}
			return m, nil, true
		}
		tl, cmd := m.openList(l.entries[l.cursor])
		l.open = tl
		return m, cmd, true
	case "esc", "left", "backspace":
		m.focus = focusMenu
	default:
		return m, nil, false
	}
	return m, nil, true
}

func (l *library) clamp(h int) {
	l.cursor, l.scroll = fitScroll(l.cursor, l.scroll, len(l.entries), h)
	if l.open != nil {
		l.open.clamp(h)
	}
}

// ---- çizim ----

// accountView, Spotify hesabı hazır değilse sayfaların göstereceği satırlar.
func (m Model) accountView(title string) []string {
	switch m.spState {
	case spNoClient:
		return []string{title, "",
			theme.Plain.Render(T("lib.noclient")),
			theme.Faint.Render(T("lib.noclient.how")) + theme.Dim.Render(m.cfg.ConfigFile()),
		}
	case spLoggedOut:
		rows := []string{title, "",
			theme.Plain.Render(T("lib.loggedout")),
			theme.Dim.Render(T("lib.login.how")),
		}
		if m.spErr != nil && !errors.Is(m.spErr, spotify.ErrNotLoggedIn) {
			rows = append(rows, "", theme.Error.Render(m.spErr.Error()))
		}
		return rows
	case spLoggingIn:
		return []string{title, "",
			theme.Plain.Render(T("lib.waiting")),
			theme.Faint.Render(T("lib.waiting.how")),
		}
	}
	return nil
}

func (m Model) libraryView(w, h int) []string {
	title := theme.Title.Render(T("page.library"))
	if rows := m.accountView(title); rows != nil {
		return rows
	}
	l := m.lib
	active := m.focus == focusContent
	if l.open != nil {
		return m.listView(l.open, w, active)
	}

	playingCtx := ""
	if m.state.Context != nil {
		playingCtx = m.state.Context.URI
	}
	sub := theme.Dim.Render(T("lib.loading"))
	if l.loaded {
		sub = theme.Dim.Render(F("lib.playlists", len(l.entries)-1)) + theme.Faint.Render(T("lib.plus.liked"))
	} else if l.err != nil {
		sub = theme.Error.Render(l.err.Error())
	}
	rows := []string{title, sub, "", ""}
	end := min(len(l.entries), l.scroll+m.listHeight())
	for i := l.scroll; i < end; i++ {
		e := l.entries[i]
		mark := "  "
		if e.uri != "" && e.uri == playingCtx {
			mark = theme.GoldText.Render("♪ ")
		}
		name := e.name
		if e.kind == kindLiked {
			name = "♥ " + name
		}
		rows = append(rows, mark+row(name, F("lib.songs", e.total), w-2, active && i == l.cursor))
	}
	return rows
}

// likedName, beğenilenlerin geçerli dildeki adı.
func (m Model) likedName() string { return T("lib.liked") }
