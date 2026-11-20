package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Ehtisam/sotto/internal/bridge"
	"github.com/Ehtisam/sotto/internal/spclient"
	"github.com/Ehtisam/sotto/internal/spotify"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// VII · Profil, resmî kısım: fotoğraf, sayılar, en çok dinlenen sanatçılar
// ve şarkılar (üç zaman aralığı), son dinlenen sanatçılar, herkese açık
// listeler. Takipçi listesi ve arkadaş etkinliği ✱ sonraya.

var profileRanges = []string{spotify.ShortTerm, spotify.MediumTerm, spotify.LongTerm}

const (
	profileTopRefresh   = 10 * time.Minute
	profileCountRefresh = 5 * time.Minute
	profileTopCount     = 10
	profilePanels       = 4
	profileTopRowsMax   = 5 // üst sıradaki panellerin en çok satırı
)

type profileTop struct {
	artists []spotify.FullArtist
	tracks  []spotify.Track
	fetched time.Time
	loading bool
	err     error
}

type profileState struct {
	rng  int // profileRanges içinde
	tops [3]profileTop

	following, albums, shows int
	countsAt                 time.Time
	countsLoading            bool

	// ✱ profilde görünen listeler (iç yoldan); nil: henüz yok ya da kapalı.
	view        *spclient.Profile
	viewAt      time.Time
	viewLoading bool

	// Dört panel: 0 sanatçılar, 1 şarkılar (üst sıra), 2 son dinlenen
	// sanatçılar, 3 herkese açık listeler (alt sıra). Her birinin kendi imleci var.
	panel  int
	cursor [profilePanels]int
	scroll [profilePanels]int
	open   *trackList
}

type profileTopMsg struct {
	rng     int
	artists []spotify.FullArtist
	tracks  []spotify.Track
	err     error
}

type profileViewMsg struct {
	view *spclient.Profile
}

type profileCountsMsg struct {
	following, albums, shows int
	err                      error
}

func (m Model) loadProfileTop(rng int) tea.Cmd {
	sp, tr := m.sp, profileRanges[rng]
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		msg := profileTopMsg{rng: rng}
		var wg sync.WaitGroup
		var e1, e2 error
		wg.Add(2)
		go func() { defer wg.Done(); msg.artists, e1 = sp.TopArtists(ctx, tr, profileTopCount) }()
		go func() { defer wg.Done(); msg.tracks, e2 = sp.TopTracks(ctx, tr, profileTopCount) }()
		wg.Wait()
		msg.err = firstErr(e1, e2)
		return msg
	}
}

func (m Model) loadProfileCounts() tea.Cmd {
	sp := m.sp
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		var msg profileCountsMsg
		var wg sync.WaitGroup
		var e1, e2, e3 error
		wg.Add(3)
		go func() { defer wg.Done(); msg.following, e1 = sp.FollowedArtists(ctx) }()
		go func() { defer wg.Done(); msg.albums, e2 = sp.SavedAlbums(ctx) }()
		go func() { defer wg.Done(); msg.shows, e3 = sp.SavedShows(ctx) }()
		wg.Wait()
		msg.err = firstErr(e1, e2, e3)
		return msg
	}
}

// loadProfileView, profilde görünen listeleri iç yoldan ister (✱). Hesap
// bağlanmamışsa ya da bir şey ters giderse sessizce vazgeçer; sayfa o zaman
// resmî bilgiyle yetinir.
func (m Model) loadProfileView() tea.Cmd {
	br, id := m.rt.bridge, m.me.ID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		for try := 0; try < 2; try++ {
			tok, err := br.Token(ctx)
			var need *bridge.LoginNeeded
			if errors.As(err, &need) || err != nil {
				return profileViewMsg{}
			}
			v, err := spclient.ProfileView(ctx, tok, id)
			if errors.Is(err, spclient.ErrUnauthorized) {
				br.Invalidate()
				continue
			}
			if err != nil {
				return profileViewMsg{}
			}
			return profileViewMsg{&v}
		}
		return profileViewMsg{}
	}
}

// maybeLoadProfileView, profildeki listeleri (✱) eksik ya da eskiyse ister.
// Profil sayfası ve Kitaplık (işaretler, "Profilinde" süzgeci) kullanır.
func (m *Model) maybeLoadProfileView() tea.Cmd {
	p := &m.prof
	if !m.cfg.Extras.Enabled || m.spState != spReady || m.me.ID == "" || p.viewLoading ||
		(!p.viewAt.IsZero() && m.now.Sub(p.viewAt) < profileCountRefresh) {
		return nil
	}
	p.viewLoading = true
	return m.loadProfileView()
}

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

// maybeLoadProfile, sayfaya girilince ya da aralık değişince eskimiş olanı tazeler.
func (m *Model) maybeLoadProfile() tea.Cmd {
	if m.spState != spReady {
		return nil
	}
	p := &m.prof
	var cmds []tea.Cmd
	if t := &p.tops[p.rng]; !t.loading && (t.fetched.IsZero() || m.now.Sub(t.fetched) > profileTopRefresh) {
		t.loading = true
		cmds = append(cmds, m.loadProfileTop(p.rng))
	}
	if !p.countsLoading && (p.countsAt.IsZero() || m.now.Sub(p.countsAt) > profileCountRefresh) {
		p.countsLoading = true
		cmds = append(cmds, m.loadProfileCounts())
	}
	cmds = append(cmds, m.maybeLoadProfileView())
	return tea.Batch(append(cmds, m.maybeLoadHome())...) // son dinlenen sanatçılar ana sayfanın verisinden
}

func (m Model) updateProfile(msg tea.Msg) (tea.Model, tea.Cmd) {
	p := &m.prof
	switch msg := msg.(type) {
	case profileTopMsg:
		t := &p.tops[msg.rng]
		t.loading, t.fetched, t.err = false, m.now, msg.err
		if msg.err != nil && len(msg.artists) == 0 && len(msg.tracks) == 0 {
			return m.spotifyFailed(msg.err), nil
		}
		t.artists, t.tracks = msg.artists, msg.tracks
		for _, tr := range msg.tracks {
			m.rt.known[tr.URI] = tr
		}
	case profileViewMsg:
		p.viewLoading, p.viewAt = false, m.now
		if msg.view != nil {
			p.view = msg.view
		}
	case profileCountsMsg:
		p.countsLoading, p.countsAt = false, m.now
		if msg.err == nil {
			p.following, p.albums, p.shows = msg.following, msg.albums, msg.shows
		}
	}
	return m, nil
}

// profilePanels, dört panelin başlıkları ve satırları; her çizimde durumdan kurulur.
func (m Model) profilePanels() (titles [profilePanels]string, items [profilePanels][]searchItem) {
	p := m.prof
	rangeName := T("prof.range." + profileRanges[p.rng])
	top := p.tops[p.rng]

	titles[0] = F("prof.topartists", rangeName)
	for i, a := range top.artists {
		items[0] = append(items[0], searchItem{kind: "artist", name: a.Name, sub: rankLabel(i), uri: a.URI})
	}

	titles[1] = T("prof.toptracks")
	for i := range top.tracks {
		t := top.tracks[i]
		items[1] = append(items[1], searchItem{kind: trackKind(t), name: t.Name, sub: t.ArtistNames(), uri: t.URI, track: &top.tracks[i]})
	}

	// Son dinlenen sanatçılar: son çalınanlardan, her sanatçı bir kez.
	titles[2] = T("prof.recentartists")
	seen := map[string]bool{}
	for _, r := range m.home.recent {
		if len(r.Track.Artists) == 0 || len(items[2]) >= 10 {
			continue
		}
		a := r.Track.Artists[0]
		if a.URI == "" || seen[a.URI] {
			continue
		}
		seen[a.URI] = true
		items[2] = append(items[2], searchItem{kind: "artist", name: a.Name, sub: ago(m.now, r.PlayedAt), uri: a.URI})
	}

	// Profilde görünen listeler iç yoldan geliyor; o yoksa (✱ kapalı)
	// herkese açık listeler, adı da öyle yazılarak.
	if p.view != nil {
		titles[3] = T("prof.onprofile")
		for _, pl := range p.view.Playlists {
			e := m.libEntryFor(pl)
			sub := pl.OwnerName
			if e.total > 0 {
				sub = F("lib.songs", e.total)
			}
			items[3] = append(items[3], searchItem{kind: "playlist", name: e.name, sub: sub, uri: e.uri, entry: e})
		}
	} else {
		titles[3] = T("prof.public")
		for _, e := range m.publicPlaylists() {
			items[3] = append(items[3], searchItem{kind: "playlist", name: e.name, sub: F("lib.songs", e.total), uri: e.uri, entry: e})
		}
	}
	return titles, items
}

// libEntryFor, profildeki listeyi kitaplıktaki karşılığıyla eşler; kitaplıkta
// yoksa (takip edilmiyorsa) eldeki bilgiyle kurar.
func (m Model) libEntryFor(pl spclient.ProfilePlaylist) libEntry {
	for _, e := range m.lib.entries {
		if e.uri == pl.URI {
			return e
		}
	}
	return libEntry{kind: kindPlaylist, key: spotify.IDFromURI(pl.URI), uri: pl.URI, name: pl.Name,
		owner: pl.OwnerName, image: pl.ImageURL}
}

// publicPlaylists, kullanıcının kendi, herkese açık listeleri.
func (m Model) publicPlaylists() []libEntry {
	var out []libEntry
	for _, e := range m.lib.entries {
		if e.kind == kindPlaylist && e.public && m.me.ID != "" && e.ownerID == m.me.ID {
			out = append(out, e)
		}
	}
	return out
}

func rankLabel(i int) string { return fmt.Sprintf("#%d", i+1) }

// profileSelected, seçili satır (eylem menüsü için).
func (m Model) profileSelected() (searchItem, bool) {
	_, items := m.profilePanels()
	p := m.prof
	list := items[p.panel]
	if len(list) == 0 {
		return searchItem{}, false
	}
	return list[min(p.cursor[p.panel], len(list)-1)], true
}

// profileRows, üst ve alt sıradaki panellerin satır sayıları (başlıklar hariç).
func (m Model) profileRows(items [profilePanels][]searchItem) (topRows, bottomRows int) {
	topRows = max(min(profileTopRowsMax, max(len(items[0]), len(items[1]))), 1)
	// başlık + üst satırlar + boş + başlık + alt satırlar
	bottomRows = max(m.profileListHeight()-topRows-3, 1)
	return topRows, bottomRows
}

func (m Model) panelRows(panel int, items [profilePanels][]searchItem) int {
	top, bottom := m.profileRows(items)
	if panel < 2 {
		return top
	}
	return bottom
}

func (m Model) profileKey(k string) (Model, tea.Cmd, bool) {
	if m.spState != spReady {
		return m.loginKeys(k)
	}
	p := &m.prof
	if p.open != nil {
		cmd, handled, back := m.listKey(p.open, k)
		if back {
			p.open = nil
		}
		return m, cmd, handled
	}
	_, items := m.profilePanels()
	// focus, boş olmayan bir panele geçer; yoksa yerinde kalır.
	focus := func(panel int) bool {
		if panel < 0 || panel >= profilePanels || len(items[panel]) == 0 {
			return false
		}
		p.panel = panel
		return true
	}
	move := func(d int) {
		n := len(items[p.panel])
		c := p.cursor[p.panel] + d
		switch {
		case n > 0 && c >= 0 && c < n:
			p.cursor[p.panel] = c
		case c < 0:
			focus(p.panel - 2) // üst sıraya çık
		case c >= n:
			focus(p.panel + 2) // alt sıraya in
		}
		rows := m.panelRows(p.panel, items)
		p.cursor[p.panel], p.scroll[p.panel] = fitScroll(p.cursor[p.panel], p.scroll[p.panel], len(items[p.panel]), rows)
	}
	switch k {
	case "up", "k":
		move(-1)
	case "down", "j":
		move(1)
	case "right":
		if p.panel%2 == 0 {
			if !focus(p.panel+1) && p.panel == 0 {
				focus(3)
			}
		}
	case "left":
		if p.panel%2 == 0 {
			m.focus = focusMenu
		} else if !focus(p.panel-1) && p.panel == 3 {
			focus(0)
		}
	case "tab", "shift+tab":
		// Sıradaki (boş olmayan) panele: sanatçılar → şarkılar → son dinlenenler → profilinde.
		d := 1
		if k == "shift+tab" {
			d = profilePanels - 1
		}
		for i, next := 0, p.panel; i < profilePanels-1; i++ {
			next = (next + d) % profilePanels
			if focus(next) {
				break
			}
		}
	case "pgdown", "pgup":
		// Alttaki ya da üstteki panele doğrudan atla (aynı sütun).
		if k == "pgdown" {
			focus(p.panel + 2)
		} else {
			focus(p.panel - 2)
		}
	case "home", "g":
		p.cursor[p.panel], p.scroll[p.panel] = 0, 0
	case "end", "G":
		move(1 << 20)
	case "[", "]":
		d := 1
		if k == "[" {
			d = len(profileRanges) - 1
		}
		p.rng = (p.rng + d) % len(profileRanges)
		p.cursor[0], p.cursor[1], p.scroll[0], p.scroll[1] = 0, 0, 0, 0
		return m, m.maybeLoadProfile(), true
	case "enter":
		it, ok := m.profileSelected()
		if !ok {
			return m, nil, true
		}
		tl, cmd := m.actOn(it)
		if tl != nil {
			p.open = tl
		}
		return m, cmd, true
	case "e":
		if m.me.ID == "" {
			return m, nil, true
		}
		if err := openBrowser(spotify.ProfileLink(m.me.ID)); err != nil {
			return m.say(err.Error()), nil, true
		}
		return m.note(T("prof.opened")), nil, true
	case "R":
		p.tops[p.rng].fetched, p.countsAt, m.home.fetched = time.Time{}, time.Time{}, time.Time{}
		return m, m.maybeLoadProfile(), true
	case "esc", "backspace":
		m.focus = focusMenu
	default:
		return m, nil, false
	}
	return m, nil, true
}

// profilePhoto: fotoğraf yeterince yer varsa başlıkta gösterilir.
func (m Model) profilePhoto() (url string, cols, rows int) {
	url = pickImage(m.me.Images)
	if url == "" || m.height < 30 || !m.cfg.Look.CoversOn() {
		return "", 0, 0
	}
	return url, 10, 5
}

// profileHeader, ad, fotoğraf ve sayılar.
func (m Model) profileHeader() []string {
	p := m.prof
	name := m.me.DisplayName
	if name == "" {
		name = m.me.ID
	}
	var stats1, stats2 []string
	switch m.me.Product {
	case "premium":
		stats1 = append(stats1, "Premium")
	case "free", "open":
		stats1 = append(stats1, "Free")
	}
	stats1 = append(stats1, F("prof.followers", m.me.Followers.Total))
	if !p.countsAt.IsZero() {
		stats1 = append(stats1, F("prof.following", p.following))
	}
	playlists, liked := 0, -1
	for _, e := range m.lib.entries {
		switch e.kind {
		case kindPlaylist:
			playlists++
		case kindLiked:
			liked = e.total
		}
	}
	if m.lib.loaded || playlists > 0 {
		stats2 = append(stats2, F("lib.playlists", playlists))
	}
	if p.view != nil {
		stats2 = append(stats2, F("prof.onprofile.n", p.view.Total))
	}
	if liked >= 0 {
		stats2 = append(stats2, F("prof.liked", liked))
	}
	if !p.countsAt.IsZero() {
		stats2 = append(stats2, F("prof.albums", p.albums), F("prof.shows", p.shows))
	}
	sep := theme.Faint.Render(" · ")
	join := func(parts []string) string {
		out := make([]string, len(parts))
		for i, s := range parts {
			out[i] = theme.Dim.Render(s)
		}
		return strings.Join(out, sep)
	}
	info := []string{theme.Hi.Bold(true).Render(name), join(stats1), join(stats2)}

	url, cw, ch := m.profilePhoto()
	if url == "" {
		return info
	}
	info = append([]string{""}, info...) // fotoğrafın ortasına yakın dursun
	for len(info) < ch {
		info = append(info, "")
	}
	block := lipgloss.JoinHorizontal(lipgloss.Top, m.coverBox(url, cw, ch), "   ", strings.Join(info, "\n"))
	return strings.Split(block, "\n")
}

// profileTopRows: başlık, alt satır, boş, profil bilgisi, boş.
func (m Model) profileTopRows() int {
	_, _, ch := m.profilePhoto()
	return 3 + max(ch, 3) + 1
}

// profileListHeight, panellere kalan yükseklik.
func (m Model) profileListHeight() int { return max(m.height-7-m.profileTopRows(), 1) }

func (m Model) profileView(w int) []string {
	title := theme.Title.Render(T("page.profile"))
	if rows := m.accountView(title); rows != nil {
		return rows
	}
	p := m.prof
	active := m.focus == focusContent
	if p.open != nil {
		return m.listView(p.open, w, active)
	}
	if m.me.ID == "" {
		return []string{title, "", theme.Dim.Render(T("lib.loading"))}
	}
	rows := []string{title, theme.Faint.Render(T("prof.sub")), ""}
	rows = append(rows, m.profileHeader()...)
	rows = append(rows, "")

	titles, items := m.profilePanels()
	top := p.tops[p.rng]
	if len(items[0]) == 0 && len(items[1]) == 0 {
		switch {
		case top.err != nil:
			rows = append(rows, theme.Error.Render(top.err.Error()))
		case top.loading || top.fetched.IsZero():
			rows = append(rows, theme.Dim.Render(T("lib.loading")))
		default:
			rows = append(rows, theme.Dim.Render(T("prof.empty")))
		}
		return rows
	}

	const gap = 3
	colW := max((w-gap)/2, 10)
	topRows, bottomRows := m.profileRows(items)
	panel := func(i, h int) string {
		return m.profilePanel(titles[i], items[i], p.cursor[i], p.scroll[i], colW, h, active && p.panel == i)
	}
	pair := func(a, b, h int) []string {
		block := lipgloss.JoinHorizontal(lipgloss.Top, panel(a, h), strings.Repeat(" ", gap), panel(b, h))
		return strings.Split(block, "\n")
	}
	rows = append(rows, pair(0, 1, topRows)...)
	rows = append(rows, "")
	return append(rows, pair(2, 3, bottomRows)...)
}

// profilePanel, başlığı ve en çok h satırı olan bir sütun. Aşağıda kalan
// satırlar varsa son satırda kaç tane olduğu yazar.
func (m Model) profilePanel(title string, items []searchItem, cursor, scroll, w, h int, active bool) string {
	head := theme.Dim.Italic(true).Render(truncate(title, w))
	if active {
		head = theme.Hi.Italic(true).Render(truncate(title, w))
	}
	lines := []string{head}
	if len(items) == 0 {
		lines = append(lines, theme.Faint.Render(truncate("  "+T("prof.none"), w)))
	} else {
		show := h
		if len(items) > h && h > 1 {
			show = h - 1 // son satır "↓ n daha" için
		}
		cursor, scroll = fitScroll(cursor, scroll, len(items), show)
		playing := ""
		if m.state.Item != nil {
			playing = m.state.Item.URI
		}
		end := min(len(items), scroll+show)
		for i := scroll; i < end; i++ {
			it := items[i]
			mark := "  "
			if it.uri != "" && it.uri == playing {
				mark = theme.GoldText.Render("♪ ")
			}
			lines = append(lines, mark+row(it.name, it.sub, w-2, active && i == cursor))
		}
		if rest := len(items) - end; rest > 0 && show < h {
			lines = append(lines, theme.Faint.Render(F("prof.more", rest)))
		}
	}
	return lipgloss.NewStyle().Width(w).Height(h + 1).MaxHeight(h + 1).Render(strings.Join(lines, "\n"))
}
