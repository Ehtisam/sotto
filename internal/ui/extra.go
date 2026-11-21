package ui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"charm.land/lipgloss/v2"

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
	extraArtist  // sanatçı sayfası (artist.go)
	extraInfo    // ayrıntılar ve künye (info.go)
	extraNews    // Yenilikler (news.go)
	extraPeople  // takipçiler, takip edilenler, arkadaş etkinliği (people.go)
	extraUser    // başka bir kullanıcının profili (people.go)
	extraHistory // dinleme geçmişi ve istatistik (histpage.go)
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

	artist *artistState
	info   *infoState
	news   *newsState
	people *peopleState
	user   *userState
	hist   *histState

	// prev: bu sayfa başka bir ek sayfanın üstünde açıldıysa o (esc ona döner).
	prev *extraState
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
	case extraArtist:
		x.items = m.artistItems()
	case extraInfo:
		x.items = m.infoItems()
	case extraNews:
		x.items = m.newsItems()
	case extraPeople:
		x.items = m.peopleItems()
	case extraUser:
		x.items = m.userItems()
	case extraHistory:
		x.items = m.histItems()
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

// extraHeight, ek sayfada liste satırlarına kalan yükseklik (fotoğraf varsa altı).
func (m Model) extraHeight() int {
	if _, _, rows := m.extraPhoto(); rows > 0 {
		return max(m.listHeight()-rows-1, 1)
	}
	return m.listHeight()
}

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
	if !anySelectable(x.items) { // yalnız yazı (Hakkında): j/k kaydırır
		scroll := func(d int) {
			x.scroll = max(0, min(x.scroll+d, len(x.items)-lh))
		}
		switch k {
		case "up", "k":
			scroll(-1)
			return m, nil, true
		case "down", "j":
			scroll(1)
			return m, nil, true
		case "pgup":
			scroll(-lh)
			return m, nil, true
		case "pgdown":
			scroll(lh)
			return m, nil, true
		case "home", "g":
			scroll(-1 << 30)
			return m, nil, true
		case "end", "G":
			scroll(1 << 30)
			return m, nil, true
		}
	}
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
	case "tab", "shift+tab":
		d := 1
		if k == "shift+tab" {
			d = -1
		}
		switch x.page {
		case extraArtist:
			mm, cmd := m.switchArtistSection(d)
			return mm, cmd, true
		case extraPeople:
			mm, cmd := m.switchPeopleSection(d)
			return mm, cmd, true
		case extraNews:
			mm, cmd := m.switchNewsSection(d)
			return mm, cmd, true
		case extraHistory:
			mm, cmd := m.switchHistRange(d)
			return mm, cmd, true
		}
		return m, nil, false
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
		if mm, cmd, ok := m.reloadExtra(); ok {
			return mm, cmd, true
		}
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
		if x.page == extraUser && x.user != nil {
			return m.openUserInBrowser(), nil, true
		}
		if x.page != extraLog {
			return m, nil, false
		}
		if err := openBrowser(logFile(m.cfg.LocalDir)); err != nil {
			return m.say(err.Error()), nil, true
		}
		return m, nil, true
	case "esc", "left", "backspace":
		if x.prev != nil && x.prev.host == m.page {
			m.extra = *x.prev
			m.rebuildExtra()
			return m, nil, true
		}
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
	if head, ok := m.extraHeader(w); ok {
		rows := head
		if _, cw, _ := m.extraPhoto(); cw > 0 {
			rows = append(rows, m.photoBlock(m.photoText(w-cw-3))...)
		}
		switch {
		case len(x.items) == 0:
			text, bad := m.extraEmpty()
			if bad {
				return append(rows, theme.Error.Render(truncate(text, w)))
			}
			return append(rows, theme.Dim.Render(truncate(text, w)))
		}
		return append(rows, m.itemRowsH(x.items, x.cursor, x.scroll, w, m.extraHeight(), active && anySelectable(x.items))...)
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
		lh := m.extraHeight()
		bw := w - scrollbarW(len(x.items), lh)
		top := len(rows)
		end := min(len(x.items), x.scroll+lh)
		for i := x.scroll; i < end; i++ {
			it := x.items[i]
			text := theme.Plain
			if it.kind == "logerr" {
				text = theme.Error
			}
			line := theme.Faint.Render(it.sub+"  ") + text.Render(truncate(it.name, bw-10))
			if active && i == x.cursor {
				line = theme.SelectedRow.Width(bw).Render(truncate(it.sub+"  "+it.name, bw))
			}
			rows = append(rows, line)
		}
		return withScrollbar(rows, top, len(x.items), x.scroll, lh, w)
	}
	return append(rows, m.itemRowsH(x.items, x.cursor, x.scroll, w, m.extraHeight(), active)...)
}

// anySelectable: satırlardan en az biri seçilebilir.
func anySelectable(items []searchItem) bool {
	for _, it := range items {
		if it.selectable() {
			return true
		}
	}
	return false
}

// extraHeader, kendi başlığını çizen ek sayfaların başlığı (4 satır).
func (m Model) extraHeader(w int) ([]string, bool) {
	switch m.extra.page {
	case extraHistory:
		return m.histHeader(w), true
	case extraArtist:
		return m.artistHeader(w), true
	case extraInfo:
		return m.infoHeader(w), true
	case extraNews:
		return m.newsHeader(w), true
	case extraPeople:
		return m.peopleHeader(w), true
	case extraUser:
		return m.userHeader(w), true
	}
	return nil, false
}

// extraEmpty, satırı olmayan ek sayfada söylenecek (hata ise bad).
func (m Model) extraEmpty() (text string, bad bool) {
	switch m.extra.page {
	case extraArtist:
		return m.artistEmpty()
	case extraInfo:
		return m.infoEmpty()
	case extraNews:
		return m.newsEmpty()
	case extraPeople:
		return m.peopleEmpty()
	case extraUser:
		return m.userEmpty()
	case extraHistory:
		if h := m.extra.hist; h != nil && h.err != nil {
			return h.err.Error(), true
		}
		return T("hist.empty"), false
	}
	return T("extra.empty"), false
}

// reloadExtra, R: kendi verisini yükleyen ek sayfaları yeniden ister.
func (m Model) reloadExtra() (Model, tea.Cmd, bool) {
	x := &m.extra
	switch x.page {
	case extraArtist:
		a := x.artist
		a.viewLoading, a.albumsAsked = true, false
		cmds := []tea.Cmd{m.loadArtistView(a)}
		if a.section == artDisco {
			a.albumsAsked, a.albumsLoading = true, true
			cmds = append(cmds, m.loadArtistAlbums(a))
		}
		return m, tea.Batch(cmds...), true
	case extraInfo:
		return m, m.loadInfo(x.info), true
	case extraHistory:
		m.loadHistory()
		m.rebuildExtra()
		return m, nil, true
	case extraNews:
		x.news.loading = true
		return m, m.loadNews(), true
	case extraPeople:
		return m, m.loadPeople(x.people.section), true
	case extraUser:
		x.user.loading = true
		return m, m.loadUser(x.user.id), true
	}
	return m, nil, false
}

// openLists, sayfalarda ve ek sayfalarda (yığındakiler dahil) açık listeler.
func (m Model) openLists() []*trackList {
	out := []*trackList{m.lib.open, m.search.open, m.home.open, m.prof.open}
	for x := &m.extra; x != nil; x = x.prev {
		if x.open != nil {
			out = append(out, x.open)
		}
	}
	return out
}

// extraPhoto: sanatçının Hakkında bölümünde ve kullanıcı profilinde fotoğraf
// (pencere yetiyorsa). Fotoğrafın altında listeye en az 6 satır kalır.
func (m Model) extraPhoto() (url string, cols, rows int) {
	if !m.cfg.Look.CoversOn() || !m.extraOn() || m.extra.open != nil {
		return "", 0, 0
	}
	x := m.extra
	switch {
	case x.page == extraArtist && x.artist != nil && x.artist.section == artAbout:
		if v := x.artist.view; v != nil {
			url = v.Image
		}
		if in := x.artist.info; url == "" && in != nil {
			url = pickImage(in.Images)
		}
	case x.page == extraUser && x.user != nil && x.user.view != nil:
		url = x.user.view.Image
	}
	if url == "" {
		return "", 0, 0
	}
	rows = min(8, m.listHeight()-1-6, (m.contentWidth()-3-24)/2)
	if rows < 4 {
		return "", 0, 0
	}
	return url, rows * 2, rows
}

// photoBlock, fotoğraf ve yanındaki yazılar (dikeyde ortalı) ile altına bir boş satır.
func (m Model) photoBlock(lines []string) []string {
	url, cw, ch := m.extraPhoto()
	if url == "" {
		return nil
	}
	for i := 0; i < (ch-len(lines))/2; i++ {
		lines = append([]string{""}, lines...)
	}
	for len(lines) < ch {
		lines = append(lines, "")
	}
	block := lipgloss.JoinHorizontal(lipgloss.Top, m.coverBox(url, cw, ch), "   ", strings.Join(lines[:ch], "\n"))
	return append(strings.Split(block, "\n"), "")
}

// photoText, fotoğrafın yanına yazılacaklar.
func (m Model) photoText(w int) []string {
	x := m.extra
	var out []string
	switch x.page {
	case extraArtist:
		for _, it := range m.artistFacts(x.artist) {
			out = append(out, row(it.name, it.sub, w, false))
		}
	case extraUser:
		v := x.user.view
		out = append(out, theme.Plain.Render(F("people.nfollowers", groupDigits(v.Followers))),
			theme.Plain.Render(F("people.nfollowing", groupDigits(v.FollowingN))))
		if v.Following {
			out = append(out, theme.Hi.Render(T("people.youfollow")))
		}
	}
	return out
}
