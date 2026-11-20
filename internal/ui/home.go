package ui

import (
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/spotify"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// II · Ana sayfa: kaldığın yerden devam, son çalınanlar, bu ay en çok
// dinlediğin şarkılar ve sanatçılar.

const homeRefresh = 2 * time.Minute

type homeState struct {
	loading bool
	fetched time.Time
	err     error

	recent     []spotify.PlayHistory
	topTracks  []spotify.Track
	topArtists []spotify.FullArtist

	items  []searchItem
	cursor int
	scroll int

	open *trackList
}

type homeMsg struct {
	recent     []spotify.PlayHistory
	topTracks  []spotify.Track
	topArtists []spotify.FullArtist
	err        error
}

func (m Model) loadHome() tea.Cmd {
	rt := m.rt
	if m.spState != spReady {
		return nil
	}
	sp := m.sp
	return func() tea.Msg {
		ctx, cancel := apiCtx(rt, 20*time.Second)
		defer cancel()
		var msg homeMsg
		var wg sync.WaitGroup
		var e1, e2, e3 error
		wg.Add(3)
		go func() { defer wg.Done(); msg.recent, e1 = sp.RecentlyPlayed(ctx, 50) }()
		go func() { defer wg.Done(); msg.topTracks, e2 = sp.TopTracks(ctx, spotify.ShortTerm, 5) }()
		go func() { defer wg.Done(); msg.topArtists, e3 = sp.TopArtists(ctx, spotify.ShortTerm, 5) }()
		wg.Wait()
		for _, e := range []error{e1, e2, e3} {
			if e != nil {
				msg.err = e
				break
			}
		}
		return msg
	}
}

// maybeLoadHome, ana sayfaya girilince eskimişse tazeler.
func (m *Model) maybeLoadHome() tea.Cmd {
	if m.home.loading || (!m.home.fetched.IsZero() && m.now.Sub(m.home.fetched) < homeRefresh) {
		return nil
	}
	cmd := m.loadHome()
	if cmd != nil {
		m.home.loading = true
	}
	return cmd
}

func (m Model) updateHome(msg homeMsg) (Model, tea.Cmd) {
	h := &m.home
	h.loading, h.fetched = false, m.now
	if msg.err != nil {
		h.err = msg.err
		if len(msg.recent) == 0 && len(msg.topTracks) == 0 {
			return m.spotifyFailed(msg.err), nil
		}
	} else {
		h.err = nil
	}
	h.recent, h.topTracks, h.topArtists = msg.recent, msg.topTracks, msg.topArtists
	for _, r := range msg.recent {
		m.rt.known[r.Track.URI] = r.Track
	}
	m.rebuildHome()
	return m, nil
}

// rebuildHome, satırları yeniden kurar (kitaplık sonradan gelince liste adları da bulunur).
func (m *Model) rebuildHome() {
	h := &m.home
	keep := ""
	if h.cursor < len(h.items) {
		keep = h.items[h.cursor].uri
	}
	h.items = m.homeItems()
	h.cursor = nextSelectable(h.items, 0, 1)
	for i, it := range h.items {
		if keep != "" && it.uri == keep {
			h.cursor = i
		}
	}
	h.cursor, h.scroll = fitScroll(h.cursor, h.scroll, len(h.items), m.listHeight())
}

func (m Model) homeItems() []searchItem {
	h := m.home
	var out []searchItem
	section := func(key string, items []searchItem) {
		if len(items) > 0 {
			if len(out) > 0 {
				out = append(out, searchItem{kind: "space"})
			}
			out = append(out, searchItem{kind: "header", section: key})
			out = append(out, items...)
		}
	}

	// Kaldığın yerden devam: son çalınanların bağlamları (liste, albüm, sanatçı, beğenilenler).
	var jump []searchItem
	seen := map[string]bool{}
	for _, r := range h.recent {
		if r.Context == nil || r.Context.URI == "" || seen[r.Context.URI] || len(jump) >= 6 {
			continue
		}
		if it, ok := m.contextItem(r); ok {
			seen[r.Context.URI] = true
			jump = append(jump, it)
		}
	}
	section("home.jump", jump)

	// Son çalınanlar: aynı şarkı art arda tekrarlanmasın.
	var recent []searchItem
	last := ""
	for _, r := range h.recent {
		t := r.Track
		if t.URI == last || len(recent) >= 8 {
			continue
		}
		last = t.URI
		ctx := ""
		if r.Context != nil && r.Context.Type != "collection" {
			ctx = r.Context.URI
		}
		tt := t
		recent = append(recent, searchItem{kind: "track", name: t.Name, sub: t.ArtistNames() + "  " + ago(m.now, r.PlayedAt), uri: t.URI, track: &tt, ctx: ctx, history: true})
	}
	section("home.recent", recent)

	var tops []searchItem
	for i := range h.topTracks {
		t := h.topTracks[i]
		tops = append(tops, searchItem{kind: "track", name: t.Name, sub: t.ArtistNames(), uri: t.URI, track: &h.topTracks[i]})
	}
	section("home.toptracks", tops)

	var artists []searchItem
	for _, a := range h.topArtists {
		artists = append(artists, searchItem{kind: "artist", name: a.Name, uri: a.URI})
	}
	section("home.topartists", artists)
	return out
}

// contextItem, son çalınan bir şarkının bağlamını açılabilir bir satıra çevirir.
func (m Model) contextItem(r spotify.PlayHistory) (searchItem, bool) {
	uri := r.Context.URI
	switch r.Context.Type {
	case "collection":
		e := libEntry{kind: kindLiked, key: likedKey, name: T("lib.liked")}
		for _, le := range m.lib.entries {
			if le.kind == kindLiked {
				e = le
			}
		}
		return searchItem{kind: "liked", name: "♥ " + e.name, sub: T("kind.collection"), uri: uri, entry: e}, true
	case "playlist":
		for _, le := range m.lib.entries {
			if le.uri == uri {
				return searchItem{kind: "playlist", name: le.name, sub: T("kind.playlist"), uri: uri, entry: le}, true
			}
		}
	case "album":
		a := r.Track.Album
		if a.URI != uri || a.Name == "" {
			return searchItem{}, false
		}
		e := libEntry{kind: kindAlbum, key: a.ID, uri: a.URI, name: a.Name, owner: artistNames(a.Artists),
			image: pickImage(a.Images), album: a}
		sub := T("kind.album")
		if e.owner != "" {
			sub += " · " + e.owner
		}
		return searchItem{kind: "album", name: a.Name, sub: sub, uri: uri, entry: e}, true
	case "artist":
		for _, a := range r.Track.Artists {
			if a.URI == uri {
				return searchItem{kind: "artist", name: a.Name, sub: T("kind.artist"), uri: uri}, true
			}
		}
	}
	return searchItem{}, false
}

// ago, "3 dk önce" gibi göreli zaman.
func ago(now, t time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return T("ago.now")
	case d < time.Hour:
		return F("ago.min", int(d.Minutes()))
	case d < 24*time.Hour:
		return F("ago.hour", int(d.Hours()))
	case d < 48*time.Hour:
		return T("ago.yesterday")
	}
	return F("ago.days", int(d.Hours()/24))
}

func (m Model) homeKey(k string) (Model, tea.Cmd, bool) {
	if m.spState != spReady {
		return m.loginKeys(k)
	}
	h := &m.home
	if h.open != nil {
		cmd, handled, back := m.listKey(h.open, k)
		if back {
			h.open = nil
		}
		return m, cmd, handled
	}
	lh := m.listHeight()
	move := func(d int) {
		h.cursor = nextSelectable(h.items, h.cursor+d, d)
		h.cursor, h.scroll = fitScroll(h.cursor, h.scroll, len(h.items), lh)
		if h.cursor == nextSelectable(h.items, 0, 1) {
			h.scroll = 0 // ilk bölüm başlığı da görünsün
		}
	}
	switch k {
	case "up", "k":
		move(-1)
	case "down", "j":
		move(1)
	case "enter", "right":
		if len(h.items) == 0 {
			return m, nil, true
		}
		tl, cmd := m.actOn(h.items[h.cursor])
		if tl != nil {
			h.open = tl
		}
		return m, cmd, true
	case "R":
		m.refreshNow()
		h.fetched = time.Time{}
		cmd := m.maybeLoadHome()
		return m, cmd, true
	case "esc", "left", "backspace":
		m.focus = focusMenu
	default:
		return m, nil, false
	}
	return m, nil, true
}

func (m Model) homeView(w int) []string {
	title := theme.Title.Render(T("page.home"))
	if rows := m.accountView(title); rows != nil {
		return rows
	}
	h := m.home
	active := m.focus == focusContent
	if h.open != nil {
		return m.listView(h.open, w, active)
	}
	name := m.me.DisplayName
	sub := theme.Dim.Render(greeting(m.now))
	if name != "" {
		sub = theme.Dim.Render(F("home.greet", greeting(m.now), name))
	}
	rows := []string{title, sub, "", ""}
	switch {
	case h.err != nil && len(h.items) == 0:
		return append(rows, theme.Error.Render(h.err.Error()))
	case len(h.items) == 0 && (h.loading || h.fetched.IsZero()):
		return append(rows, theme.Dim.Render(T("lib.loading")))
	case len(h.items) == 0:
		return append(rows, theme.Dim.Render(T("home.empty")))
	}
	return append(rows, m.itemRows(h.items, h.cursor, h.scroll, w, active)...)
}

func greeting(t time.Time) string {
	switch hr := t.Hour(); {
	case hr >= 5 && hr < 12:
		return T("greet.morning")
	case hr >= 12 && hr < 18:
		return T("greet.afternoon")
	case hr >= 18 && hr < 23:
		return T("greet.evening")
	}
	return T("greet.night")
}
