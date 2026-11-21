package ui

import (
	"context"
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

// trackList, bir listenin, albümün ya da beğenilenlerin şarkıları. Kitaplık
// ve Ara sayfası aynı görünümü kullanır.

const (
	kindLiked    = "liked"
	kindPlaylist = "playlist"
	kindAlbum    = "album"
	kindShow     = "show"
)

const likedKey = "liked"

// libEntry, açılabilen bir şarkı topluluğu.
type libEntry struct {
	kind  string // kindLiked, kindPlaylist, kindAlbum
	key   string // likedKey, liste ya da albüm ID'si
	uri   string
	name  string
	owner string // liste sahibi ya da albümün sanatçısı
	desc  string
	image string
	total int
	album spotify.Album // yalnızca albümler için

	ownerID       string // listeler: sahibinin kullanıcı kimliği
	collaborative bool
	public        bool // listeler: profilde herkese açık
	pinned        bool // Spotify'da sabitlenmiş (Kitaplık'ta üstte; pins.go)
}

var tagRE = regexp.MustCompile(`<[^>]*>`)

// cleanDesc, Spotify'ın HTML kaçışlı açıklamasını düz metne çevirir.
func cleanDesc(s string) string {
	return strings.Join(strings.Fields(html.UnescapeString(tagRE.ReplaceAllString(s, ""))), " ")
}

type trackList struct {
	entry   libEntry
	tracks  []spotify.Track
	total   int
	loading bool
	err     error
	cursor  int
	scroll  int
	moving  bool   // şarkı taşıma isteği yolda
	seek    string // c: bulunana kadar sayfa yüklenen şarkı

	find   finder // sayfa içi arama (find.go)
	sortBy string // sıralama (sort.go); "" listenin kendi sırası
	desc   bool

	ver      int // şarkılar değişince artar (görünüm önbelleği için)
	cache    []int
	cacheKey viewKey
}

const pageSize = 50

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

// openList, bir topluluğu açar ve ilk sayfasını ister.
func (m Model) openList(e libEntry) (*trackList, tea.Cmd) {
	tl := &trackList{entry: e, total: e.total, loading: true}
	if s, ok := m.rt.sorts[e.key]; ok && e.key != "" {
		tl.sortBy, tl.desc = s.By, s.Desc
	}
	return tl, m.loadTracks(e, 0)
}

func (m Model) loadTracks(e libEntry, offset int) tea.Cmd {
	sp := m.sp
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		msg := tracksPageMsg{key: e.key, offset: offset}
		switch e.kind {
		case kindLiked:
			p, err := sp.LikedTracks(ctx, offset, pageSize)
			if err != nil {
				msg.err = err
				return msg
			}
			msg.total = p.Total
			for _, it := range p.Items {
				t := it.Track
				t.AddedAt = it.AddedAt
				msg.tracks = append(msg.tracks, t)
			}
		case kindShow:
			p, err := sp.ShowEpisodes(ctx, e.key, offset, pageSize)
			if err != nil {
				msg.err = err
				return msg
			}
			msg.total = p.Total
			for _, ep := range p.Items {
				if ep != nil && ep.URI != "" {
					msg.tracks = append(msg.tracks, *ep)
				}
			}
		case kindAlbum:
			p, err := sp.AlbumTracks(ctx, e.album, offset, pageSize)
			if err != nil {
				msg.err = err
				return msg
			}
			msg.total, msg.tracks = p.Total, p.Items
		default:
			p, err := sp.PlaylistItems(ctx, e.key, offset, pageSize)
			if err != nil {
				msg.err = err
				return msg
			}
			msg.total = p.Total
			for _, it := range p.Items {
				if t := it.T(); t != nil && t.URI != "" {
					tt := *t
					tt.AddedAt = it.AddedAt
					msg.tracks = append(msg.tracks, tt)
				}
			}
		}
		return msg
	}
}

// receive, gelen sayfayı listeye ekler. Liste başkasıysa false döner.
func (tl *trackList) receive(msg tracksPageMsg) bool {
	if tl == nil || tl.entry.key != msg.key {
		return false
	}
	tl.loading = false
	if msg.err != nil {
		tl.err = msg.err
		return true
	}
	if msg.offset == len(tl.tracks) {
		tl.tracks = append(tl.tracks, msg.tracks...)
	}
	tl.total = msg.total
	return true
}

// remove, şarkıyı listeden çıkarır (listeden silinince ekran hemen güncellensin).
func (tl *trackList) remove(uri string) {
	out := tl.tracks[:0]
	for _, t := range tl.tracks {
		if t.URI != uri {
			out = append(out, t)
		}
	}
	if removed := len(tl.tracks) - len(out); removed > 0 {
		tl.total -= removed
	}
	tl.tracks = out
	tl.changed()
	tl.cursor = max(0, min(tl.cursor, len(tl.tracks)-1))
}

// clamp, imleci görünen satırlar içinde ve ekranda tutar.
func (tl *trackList) clamp(h int) {
	n := tl.rows()
	if n == 0 {
		tl.scroll = 0
		return
	}
	p := max(tl.pos(), 0)
	p, tl.scroll = fitScroll(p, tl.scroll, n, h)
	tl.cursor = tl.at(p)
}

// fitScroll, imleci görünür tutacak şekilde kaydırmayı ayarlar.
func fitScroll(cursor, scroll, n, h int) (int, int) {
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

// listKey, açık bir listedeyken tuşlar. back=true: kullanıcı listeden çıktı.
func (m Model) listKey(tl *trackList, k string) (cmd tea.Cmd, handled, back bool) {
	h := m.listHeight()
	move := func(d int) {
		n := tl.rows()
		if n == 0 {
			return
		}
		tl.cursor = tl.at(max(0, min(n-1, max(tl.pos(), 0)+d)))
		tl.clamp(h)
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
		if tl.rows() == 0 {
			return nil, true, false
		}
		return m.playFrom(tl.entry, tl.tracks, tl.cursor), true, false
	// Taşıma: shift+J aşağı, shift+K yukarı.
	case "K", "J":
		d := 1
		if k == "K" {
			d = -1
		}
		if !tl.natural() {
			return func() tea.Msg { return flashMsg(T("find.nomove")) }, true, false
		}
		return m.moveInList(tl, d), true, false
	case "x", "delete":
		return m.removeFromList(tl), true, false
	case "esc", "left", "backspace":
		return nil, true, true
	default:
		return nil, false, false
	}
	// Sonuna yaklaşınca sonraki sayfayı getir.
	if !tl.loading && len(tl.tracks) < tl.total && max(tl.pos(), 0) >= tl.rows()-10 {
		tl.loading = true
		return m.loadTracks(tl.entry, len(tl.tracks)), true, false
	}
	return nil, true, false
}

// ---- çalma ----

// playFrom, seçili şarkıyı topluluğuyla birlikte çalar.
func (m Model) playFrom(e libEntry, tracks []spotify.Track, i int) tea.Cmd {
	start := tracks[i].URI
	if e.kind == kindLiked {
		end := min(len(tracks), i+100)
		uris := make([]string, 0, end-i)
		for _, t := range tracks[i:end] {
			uris = append(uris, t.URI)
		}
		return m.playOn(func(ctx context.Context, sp *spotify.Client, id string) error {
			return sp.PlayTracks(ctx, id, uris, start)
		})
	}
	return m.playContext(e.uri, start)
}

// playContext, bir listeyi, albümü, sanatçıyı ya da podcast'i (isteğe bağlı bir şarkıdan) çalar.
func (m Model) playContext(contextURI, startURI string) tea.Cmd {
	return m.playOn(func(ctx context.Context, sp *spotify.Client, id string) error {
		return sp.PlayContext(ctx, id, contextURI, startURI)
	})
}

// playOn, çalma isteğini Soloist açıksa ona, değilse o an etkin olan cihaza gönderir.
func (m Model) playOn(f func(ctx context.Context, sp *spotify.Client, deviceID string) error) tea.Cmd {
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
		return playedMsg{err: f(ctx, sp, id), elsewhere: elsewhere}
	}
}

func (m Model) played(msg playedMsg) (Model, tea.Cmd) {
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

// ---- çizim ----

// listView, açık listenin başlığı (küçük kapak, ad, sahibi, açıklama) ve şarkıları.
func (m Model) listView(tl *trackList, w int, active bool) []string {
	e := tl.entry
	var sub string
	switch e.kind {
	case kindLiked:
		sub = T("lib.liked.sub")
	case kindAlbum:
		sub = e.owner
		if e.desc != "" {
			sub += " · " + e.desc
		}
	case kindShow:
		sub = T("kind.show")
	default:
		sub = e.owner
	}
	count := F("lib.songs.short", tl.total)
	if e.kind == kindShow {
		count = " · " + F("search.episodecount", tl.total)
	}
	info := theme.Dim.Render(sub) + theme.Faint.Render(count) + theme.Faint.Render(sortLabel(tl.sortBy, tl.desc))
	if e.kind == kindPlaylist && m.editable(e) {
		info += theme.Faint.Render(T("pl.move") + T("pl.x"))
	}
	if tl.err != nil {
		info = theme.Error.Render(tl.err.Error())
	}
	const cw, ch = 6, 3 // kare: 6 sütun x 6 piksel
	img := e.image
	if !m.cfg.Look.CoversOn() {
		img = ""
	}
	textW := w
	if img != "" {
		textW = w - cw - 2
	}
	desc := ""
	if e.desc != "" && e.kind != kindAlbum {
		desc = theme.Dim.Italic(true).Render(truncate(e.desc, textW))
	}
	head := []string{theme.Title.Render(truncate(e.name, textW)), info, desc}
	if img != "" {
		block := lipgloss.JoinHorizontal(lipgloss.Top, m.coverBox(img, cw, ch), "  ", strings.Join(head, "\n"))
		head = strings.Split(block, "\n")[:ch]
	}
	// Başlığın altındaki boş satır: sayfa içi arama açıksa onun yazısı.
	head = append(head, findBar(tl.find, tl.rows(), len(tl.tracks), w))
	rows := head
	if len(tl.tracks) == 0 && tl.loading {
		return append(rows, theme.Dim.Render(T("lib.loading")))
	}
	playing := ""
	if m.state.Item != nil {
		playing = m.state.Item.URI
	}
	n := tl.rows()
	if n == 0 && tl.find.query != "" {
		rows = append(rows, theme.Dim.Render(T("find.none")))
	}
	end := min(n, tl.scroll+m.listHeight())
	cur := tl.pos()
	numW := 5 // "123  "; numaralar kapalıyken yalnızca ♪ için iki sütun
	if m.cfg.Look.LineNumbers == "off" {
		numW = 2
	}
	barW := scrollbarW(n, m.listHeight())
	listStart := len(rows)
	for p := tl.scroll; p < end; p++ {
		i := tl.at(p)
		t := tl.tracks[i]
		var num string
		switch {
		case t.URI == playing && numW == 2:
			num = theme.Playing.Render(theme.Icon.Note + " ")
		case t.URI == playing:
			num = theme.Playing.Render(fmt.Sprintf("%3s  ", theme.Icon.Note))
		case numW == 2:
			num = "  "
		case m.cfg.Look.LineNumbers == "relative" && active && p == cur:
			num = theme.Dim.Render(fmt.Sprintf("%-3s  ", m.lineNumber(i+1, p, cur, active)))
		default:
			num = theme.Faint.Render(fmt.Sprintf("%3s  ", m.lineNumber(i+1, p, cur, active)))
		}
		right := clock(time.Duration(t.DurationMS) * time.Millisecond)
		switch e.kind {
		case kindShow:
			right = episodeInfo(t)
		case kindAlbum:
		default:
			right = t.ArtistNames() + "  " + right
		}
		rows = append(rows, num+row(t.Name, right, w-numW-barW, active && i == tl.cursor))
	}
	rows = withScrollbar(rows, listStart, n, tl.scroll, m.listHeight(), w)
	if tl.loading && len(tl.tracks) > 0 && end == n {
		rows = append(rows, theme.Faint.Render("     "+T("lib.loading")))
	}
	return rows
}

// episodeInfo, bir podcast bölümünün tarihi ve süresi.
func episodeInfo(t spotify.Track) string {
	d := clock(time.Duration(t.DurationMS) * time.Millisecond)
	if t.ReleaseDate == "" {
		return d
	}
	return t.ReleaseDate + "  " + d
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
