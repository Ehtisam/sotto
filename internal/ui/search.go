package ui

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/spotify"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// III · Ara: yazdıkça arar. Süzgeçler: Hepsi, Şarkılar, Sanatçılar,
// Albümler, Listeler, Podcast'ler (tab ile geçilir).

const (
	filterAll = iota
	filterSongs
	filterArtists
	filterAlbums
	filterPlaylists
	filterShows
	filterEpisodes
	filterCount
)

var filterKeys = [filterCount]string{"search.all", "search.songs", "search.artists", "search.albums", "search.playlists", "search.shows", "search.episodes"}
var filterTypes = [filterCount][]string{
	{"track", "artist", "album", "playlist", "show", "episode"},
	{"track"}, {"artist"}, {"album"}, {"playlist"}, {"show"}, {"episode"},
}

// "Hepsi" görünümünde her bölümden kaç sonuç gösterilir.
var allCounts = map[string]int{"track": 5, "artist": 3, "album": 3, "playlist": 3, "show": 3, "episode": 3}

type searchItem struct {
	kind    string // track, artist, album, playlist, show; header: bölüm başlığı
	name    string
	sub     string
	uri     string
	track   *spotify.Track
	ctx     string   // şarkı bu bağlamda (liste, albüm) çalınsın; boşsa albümünde
	entry   libEntry // album, playlist ve show açılırken
	section string   // header satırları için
	skip    int      // sıra sayfası: bu şarkıya kaç atlamayla gelinir (-1: çalan şarkı)
	// history: bağlam Spotify'ın son çalınanlar kaydından geliyor; güvenilmez
	// (otomatik devamda çalınan şarkılar da listenin adıyla kaydediliyor).
	history bool
}

type searchState struct {
	query   string
	typing  bool // yazı kutusu odakta
	filter  int
	seq     int // yazarken bekleme sayacı
	last    string
	lastF   int
	loading bool
	err     error

	items  []searchItem
	total  int // tek türde: Spotify'ın toplam sonuç sayısı
	cursor int
	scroll int

	open *trackList

	recent []savedSearch // son aramalar (recent.go)
}

type searchFireMsg struct{ seq int }

type searchResultMsg struct {
	query  string
	filter int
	offset int
	res    spotify.SearchResult
	err    error
}

const searchDelay = 350 * time.Millisecond

func (m Model) scheduleSearch() (Model, tea.Cmd) {
	m.search.seq++
	seq := m.search.seq
	return m, tea.Tick(searchDelay, func(time.Time) tea.Msg { return searchFireMsg{seq} })
}

func (m Model) runSearch(offset int) (Model, tea.Cmd) {
	q := strings.TrimSpace(m.search.query)
	if q == "" {
		// Kutu boş: son aramalar.
		m.search.items, m.search.last, m.search.err = m.recentItems(), "", nil
		m.search.loading = false // yolda olan eski arama artık önemsiz
		m.search.total, m.search.cursor, m.search.scroll = 0, nextSelectable(m.search.items, 0, 1), 0
		return m, nil
	}
	if offset == 0 {
		m.search.last, m.search.lastF = q, m.search.filter
	}
	m.search.loading = true
	sp, f := m.sp, m.search.filter
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		res, err := sp.Search(ctx, q, filterTypes[f], offset)
		return searchResultMsg{q, f, offset, res, err}
	}
}

func (m Model) updateSearch(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case searchFireMsg:
		if msg.seq != m.search.seq {
			return m, nil // yazmaya devam edildi
		}
		q := strings.TrimSpace(m.search.query)
		if q == m.search.last && m.search.filter == m.search.lastF {
			return m, nil
		}
		return m.runSearch(0)

	case searchResultMsg:
		s := &m.search
		if msg.query != strings.TrimSpace(s.query) || msg.filter != s.filter {
			return m, nil // eski bir arama
		}
		s.loading = false
		if msg.err != nil {
			s.err = msg.err
			return m.spotifyFailed(msg.err), nil
		}
		s.err = nil
		items := searchItems(msg.res, msg.filter)
		if msg.offset == 0 {
			s.items, s.cursor, s.scroll = items, 0, 0
		} else {
			s.items = append(s.items, items...)
		}
		s.total = searchTotal(msg.res, msg.filter)
		s.cursor = nextSelectable(s.items, s.cursor, 1)
		for _, it := range s.items {
			if it.track != nil {
				m.rt.known[it.track.URI] = *it.track
			}
		}
	}
	return m, nil
}

// searchItems, sonuçları ekrandaki satırlara çevirir. "Hepsi"nde bölüm başlıkları eklenir.
func searchItems(r spotify.SearchResult, filter int) []searchItem {
	var out []searchItem
	add := func(kind, section string, items []searchItem) {
		if len(items) == 0 {
			return
		}
		if filter == filterAll {
			if len(out) > 0 {
				out = append(out, searchItem{kind: "space"})
			}
			out = append(out, searchItem{kind: "header", section: section})
			items = items[:min(len(items), allCounts[kind])]
		}
		out = append(out, items...)
	}

	var tracks, artists, albums, lists, shows, episodes []searchItem
	for _, t := range r.Tracks.Items {
		if t != nil {
			tracks = append(tracks, searchItem{kind: "track", name: t.Name, sub: t.ArtistNames() + "  " + clock(time.Duration(t.DurationMS)*time.Millisecond), uri: t.URI, track: t})
		}
	}
	for _, a := range r.Artists.Items {
		if a != nil {
			artists = append(artists, searchItem{kind: "artist", name: a.Name, uri: a.URI})
		}
	}
	for _, a := range r.Albums.Items {
		if a == nil {
			continue
		}
		artist := artistNames(a.Artists)
		year := a.ReleaseDate
		if len(year) > 4 {
			year = year[:4]
		}
		e := libEntry{kind: kindAlbum, key: a.ID, uri: a.URI, name: a.Name, owner: artist, desc: year,
			image: pickImage(a.Images), total: a.TotalTracks, album: a.Album}
		sub := artist
		if year != "" {
			sub += " · " + year
		}
		albums = append(albums, searchItem{kind: "album", name: a.Name, sub: sub, uri: a.URI, entry: e})
	}
	for _, p := range r.Playlists.Items {
		if p != nil && p.ID != "" {
			lists = append(lists, searchItem{kind: "playlist", name: p.Name, sub: p.Owner.DisplayName, uri: p.URI, entry: playlistEntry(*p)})
		}
	}
	for _, s := range r.Shows.Items {
		if s == nil {
			continue
		}
		count := F("search.episodecount", s.TotalEpisodes)
		e := libEntry{kind: kindShow, key: s.ID, uri: s.URI, name: s.Name, owner: count,
			desc: cleanDesc(s.Description), image: pickImage(s.Images), total: s.TotalEpisodes}
		shows = append(shows, searchItem{kind: "show", name: s.Name, sub: count, uri: s.URI, entry: e})
	}
	for _, ep := range r.Episodes.Items {
		if ep != nil {
			episodes = append(episodes, searchItem{kind: "episode", name: ep.Name, sub: episodeInfo(*ep), uri: ep.URI})
		}
	}
	add("track", "search.songs", tracks)
	add("artist", "search.artists", artists)
	add("album", "search.albums", albums)
	add("playlist", "search.playlists", lists)
	add("show", "search.shows", shows)
	add("episode", "search.episodes", episodes)
	return out
}

func searchTotal(r spotify.SearchResult, filter int) int {
	switch filter {
	case filterSongs:
		return r.Tracks.Total
	case filterArtists:
		return r.Artists.Total
	case filterAlbums:
		return r.Albums.Total
	case filterPlaylists:
		return r.Playlists.Total
	case filterShows:
		return r.Shows.Total
	case filterEpisodes:
		return r.Episodes.Total
	}
	return 0
}

func artistNames(as []spotify.Artist) string {
	names := make([]string, 0, len(as))
	for _, a := range as {
		names = append(names, a.Name)
	}
	return strings.Join(names, ", ")
}

// selectable: bölüm başlıkları ve aradaki boş satırlar seçilemez.
func (it searchItem) selectable() bool { return it.kind != "header" && it.kind != "space" }

// nextSelectable, başlıkları atlayarak seçilebilir bir satır bulur.
func nextSelectable(items []searchItem, i, dir int) int {
	if len(items) == 0 {
		return 0
	}
	i = max(0, min(len(items)-1, i))
	for j := i; j >= 0 && j < len(items); j += dir {
		if items[j].selectable() {
			return j
		}
	}
	for j := i; j >= 0 && j < len(items); j -= dir {
		if items[j].selectable() {
			return j
		}
	}
	return i
}

// actOn, bir satırda enter: şarkıyı ya da sanatçıyı çalar, albümü,
// listeyi ya da podcast'i açar (açılan liste döner).
func (m Model) actOn(it searchItem) (*trackList, tea.Cmd) {
	switch it.kind {
	case "track":
		album := ""
		if it.track != nil {
			album = it.track.Album.URI
		}
		ctx := it.ctx
		if ctx == "" {
			ctx = album
		}
		if ctx == "" {
			return nil, m.playOne(it.uri)
		}
		if it.history && ctx != album {
			return nil, m.playFromHistory(ctx, it.uri, album)
		}
		return nil, m.playContext(ctx, it.uri)
	case "album", "playlist", "show", "liked":
		return m.openList(it.entry)
	case "artist":
		return nil, m.playContext(it.uri, "")
	case "episode":
		return nil, m.playOne(it.uri)
	}
	return nil, nil
}

// playFromHistory, son çalınanlardan bir şarkıyı kaydedilen bağlamında çalar;
// şarkı o bağlamda değilse (otomatik devamla çalınmış) ya da bağlam bir
// sanatçıysa (Spotify sanatçıyı bir şarkıdan başlatmıyor) albümünden çalar.
// Yoksa Spotify isteği kabul edip hiçbir şey çalmıyor, çalan şarkı da duruyor.
func (m Model) playFromHistory(ctxURI, uri, album string) tea.Cmd {
	return m.playOn(func(c context.Context, sp *spotify.Client, id string) error {
		use := ctxURI
		switch {
		case strings.HasPrefix(ctxURI, "spotify:artist:"):
			use = album
		case strings.HasPrefix(ctxURI, "spotify:playlist:"):
			if has, err := sp.PlaylistHas(c, spotify.IDFromURI(ctxURI), uri); err != nil || !has {
				use = album
			}
		}
		if use == "" {
			return sp.PlayTracks(c, id, []string{uri}, "")
		}
		return sp.PlayContext(c, id, use, uri)
	})
}

func (m Model) playOne(uri string) tea.Cmd {
	return m.playOn(func(c context.Context, sp *spotify.Client, id string) error {
		return sp.PlayTracks(c, id, []string{uri}, "")
	})
}

// itemRows, bölümlü bir sonuç listesinin görünen satırları.
func (m Model) itemRows(items []searchItem, cursor, scroll, w int, active bool) []string {
	return m.itemRowsH(items, cursor, scroll, w, m.listHeight(), active)
}

// itemRowsH, itemRows'un yüksekliği verilen hâli (başlığı uzun sayfalar için).
func (m Model) itemRowsH(items []searchItem, cursor, scroll, w, h int, active bool) []string {
	playing, playingCtx := "", ""
	if m.state.Item != nil {
		playing = m.state.Item.URI
	}
	if m.state.Context != nil {
		playingCtx = m.state.Context.URI
	}
	var rows []string
	end := min(len(items), scroll+h)
	for i := scroll; i < end; i++ {
		it := items[i]
		switch it.kind {
		case "header":
			text := T(it.section)
			if it.name != "" {
				text = it.name // adı değişen başlık (örn. "X listesinden sıradakiler")
			}
			rows = append(rows, theme.Dim.Italic(true).Render(text))
			continue
		case "space":
			rows = append(rows, "")
			continue
		}
		mark := "  "
		if it.uri != "" && (it.uri == playing || it.uri == playingCtx) {
			mark = theme.GoldText.Render("♪ ")
		}
		rows = append(rows, mark+row(it.name, it.sub, w-2, active && i == cursor))
	}
	return rows
}

// enterSearch, sayfaya girilince yazı kutusunu odaklar.
func (m Model) enterSearch() Model {
	if m.search.open == nil {
		m.search.typing = true
	}
	if strings.TrimSpace(m.search.query) == "" {
		m.search.items = m.recentItems()
	}
	return m
}

// searchKey, Ara sayfası odaktayken tuşlar. Yazarken bütün harfler kutuya gider.
func (m Model) searchKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	k := msg.String()
	if m.spState != spReady {
		return m.loginKeys(k)
	}
	s := &m.search

	if s.open != nil {
		cmd, handled, back := m.listKey(s.open, k)
		if back {
			s.open = nil
		}
		return m, cmd, handled
	}

	switch k {
	case "tab", "shift+tab":
		d := 1
		if k == "shift+tab" {
			d = filterCount - 1
		}
		s.filter = (s.filter + d) % filterCount
		s.items, s.cursor, s.scroll = nil, 0, 0
		mm, cmd := m.runSearch(0)
		return mm, cmd, true
	}

	if s.typing {
		switch k {
		case "esc":
			if s.query != "" {
				// Hemen boş kutuya geç (son aramalar); beklenirse aynı sözcük
				// yeniden yazılınca "zaten arandı" sanılıp boş liste kalıyordu.
				s.query = ""
				mm, cmd := m.runSearch(0)
				return mm, cmd, true
			}
			m.focus = focusMenu
			return m, nil, true
		case "enter", "down":
			s.typing = false
			s.cursor = nextSelectable(s.items, 0, 1)
			if strings.TrimSpace(s.query) != s.last || s.filter != s.lastF {
				mm, cmd := m.runSearch(0)
				return mm, cmd, true
			}
			return m, nil, true
		case "backspace":
			if s.query != "" {
				_, size := utf8.DecodeLastRuneInString(s.query)
				s.query = s.query[:len(s.query)-size]
			}
		case "ctrl+u":
			s.query = ""
			mm, cmd := m.runSearch(0)
			return mm, cmd, true
		case "ctrl+c":
			return m, nil, false
		default:
			text := msg.Key().Text
			if k == "space" {
				text = " "
			}
			if text == "" {
				return m, nil, true // ok tuşları vb. yazarken yok sayılır
			}
			s.query += text
		}
		mm, cmd := m.scheduleSearch()
		return mm, cmd, true
	}

	h := m.listHeight()
	move := func(d int) {
		next := s.cursor + d
		if next < 0 {
			s.typing = true
			return
		}
		s.cursor = nextSelectable(s.items, next, d)
		s.cursor, s.scroll = fitScroll(s.cursor, s.scroll, len(s.items), h)
	}
	switch k {
	case "up", "k":
		move(-1)
	case "down", "j":
		move(1)
	case "/":
		s.typing = true
	case "enter", "right":
		if len(s.items) == 0 {
			s.typing = true
			return m, nil, true
		}
		it := s.items[s.cursor]
		tl, cmd := m.actOn(it)
		if tl != nil {
			s.open = tl
		}
		mm, scmd := m.rememberSearch(it)
		return mm, tea.Batch(cmd, scmd), true
	case "x", "delete":
		// Kutu boşken listelenen son aramalardan birini siler.
		if strings.TrimSpace(s.query) != "" || len(s.items) == 0 {
			return m, nil, true
		}
		mm, cmd := m.forgetSearch(s.items[s.cursor].uri)
		mm.search.items = mm.recentItems()
		mm.search.cursor = nextSelectable(mm.search.items, min(s.cursor, len(mm.search.items)-1), -1)
		if len(mm.search.items) == 0 {
			mm.search.typing = true
		}
		return mm, cmd, true
	case "esc", "left", "backspace":
		m.focus = focusMenu
	default:
		return m, nil, false
	}
	// Tek türde sonuna yaklaşınca devamını getir.
	if s.filter != filterAll && !s.loading && len(s.items) < s.total && s.cursor >= len(s.items)-3 {
		mm, cmd := m.runSearch(len(s.items))
		return mm, cmd, true
	}
	return m, nil, true
}

// ---- çizim ----

func (m Model) searchView(w int) []string {
	title := theme.Title.Render(T("page.search"))
	if rows := m.accountView(title); rows != nil {
		return rows
	}
	s := m.search
	active := m.focus == focusContent
	if s.open != nil {
		return m.listView(s.open, w, active)
	}

	// Yazı kutusu.
	cursor := ""
	if active && s.typing {
		cursor = theme.Hi.Render("▏")
	}
	var input string
	if s.query == "" {
		input = theme.Hi.Render("› ") + cursor + theme.Faint.Render(T("search.hint"))
	} else {
		input = theme.Hi.Render("› ") + theme.Plain.Render(s.query) + cursor
	}

	// Süzgeçler.
	var chips []string
	for i, key := range filterKeys {
		if i == s.filter {
			chips = append(chips, theme.Hi.Underline(true).Render(T(key)))
		} else {
			chips = append(chips, theme.Dim.Render(T(key)))
		}
	}
	rows := []string{title, input, strings.Join(chips, theme.Faint.Render(" · ")), ""}

	switch {
	case s.err != nil:
		return append(rows, theme.Error.Render(s.err.Error()))
	case strings.TrimSpace(s.query) == "" && len(s.items) == 0:
		return append(rows, theme.Faint.Render(T("search.empty")))
	case s.loading && len(s.items) == 0:
		return append(rows, theme.Dim.Render(T("lib.loading")))
	case len(s.items) == 0 && s.last != "":
		return append(rows, theme.Dim.Render(T("search.none")))
	}

	return append(rows, m.itemRows(s.items, s.cursor, s.scroll, w, active && !s.typing)...)
}
