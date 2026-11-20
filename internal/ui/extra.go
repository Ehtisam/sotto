package ui

import (
	"fmt"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/spotify"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// Ek sayfalar (menüde yok, tuşla ya da Ana sayfadan açılır): en çok
// dinlediklerin (g t), son çalınanlar (g r), günlük (g o). Bulunulan
// sayfanın içeriğinin yerine açılır; esc geri döner, başka bir sayfaya
// geçince kapanır. Çalan bağlam c ile açılır (quick.go).

type extraPage int

const (
	extraNone extraPage = iota
	extraTop
	extraRecent
	extraLog
)

type extraState struct {
	page extraPage
	host page // açıldığı sayfa (başka sayfaya geçilince kapanır)

	items  []searchItem
	cursor int
	scroll int
	open   *trackList // buradan açılan albüm ya da liste

	rng     int // en çok dinlenenler: zaman aralığı (profileRanges)
	loading bool
	err     error
	fetched time.Time

	tracks  []spotify.Track
	artists []spotify.FullArtist
	recent  []spotify.PlayHistory
}

type extraTopMsg struct {
	rng     int
	tracks  []spotify.Track
	artists []spotify.FullArtist
	err     error
}

type extraRecentMsg struct {
	items []spotify.PlayHistory
	err   error
}

// extraOn: bir ek sayfa açık ve bulunulan sayfanın üstünde.
func (m Model) extraOn() bool { return m.extra.page != extraNone && m.extra.host == m.page }

func (m Model) openExtra(p extraPage) (tea.Model, tea.Cmd) {
	if p != extraLog && m.spState != spReady {
		return m.say(T("flash.noaccount")), nil
	}
	rng := m.extra.rng
	m.extra = extraState{page: p, host: m.page, rng: rng}
	m.focus = focusContent
	switch p {
	case extraTop:
		m.extra.loading = true
		return m, m.loadExtraTop(rng)
	case extraRecent:
		m.extra.loading = true
		return m, m.loadExtraRecent()
	case extraLog:
		m.rebuildExtra()
	}
	return m, nil
}

func (m Model) loadExtraTop(rng int) tea.Cmd {
	sp, rt := m.sp, m.rt
	return func() tea.Msg {
		ctx, cancel := apiCtx(rt, 20*time.Second)
		defer cancel()
		msg := extraTopMsg{rng: rng}
		var wg sync.WaitGroup
		var e1, e2 error
		wg.Add(2)
		go func() { defer wg.Done(); msg.tracks, e1 = sp.TopTracks(ctx, profileRanges[rng], 50) }()
		go func() { defer wg.Done(); msg.artists, e2 = sp.TopArtists(ctx, profileRanges[rng], 50) }()
		wg.Wait()
		msg.err = e1
		if msg.err == nil {
			msg.err = e2
		}
		return msg
	}
}

func (m Model) loadExtraRecent() tea.Cmd {
	sp, rt := m.sp, m.rt
	return func() tea.Msg {
		ctx, cancel := apiCtx(rt, 20*time.Second)
		defer cancel()
		items, err := sp.RecentlyPlayed(ctx, 50)
		return extraRecentMsg{items, err}
	}
}

func (m Model) updateExtra(msg tea.Msg) (Model, tea.Cmd) {
	x := &m.extra
	switch msg := msg.(type) {
	case extraTopMsg:
		if x.page != extraTop || msg.rng != x.rng {
			return m, nil
		}
		x.loading, x.err, x.fetched = false, msg.err, m.now
		x.tracks, x.artists = msg.tracks, msg.artists
		for _, t := range msg.tracks {
			m.rt.known[t.URI] = t
		}
	case extraRecentMsg:
		if x.page != extraRecent {
			return m, nil
		}
		x.loading, x.err, x.fetched = false, msg.err, m.now
		x.recent = msg.items
	}
	if x.err != nil && len(x.tracks) == 0 && len(x.recent) == 0 {
		m = m.spotifyFailed(x.err)
	}
	m.rebuildExtra()
	return m, nil
}

// rebuildExtra, satırları yeniden kurar (imleç aynı öğede kalır).
func (m *Model) rebuildExtra() {
	x := &m.extra
	keep := ""
	if x.cursor < len(x.items) {
		keep = x.items[x.cursor].uri
	}
	switch x.page {
	case extraTop:
		x.items = m.topItems()
	case extraRecent:
		x.items = m.recentPlayedItems()
	case extraLog:
		// Yeni kayıtlar üste eklenir: imleç aynı kayıtta kalsın.
		old := len(x.items)
		x.items = m.logItems()
		if old > 0 && x.cursor > 0 {
			x.cursor += len(x.items) - old
			x.cursor, x.scroll = fitScroll(x.cursor, x.scroll+len(x.items)-old, len(x.items), m.extraHeight())
			return
		}
		x.cursor, x.scroll = 0, 0
		return
	}
	x.cursor = nextSelectable(x.items, 0, 1)
	for i, it := range x.items {
		if keep != "" && it.uri == keep && it.selectable() {
			x.cursor = i
			break
		}
	}
	x.cursor, x.scroll = fitScroll(x.cursor, x.scroll, len(x.items), m.extraHeight())
	if x.cursor == nextSelectable(x.items, 0, 1) {
		x.scroll = 0
	}
}

func (m Model) topItems() []searchItem {
	x := m.extra
	var out []searchItem
	if len(x.tracks) > 0 {
		out = append(out, searchItem{kind: "header", section: "extra.top.tracks"})
		for i := range x.tracks {
			t := x.tracks[i]
			out = append(out, searchItem{kind: "track", name: fmt.Sprintf("%2d  %s", i+1, t.Name), sub: t.ArtistNames(), uri: t.URI, track: &x.tracks[i]})
		}
	}
	if len(x.artists) > 0 {
		if len(out) > 0 {
			out = append(out, searchItem{kind: "space"})
		}
		out = append(out, searchItem{kind: "header", section: "extra.top.artists"})
		for i, a := range x.artists {
			out = append(out, searchItem{kind: "artist", name: fmt.Sprintf("%2d  %s", i+1, a.Name), sub: T("kind.artist"), uri: a.URI})
		}
	}
	return out
}

// recentPlayedItems, son çalınanlar günlere bölünmüş olarak.
func (m Model) recentPlayedItems() []searchItem {
	var out []searchItem
	day := ""
	for _, r := range m.extra.recent {
		d := dayName(m.now, r.PlayedAt)
		if d != day {
			if len(out) > 0 {
				out = append(out, searchItem{kind: "space"})
			}
			out = append(out, searchItem{kind: "header", name: d})
			day = d
		}
		t := r.Track
		ctx := ""
		if r.Context != nil && r.Context.Type != "collection" {
			ctx = r.Context.URI
		}
		tt := t
		sub := t.ArtistNames() + "  " + r.PlayedAt.Local().Format("15:04")
		out = append(out, searchItem{kind: "track", name: t.Name, sub: sub, uri: t.URI, track: &tt, ctx: ctx, history: true})
	}
	return out
}

// dayName: "Bugün", "Dün" ya da tarih.
func dayName(now, t time.Time) string {
	now, t = now.Local(), t.Local()
	switch t.Format("2006-01-02") {
	case now.Format("2006-01-02"):
		return T("extra.today")
	case now.AddDate(0, 0, -1).Format("2006-01-02"):
		return T("extra.yesterday")
	}
	return t.Format("02.01.2006")
}

func (m Model) logItems() []searchItem {
	var out []searchItem
	for _, e := range m.rt.logs.list() {
		kind := "log"
		if e.level == logError {
			kind = "logerr"
		}
		out = append(out, searchItem{kind: kind, name: e.text, sub: e.at.Local().Format("15:04:05")})
	}
	return out
}

// extraHeight, ek sayfada liste satırlarına kalan yükseklik.
func (m Model) extraHeight() int { return m.listHeight() }

func (m Model) extraKey(k string) (Model, tea.Cmd, bool) {
	x := &m.extra
	if x.open != nil {
		cmd, handled, back := m.listKey(x.open, k)
		if back {
			x.open = nil
		}
		return m, cmd, handled
	}
	lh := m.extraHeight()
	move := func(d int) {
		x.cursor = nextSelectable(x.items, x.cursor+d, d)
		x.cursor, x.scroll = fitScroll(x.cursor, x.scroll, len(x.items), lh)
		if x.cursor == nextSelectable(x.items, 0, 1) {
			x.scroll = 0
		}
	}
	switch k {
	case "up", "k":
		move(-1)
	case "down", "j":
		move(1)
	case "pgup":
		move(-lh)
	case "pgdown":
		move(lh)
	case "home", "g":
		move(-1 << 30)
	case "end", "G":
		move(1 << 30)
	case "[", "]":
		if x.page != extraTop {
			return m, nil, false
		}
		d := 1
		if k == "[" {
			d = -1
		}
		x.rng = cycle(x.rng, d, len(profileRanges))
		x.loading, x.tracks, x.artists, x.items = true, nil, nil, nil
		return m, m.loadExtraTop(x.rng), true
	case "R":
		if x.page == extraLog {
			m.rebuildExtra()
			return m, nil, true
		}
		m.refreshNow()
		mm, cmd := m.openExtra(x.page)
		return mm.(Model), cmd, true
	case "enter", "right":
		if len(x.items) == 0 || !x.items[x.cursor].selectable() {
			return m, nil, true
		}
		it := x.items[x.cursor]
		if x.page == extraLog {
			return m.note(T("act.copied")), tea.SetClipboard(it.sub + "  " + it.name), true
		}
		tl, cmd := m.actOn(it)
		if tl != nil {
			x.open = tl
		}
		return m, cmd, true
	case "e":
		if x.page != extraLog {
			return m, nil, false
		}
		if err := openBrowser(logFile(m.cfg.LocalDir)); err != nil {
			return m.say(err.Error()), nil, true
		}
		return m, nil, true
	case "esc", "left", "backspace":
		m.extra = extraState{rng: x.rng}
		if !m.page.hasContent() {
			m.focus = focusMenu
		}
	default:
		return m, nil, false
	}
	return m, nil, true
}

func (m Model) extraView(w int) []string {
	x := m.extra
	active := m.focus == focusContent
	if x.open != nil {
		return m.listView(x.open, w, active)
	}
	var title, sub string
	switch x.page {
	case extraTop:
		title = T("extra.top")
		sub = T("prof.range."+profileRanges[x.rng]) + "  ·  " + T("extra.top.sub")
	case extraRecent:
		title, sub = T("extra.recent"), T("extra.recent.sub")
	case extraLog:
		title, sub = T("extra.log"), F("extra.log.sub", logFile(m.cfg.LocalDir))
	}
	rows := []string{theme.Title.Render(title), theme.Dim.Render(truncate(sub, w)), "", ""}
	switch {
	case x.loading && len(x.items) == 0:
		return append(rows, theme.Dim.Render(T("lib.loading")))
	case x.err != nil && len(x.items) == 0:
		return append(rows, theme.Error.Render(x.err.Error()))
	case len(x.items) == 0:
		return append(rows, theme.Dim.Render(T("extra.empty")))
	}
	if x.page == extraLog {
		end := min(len(x.items), x.scroll+m.extraHeight())
		for i := x.scroll; i < end; i++ {
			it := x.items[i]
			text := theme.Plain
			if it.kind == "logerr" {
				text = theme.Error
			}
			line := theme.Faint.Render(it.sub+"  ") + text.Render(truncate(it.name, w-10))
			if active && i == x.cursor {
				line = theme.SelectedRow.Width(w).Render(truncate(it.sub+"  "+it.name, w))
			}
			rows = append(rows, line)
		}
		return rows
	}
	return append(rows, m.itemRowsH(x.items, x.cursor, x.scroll, w, m.extraHeight(), active)...)
}
