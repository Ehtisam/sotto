package ui

import (
	"errors"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/spclient"
	"github.com/Ehtisam/sotto/internal/spotify"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// IV · Kitaplık: listeler ve beğenilenler; birine girilince şarkıları.

type library struct {
	loading bool
	loaded  bool
	err     error
	entries []libEntry
	filter  libFilter
	cursor  int // libVisible içinde
	scroll  int

	open *trackList // nil: liste düzeyi

	root   *spclient.Rootlist // kitaplığın sırası ve klasörleri (✱, folders.go)
	folder string             // "Hepsi"nde içinde bulunulan klasör ("" kök)

	// Kayıtlı albümler, sanatçılar, podcast'ler (library_saved.go).
	saved        []libEntry
	savedLoaded  bool
	savedLoading bool
	savedErr     error

	find   finder // sayfa içi arama: bütün listelerde (klasörlerin içi dahil)
	sortBy string // sort.go; "" kendi sırası (klasörlerle)
	desc   bool
}

// Kitaplık süzgeçleri; tab ile geçilir.
type libFilter int

const (
	libAll       libFilter = iota
	libMine                // senin listelerin (ve beğenilenler)
	libPublic              // senin, herkese açık listelerin
	libOnProfile           // profilinde görünenler (✱)
	libFollowed            // başkalarının, takip ettiğin listeler
	libAlbums              // kitaplıktaki albümler
	libArtists             // takip edilen sanatçılar
	libPodcasts            // kitaplıktaki podcast'ler
	libFilterCount
)

var libFilterKeys = [libFilterCount]string{"lib.f.all", "lib.f.mine", "lib.f.public", "lib.f.profile", "lib.f.followed",
	"lib.f.albums", "lib.f.artists", "lib.f.podcasts"}

// libFilters, gösterilecek süzgeçler: iç yol kapalıysa "Profilinde" yok.
func (m Model) libFilters() []libFilter {
	out := []libFilter{libAll, libMine, libPublic}
	if m.cfg.Extras.Enabled {
		out = append(out, libOnProfile)
	}
	return append(out, libFollowed, libAlbums, libArtists, libPodcasts)
}

func (m Model) mine(e libEntry) bool {
	return e.kind == kindLiked || (m.me.ID != "" && e.ownerID == m.me.ID)
}

// inFilter, girişin süzgece uyup uymadığı.
func (m Model) inFilter(e libEntry, f libFilter) bool {
	switch f {
	case libMine:
		return m.mine(e)
	case libPublic:
		return e.kind == kindPlaylist && e.public && m.mine(e)
	case libOnProfile:
		return m.onProfile(e)
	case libFollowed:
		return e.kind == kindPlaylist && !m.mine(e)
	case libAlbums, libArtists, libPodcasts:
		return e.kind == savedKind(f)
	}
	return true
}

// onProfile: liste profilde görünüyor (✱ bilgisi gelmediyse hep false).
func (m Model) onProfile(e libEntry) bool {
	if m.prof.view == nil {
		return false
	}
	for _, pl := range m.prof.view.Playlists {
		if pl.URI == e.uri {
			return true
		}
	}
	return false
}

// libVisible, seçili süzgece uyan girişler.
func (m Model) libVisible() []libEntry {
	vis := m.libFiltered()
	if m.lib.sortBy != sortCustom {
		pos := make(map[string]int, len(m.lib.entries)+len(m.lib.saved))
		for i, e := range m.lib.entries {
			pos[e.key] = i
		}
		for i, e := range m.lib.saved {
			pos[e.key] = len(m.lib.entries) + i
		}
		vis = sortEntries(vis, m.lib.sortBy, m.lib.desc, pos)
	}
	if q := m.lib.find.query; q != "" {
		order := matchOrder(nil, len(vis), func(i int) string { return vis[i].name + " " + vis[i].owner }, q)
		out := make([]libEntry, 0, len(order))
		for _, i := range order {
			out = append(out, vis[i])
		}
		return out
	}
	return vis
}

// libFiltered, süzgece uyan girişler. Sayfa içi arama açıkken klasörlerin
// içindekiler de (düz liste olarak).
func (m Model) libFiltered() []libEntry {
	if k := savedKind(m.lib.filter); k != "" {
		return m.savedOf(k)
	}
	if m.lib.filter == libAll && m.lib.find.query != "" {
		return append(append([]libEntry(nil), m.lib.entries...), m.lib.saved...)
	}
	if m.lib.filter == libAll {
		if m.foldersOn() {
			out := m.levelEntries(m.lib.folder)
			if m.lib.folder == "" { // kayıtlı albüm/sanatçı/podcast kökte, listelerin ardından
				out = append(append([]libEntry(nil), out...), m.lib.saved...)
			}
			return out
		}
		return append(append([]libEntry(nil), m.lib.entries...), m.lib.saved...)
	}
	var out []libEntry
	for _, e := range m.lib.entries {
		if m.inFilter(e, m.lib.filter) {
			out = append(out, e)
		}
	}
	return out
}

func (m *Model) clampLib() {
	l := &m.lib
	h := m.listHeight()
	l.cursor, l.scroll = fitScroll(l.cursor, l.scroll, len(m.libVisible()), h)
	if l.open != nil {
		l.open.clamp(h)
	}
}

// cycleLibFilter, sıradaki süzgece geçer. "Profilinde" seçilince ve profil
// bilgisi henüz yoksa iç yoldan ister.
func (m Model) cycleLibFilter(d int) (Model, tea.Cmd) {
	fs := m.libFilters()
	i := 0
	for j, f := range fs {
		if f == m.lib.filter {
			i = j
		}
	}
	m.lib.filter = fs[(i+d+len(fs))%len(fs)]
	m.lib.cursor, m.lib.scroll = 0, 0
	return m, m.maybeLoadProfileView()
}

type playlistsMsg struct {
	entries []libEntry
	err     error
}

func (m Model) loadLibrary() tea.Cmd {
	rt := m.rt
	sp := m.sp
	return func() tea.Msg {
		ctx, cancel := apiCtx(rt, 60*time.Second)
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
		ownerID: pl.Owner.ID, collaborative: pl.Collaborative,
		public: pl.Public != nil && *pl.Public,
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
		m.clampLib()
		if !m.home.fetched.IsZero() {
			m.rebuildHome() // liste adları artık biliniyor
		}
		var saved tea.Cmd
		if !m.lib.savedLoaded && !m.lib.savedLoading {
			m.lib.savedLoading = true
			saved = m.loadSaved()
		}
		return m, tea.Batch(m.loadRootlist(), saved) // klasörler ve kitaplık sırası
	case tracksPageMsg:
		for _, tl := range m.openLists() {
			if tl.receive(msg) {
				if msg.err != nil {
					return m.spotifyFailed(msg.err), nil
				}
				tl.clamp(m.listHeight())
				if cmd := m.continueSeek(tl); cmd != nil {
					return m, cmd
				}
				if cmd := m.continueLoadAll(tl); cmd != nil {
					return m, cmd
				}
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
	vis := m.libVisible()
	move := func(d int) {
		l.cursor = max(0, min(len(vis)-1, l.cursor+d))
		l.cursor, l.scroll = fitScroll(l.cursor, l.scroll, len(vis), h)
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
	case "tab", "shift+tab":
		d := 1
		if k == "shift+tab" {
			d = -1
		}
		mm, cmd := m.cycleLibFilter(d)
		return mm, cmd, true
	case "enter", "right":
		if len(l.entries) == 0 {
			if !l.loading {
				l.loading = true
				return m, m.loadLibrary(), true
			}
			return m, nil, true
		}
		if len(vis) == 0 {
			return m, nil, true
		}
		if e := vis[min(l.cursor, len(vis)-1)]; e.kind == kindFolder {
			return m.enterFolder(e.key), nil, true
		} else if e.kind == kindArtist {
			return m, openArtistCmd(e.uri, e.name), true
		}
		tl, cmd := m.openList(vis[min(l.cursor, len(vis)-1)])
		l.open = tl
		return m, cmd, true
	case "x", "delete":
		// Kendi listende ya da klasörde: silme onayı.
		if len(vis) == 0 {
			return m, nil, true
		}
		e := vis[min(l.cursor, len(vis)-1)]
		if !m.ownList(e) && e.kind != kindFolder {
			return m, nil, true
		}
		m.actions = actionMenu{open: true, target: entryItem(e), confirm: true, saved: -1}
		return m, nil, true
	case "R":
		m.refreshNow()
		l.loading, l.savedLoading, l.savedLoaded = true, true, false
		return m, tea.Batch(m.loadLibrary(), m.loadSaved()), true
	case "f":
		if !m.foldersOn() {
			return m.say(T("folder.extras")), nil, true
		}
		return m.openFolderForm(nil), nil, true
	case "K", "J": // shift+J aşağı, shift+K yukarı
		if l.sortBy != sortCustom || l.find.query != "" {
			return m.say(T("find.nomove")), nil, true
		}
		d := 1
		if k == "K" {
			d = -1
		}
		mm, cmd := m.moveInLibrary(d)
		return mm, cmd, true
	case "esc", "left", "backspace":
		if l.folder != "" && m.foldersOn() && l.filter == libAll {
			return m.leaveFolder(), nil, true
		}
		m.focus = focusMenu
	default:
		return m, nil, false
	}
	return m, nil, true
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
	if path := m.folderTitle(); path != "" {
		title += theme.Faint.Render(" › ") + theme.Title.Render(path)
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
		if l.savedLoaded {
			for _, c := range []struct {
				kind, one, many string
			}{{kindAlbum, "lib.n.album", "lib.n.albums"}, {kindArtist, "lib.n.artist", "lib.n.artists"}, {kindShow, "lib.n.podcast", "lib.n.podcasts"}} {
				switch n := len(m.savedOf(c.kind)); n {
				case 0:
				case 1:
					sub += theme.Faint.Render(" · " + T(c.one))
				default:
					sub += theme.Faint.Render(" · " + F(c.many, n))
				}
			}
		}
	} else if l.err != nil {
		sub = theme.Error.Render(l.err.Error())
	}

	// Süzgeçler: her birinin yanında kaç liste olduğu.
	var names, counts []string
	sel := 0
	for i, f := range m.libFilters() {
		names = append(names, T(libFilterKeys[f]))
		count := ""
		if f == l.filter {
			sel = i
		}
		if f != libAll && l.loaded && (f != libOnProfile || m.prof.view != nil) {
			n := 0
			for _, e := range l.entries {
				if e.kind == kindPlaylist && m.inFilter(e, f) {
					n++
				}
			}
			if k := savedKind(f); k != "" {
				n = len(m.savedOf(k))
			}
			if savedKind(f) == "" || l.savedLoaded {
				count = fmt.Sprint(n)
			}
		}
		counts = append(counts, count)
	}
	sub += theme.Faint.Render(sortLabel(l.sortBy, l.desc))
	rows := []string{title, sub, chipLine(names, counts, sel, w), findBar(l.find, len(m.libVisible()), len(l.entries), w)}

	vis := m.libVisible()
	if l.loaded && len(vis) == 0 {
		if l.find.query != "" {
			return append(rows, theme.Dim.Render(T("find.none")))
		}
		if l.filter == libOnProfile && m.prof.view == nil || savedKind(l.filter) != "" && !l.savedLoaded {
			if l.savedErr != nil && savedKind(l.filter) != "" {
				return append(rows, theme.Error.Render(l.savedErr.Error()))
			}
			return append(rows, theme.Dim.Render(T("lib.loading")))
		}
		return append(rows, theme.Dim.Render(T("lib.f.none")))
	}
	end := min(len(vis), l.scroll+m.listHeight())
	for i := l.scroll; i < end; i++ {
		e := vis[i]
		mark := "  "
		if e.uri != "" && e.uri == playingCtx {
			mark = theme.Playing.Render(theme.Icon.Note + " ")
		}
		name := e.name
		if e.kind == kindLiked {
			name = theme.Icon.Heart + " " + name
		}
		info := F("lib.songs", e.total)
		switch {
		case e.kind == kindFolder:
			name = theme.Icon.Folder + " " + name
			info = folderInfo(e.total)
		case e.kind == kindAlbum || e.kind == kindArtist || e.kind == kindShow:
			info = savedInfo(e)
		case e.kind == kindPlaylist && m.mine(e) && m.onProfile(e):
			info = T("lib.onprofile") + " · " + info // profilde olan zaten herkese açık
		case e.kind == kindPlaylist && e.public && m.mine(e):
			info = T("lib.public") + " · " + info
		case e.kind == kindPlaylist && !m.mine(e) && e.owner != "":
			info = e.owner + " · " + info // takip ettiğin listenin sahibi
		}
		rows = append(rows, mark+row(name, info, w-2, active && i == l.cursor))
	}
	return rows
}

// likedName, beğenilenlerin geçerli dildeki adı.
func (m Model) likedName() string { return T("lib.liked") }
