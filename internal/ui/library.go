package ui

import (
	"context"
	"errors"
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Ehtisam/sotto/internal/spotify"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// IV · Kitaplık: listeler ve beğenilenler; birine girilince şarkıları.

const likedKey = "liked"

type libEntry struct {
	key   string // likedKey ya da liste ID'si
	uri   string
	name  string
	owner string
	desc  string
	image string // liste kapağı
	total int
}

var tagRE = regexp.MustCompile(`<[^>]*>`)

// cleanDesc, Spotify'ın HTML kaçışlı liste açıklamasını düz metne çevirir.
func cleanDesc(s string) string {
	return strings.Join(strings.Fields(html.UnescapeString(tagRE.ReplaceAllString(s, ""))), " ")
}

type library struct {
	loading bool
	loaded  bool
	err     error
	entries []libEntry
	cursor  int
	scroll  int

	open          *libEntry // nil: liste düzeyi
	tracks        []spotify.Track
	total         int
	tracksLoading bool
	tracksErr     error
	tcursor       int
	tscroll       int
}

const pageSize = 50

type playlistsMsg struct {
	entries []libEntry
	err     error
}

type tracksPageMsg struct {
	key    string
	offset int
	tracks []spotify.Track
	total  int
	err    error
}

type playedMsg struct {
	err       error
	elsewhere string // Soloist kapalıyken şarkının gittiği cihaz
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
		entries := []libEntry{{key: likedKey, name: T("lib.liked"), total: liked.Total}}
		for offset := 0; offset < 2000; offset += pageSize {
			p, err := sp.MyPlaylists(ctx, offset, pageSize)
			if err != nil {
				return playlistsMsg{entries: entries, err: err}
			}
			for _, pl := range p.Items {
				if pl.ID == "" {
					continue
				}
				entries = append(entries, libEntry{
					key: pl.ID, uri: pl.URI, name: pl.Name,
					owner: pl.Owner.DisplayName, desc: cleanDesc(pl.Description), image: pickImage(pl.Images), total: pl.Total(),
				})
			}
			if p.Next == "" || len(p.Items) == 0 {
				break
			}
		}
		return playlistsMsg{entries: entries}
	}
}

func (m Model) loadTracks(e libEntry, offset int) tea.Cmd {
	sp := m.sp
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		msg := tracksPageMsg{key: e.key, offset: offset}
		if e.key == likedKey {
			p, err := sp.LikedTracks(ctx, offset, pageSize)
			if err != nil {
				msg.err = err
				return msg
			}
			msg.total = p.Total
			for _, it := range p.Items {
				msg.tracks = append(msg.tracks, it.Track)
			}
			return msg
		}
		p, err := sp.PlaylistItems(ctx, e.key, offset, pageSize)
		if err != nil {
			msg.err = err
			return msg
		}
		msg.total = p.Total
		for _, it := range p.Items {
			if t := it.T(); t != nil && t.URI != "" {
				msg.tracks = append(msg.tracks, *t)
			}
		}
		return msg
	}
}

// play, seçili şarkıyı listesiyle birlikte çalar. Soloist açıksa ona,
// değilse o an etkin olan cihaza gönderir.
func (m Model) play(e libEntry, tracks []spotify.Track, i int) tea.Cmd {
	sp := m.sp
	device := ""
	if m.soloistLive() {
		device = m.auth.DeviceName
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		id, elsewhere := "", ""
		if ds, err := sp.Devices(ctx); err == nil {
			for _, d := range ds {
				if device != "" && d.Name == device {
					id = d.ID
				}
				if device == "" && d.IsActive {
					elsewhere = d.Name
				}
			}
		}
		start := tracks[i].URI
		if e.key == likedKey {
			end := min(len(tracks), i+100)
			uris := make([]string, 0, end-i)
			for _, t := range tracks[i:end] {
				uris = append(uris, t.URI)
			}
			return playedMsg{err: sp.PlayTracks(ctx, id, uris, start), elsewhere: elsewhere}
		}
		return playedMsg{err: sp.PlayContext(ctx, id, e.uri, start), elsewhere: elsewhere}
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
		if m.lib.open == nil || m.lib.open.key != msg.key {
			return m, nil // kullanıcı bu arada başka listeye geçti
		}
		m.lib.tracksLoading = false
		if msg.err != nil {
			m.lib.tracksErr = msg.err
			return m.spotifyFailed(msg.err), nil
		}
		if msg.offset == len(m.lib.tracks) {
			m.lib.tracks = append(m.lib.tracks, msg.tracks...)
		}
		m.lib.total = msg.total
		for _, t := range msg.tracks {
			m.rt.known[t.URI] = t
		}
		m.lib.clamp(m.listHeight())
	case playedMsg:
		if msg.err != nil {
			text := msg.err.Error()
			if spotify.IsStatus(msg.err, 404) {
				text = T("flash.nodevice")
			}
			return m.say(text), nil
		}
		if msg.elsewhere != "" {
			m = m.note(F("flash.elsewhere", msg.elsewhere))
		}
		return m, func() tea.Msg { return pollSoonMsg{} }
	}
	return m, nil
}

func (m Model) libraryKey(k string) (Model, tea.Cmd, bool) {
	switch m.spState {
	case spNoClient:
		if k == "esc" || k == "left" || k == "backspace" {
			m.focus = focusMenu
			return m, nil, true
		}
		return m, nil, false
	case spLoggedOut, spLoggingIn:
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

	l := &m.lib
	h := m.listHeight()
	move := func(d int) {
		if l.open == nil {
			l.cursor = max(0, min(len(l.entries)-1, l.cursor+d))
		} else {
			l.tcursor = max(0, min(len(l.tracks)-1, l.tcursor+d))
		}
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
		if l.open == nil {
			if len(l.entries) == 0 {
				if !l.loading {
					l.loading = true
					return m, m.loadLibrary(), true
				}
				return m, nil, true
			}
			e := l.entries[l.cursor]
			l.open = &e
			l.tracks, l.total, l.tcursor, l.tscroll, l.tracksErr = nil, e.total, 0, 0, nil
			l.tracksLoading = true
			return m, m.loadTracks(e, 0), true
		}
		if len(l.tracks) == 0 {
			return m, nil, true
		}
		return m, m.play(*l.open, l.tracks, l.tcursor), true
	case "esc", "left", "backspace":
		if l.open != nil {
			l.open = nil
			return m, nil, true
		}
		m.focus = focusMenu
		return m, nil, true
	default:
		return m, nil, false
	}

	// Listenin sonuna yaklaşınca sonraki sayfayı getir.
	if l.open != nil && !l.tracksLoading && len(l.tracks) < l.total && l.tcursor >= len(l.tracks)-10 {
		l.tracksLoading = true
		return m, m.loadTracks(*l.open, len(l.tracks)), true
	}
	return m, nil, true
}

// clamp, imleci görünür tutacak şekilde kaydırmayı ayarlar.
func (l *library) clamp(h int) {
	fit := func(cursor, scroll, n int) (int, int) {
		cursor = max(0, min(n-1, cursor))
		if cursor < scroll {
			scroll = cursor
		}
		if cursor >= scroll+h {
			scroll = cursor - h + 1
		}
		scroll = max(0, min(scroll, max(n-h, 0)))
		return max(cursor, 0), scroll
	}
	l.cursor, l.scroll = fit(l.cursor, l.scroll, len(l.entries))
	l.tcursor, l.tscroll = fit(l.tcursor, l.tscroll, len(l.tracks))
}

// ---- çizim ----

func (m Model) libraryView(w, h int) []string {
	title := theme.Title.Render(T("page.library"))
	switch m.spState {
	case spNoClient:
		return []string{title, "",
			theme.Plain.Render(T("lib.noclient")),
			theme.Faint.Render(T("lib.noclient.how")) + theme.Dim.Render(m.cfg.ConfigFile()),
			theme.Faint.Render(`[spotify]  client_id = "..."`),
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

	l := m.lib
	active := m.focus == focusContent
	playingCtx := ""
	if m.state.Context != nil {
		playingCtx = m.state.Context.URI
	}

	if l.open == nil {
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
			if e.key == likedKey {
				name = "♥ " + name
			}
			right := F("lib.songs", e.total)
			rows = append(rows, mark+row(name, right, w-2, active && i == l.cursor))
		}
		return rows
	}

	e := l.open
	sub := e.owner
	if e.key == likedKey {
		sub = T("lib.liked.sub")
	}
	info := theme.Dim.Render(sub) + theme.Faint.Render(F("lib.songs.short", l.total))
	if l.tracksErr != nil {
		info = theme.Error.Render(l.tracksErr.Error())
	}
	// Başlık: solda küçük liste kapağı, yanında ad, sahibi ve açıklama.
	const cw, ch = 6, 3 // kare: 6 sütun x 6 piksel
	textW := w
	if e.image != "" {
		textW = w - cw - 2
	}
	desc := ""
	if e.desc != "" {
		desc = theme.Dim.Italic(true).Render(truncate(e.desc, textW))
	}
	head := []string{theme.Title.Render(truncate(e.name, textW)), info, desc, ""}
	if e.image != "" {
		block := lipgloss.JoinHorizontal(lipgloss.Top, m.coverBox(e.image, cw, ch), "  ", strings.Join(head, "\n"))
		head = append(strings.Split(block, "\n")[:ch], "")
	}
	rows := head
	if len(l.tracks) == 0 && l.tracksLoading {
		return append(rows, theme.Dim.Render(T("lib.loading")))
	}
	playing := ""
	if m.state.Item != nil {
		playing = m.state.Item.URI
	}
	end := min(len(l.tracks), l.tscroll+m.listHeight())
	for i := l.tscroll; i < end; i++ {
		t := l.tracks[i]
		num := theme.Faint.Render(fmt.Sprintf("%3d  ", i+1))
		if t.URI == playing {
			num = theme.GoldText.Render("  ♪  ")
		}
		right := t.ArtistNames() + "  " + clock(time.Duration(t.DurationMS)*time.Millisecond)
		rows = append(rows, num+row(t.Name, right, w-5, active && i == l.tcursor))
	}
	if l.tracksLoading && len(l.tracks) > 0 && end == len(l.tracks) {
		rows = append(rows, theme.Faint.Render("     "+T("lib.loading")))
	}
	return rows
}

// row, noktalı bir liste satırı; seçiliyse vurgu zemininde.
func row(left, right string, w int, selected bool) string {
	l, dots, r := leaderParts(left, right, w)
	if selected {
		text := l
		if r != "" {
			text += " " + dots + " " + r
		}
		return theme.SelectedRow.Width(w).Render(text)
	}
	if r == "" {
		return theme.Plain.Render(l)
	}
	return theme.Plain.Render(l) + " " + theme.Faint.Render(dots) + " " + theme.Dim.Render(r)
}

// leaderParts, "sol ···· sağ" satırının parçalarını w genişliğine sığdırır.
func leaderParts(left, right string, w int) (l, dots, r string) {
	left = strings.TrimSpace(left)
	if right == "" {
		return truncate(left, w), "", ""
	}
	rw := textWidth(right)
	if rw > w/2 {
		right = truncate(right, w/2)
		rw = textWidth(right)
	}
	l = truncate(left, max(w-rw-4, 1))
	n := w - textWidth(l) - rw - 2
	return l, strings.Repeat("·", max(n, 1)), right
}

// likedName, beğenilenlerin geçerli dildeki adı.
func (m Model) likedName() string { return T("lib.liked") }
