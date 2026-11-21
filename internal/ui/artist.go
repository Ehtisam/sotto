package ui

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/i18n"
	"github.com/Ehtisam/sotto/internal/spclient"
	"github.com/Ehtisam/sotto/internal/spotify"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// Sanatçı sayfası (ek sayfa): Popüler · Diskografi · Benzer sanatçılar ·
// Hakkında. Popüler şarkılar, aylık dinleyici, benzer sanatçılar ve
// biyografi iç yoldan (✱); diskografi resmî Web API'den. Ekstralar kapalıyken
// yalnızca Diskografi ve Hakkında (resmî bilgiyle) görünür. Bir sanatçıdan
// ötekine geçilince önceki sayfa yığında kalır; esc geri götürür.

type artistSection int

const (
	artPopular artistSection = iota
	artDisco
	artRelated
	artAbout
)

var artistSectionKeys = map[artistSection]string{
	artPopular: "artist.popular", artDisco: "artist.disco", artRelated: "artist.related", artAbout: "artist.about",
}

type artistState struct {
	uri, id, name string

	view   *spclient.ArtistView // ✱
	bio    string
	info   *spotify.ArtistInfo // resmî bilgi (ekstralar kapalıyken)
	albums []spotify.ArtistAlbum

	viewLoading, albumsLoading bool
	viewErr, albumsErr         error
	albumsAsked                bool

	section artistSection
}

type artistViewMsg struct {
	uri   string
	view  *spclient.ArtistView
	bio   string
	info  *spotify.ArtistInfo
	login string
	err   error
}

type artistAlbumsMsg struct {
	uri    string
	albums []spotify.ArtistAlbum
	err    error
}

// openArtistMsg, herhangi bir yerden sanatçı sayfasını açtırır.
type openArtistMsg struct{ uri, name string }

func openArtistCmd(uri, name string) tea.Cmd {
	return func() tea.Msg { return openArtistMsg{uri, name} }
}

// openArtist, sanatçı sayfasını bulunulan sayfanın üstünde açar. Bir ek sayfa
// zaten açıksa yığına konur (esc ona döner).
func (m Model) openArtist(uri, name string) (Model, tea.Cmd) {
	if m.spState != spReady {
		return m.say(T("flash.noaccount")), nil
	}
	id := spotify.IDFromURI(uri)
	if id == "" || !strings.HasPrefix(uri, "spotify:artist:") {
		return m, nil
	}
	var prev *extraState
	if m.extraOn() {
		p := m.extra
		prev = &p
	}
	a := &artistState{uri: uri, id: id, name: name, viewLoading: true}
	if !m.cfg.Extras.Enabled {
		a.section = artDisco
	}
	m.extra = extraState{page: extraArtist, host: m.page, rng: m.extra.rng, prev: prev, artist: a}
	m.focus = focusContent
	m.rebuildExtra()
	cmds := []tea.Cmd{m.loadArtistView(a)}
	if a.section == artDisco {
		a.albumsAsked, a.albumsLoading = true, true
		cmds = append(cmds, m.loadArtistAlbums(a))
	}
	return m, tea.Batch(cmds...)
}

func (m Model) loadArtistView(a *artistState) tea.Cmd {
	uri, id := a.uri, a.id
	sp, br, extras := m.sp, m.rt.bridge, m.cfg.Extras.Enabled
	rt := m.rt
	return func() tea.Msg {
		ctx, cancel := apiCtx(rt, 30*time.Second)
		defer cancel()
		msg := artistViewMsg{uri: uri}
		if extras {
			login, err := withBridge(ctx, br, func(tok string) error {
				v, err := spclient.Artist(ctx, tok, id)
				if err != nil {
					return err
				}
				msg.view = &v
				msg.bio, _ = spclient.ArtistBio(ctx, tok, id) // biyografi yoksa sayfa yine açılır
				return nil
			})
			msg.login, msg.err = login, err
			if msg.view != nil {
				return msg
			}
		}
		// Ekstralar kapalı ya da iç yol çalışmadı: resmî bilgi.
		info, err := sp.Artist(ctx, id)
		if err == nil {
			msg.info, msg.err = &info, nil
		} else if msg.err == nil {
			msg.err = err
		}
		return msg
	}
}

func (m Model) loadArtistAlbums(a *artistState) tea.Cmd {
	uri, id, sp, rt := a.uri, a.id, m.sp, m.rt
	return func() tea.Msg {
		ctx, cancel := apiCtx(rt, 60*time.Second)
		defer cancel()
		albums, err := sp.ArtistAlbums(ctx, id, 500)
		return artistAlbumsMsg{uri, albums, err}
	}
}

// artistsByURI, yığındaki (açık ve önceki) sanatçı sayfalarından adresi tutanlar.
func (m *Model) artistsByURI(uri string) []*artistState {
	var out []*artistState
	for x := &m.extra; x != nil; x = x.prev {
		if x.artist != nil && x.artist.uri == uri {
			out = append(out, x.artist)
		}
	}
	return out
}

func (m Model) updateArtist(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case openArtistMsg:
		return m.openArtist(msg.uri, msg.name)
	case artistViewMsg:
		for _, a := range m.artistsByURI(msg.uri) {
			a.viewLoading = false
			a.view, a.bio, a.info, a.viewErr = msg.view, msg.bio, msg.info, msg.err
			if a.view != nil && a.view.Name != "" {
				a.name = a.view.Name
			} else if a.info != nil && a.info.Name != "" {
				a.name = a.info.Name
			}
			// İç yol çalışmadıysa yalnızca resmî bölümler kalır.
			if a.view == nil && (a.section == artPopular || a.section == artRelated) {
				a.section = artDisco
			}
		}
		if msg.login != "" && msg.view == nil {
			m = m.note(T("extras.login"))
		}
		m.rebuildExtra()
		var cmd tea.Cmd
		if a := m.extra.artist; m.extraOn() && a != nil && a.uri == msg.uri && a.section == artDisco && !a.albumsAsked {
			a.albumsAsked, a.albumsLoading = true, true
			cmd = m.loadArtistAlbums(a)
		}
		return m, cmd
	case artistAlbumsMsg:
		for _, a := range m.artistsByURI(msg.uri) {
			a.albumsLoading, a.albums, a.albumsErr = false, msg.albums, msg.err
		}
		if msg.err != nil && len(msg.albums) == 0 {
			m = m.spotifyFailed(msg.err)
		}
		m.rebuildExtra()
	}
	return m, nil
}

// artistSections, gösterilecek bölümler.
func (a *artistState) sections() []artistSection {
	if a.view != nil {
		return []artistSection{artPopular, artDisco, artRelated, artAbout}
	}
	return []artistSection{artDisco, artAbout}
}

// switchArtistSection, tab ile bölüm değiştirir; diskografi ilk kez açılınca yüklenir.
func (m Model) switchArtistSection(d int) (Model, tea.Cmd) {
	a := m.extra.artist
	secs := a.sections()
	i := 0
	for j, s := range secs {
		if s == a.section {
			i = j
		}
	}
	a.section = secs[(i+d+len(secs))%len(secs)]
	m.extra.cursor, m.extra.scroll = 0, 0
	m.rebuildExtra()
	if a.section == artDisco && !a.albumsAsked {
		a.albumsAsked, a.albumsLoading = true, true
		return m, m.loadArtistAlbums(a)
	}
	return m, nil
}

// artistItems, bulunulan bölümün satırları.
func (m Model) artistItems() []searchItem {
	a := m.extra.artist
	if a == nil {
		return nil
	}
	var out []searchItem
	section := func(key, name string) {
		if len(out) > 0 {
			out = append(out, searchItem{kind: "space"})
		}
		out = append(out, searchItem{kind: "header", section: key, name: name})
	}
	artistRef := spotify.Artist{ID: a.id, Name: a.name, URI: a.uri}
	switch a.section {
	case artPopular:
		v := a.view
		if v == nil {
			return nil
		}
		out = append(out, searchItem{kind: "artist", name: theme.Icon.Play + " " + F("artist.play", a.name), sub: T("artist.play.sub"), uri: a.uri, act: "play"})
		if len(v.Popular) > 0 {
			section("artist.popular", "")
			var queue []string
			for _, t := range v.Popular {
				queue = append(queue, t.URI)
			}
			for i, t := range v.Popular {
				tr := &spotify.Track{ID: spotify.IDFromURI(t.URI), Name: t.Name, URI: t.URI, Type: "track",
					Artists: []spotify.Artist{artistRef}, Album: spotify.Album{ID: spotify.IDFromURI(t.AlbumURI), URI: t.AlbumURI}}
				if k, ok := m.rt.known[t.URI]; ok && k.Album.URI != "" {
					tr = &k
				}
				out = append(out, searchItem{kind: "track", name: fmt.Sprintf("%2d  %s", i+1, t.Name), sub: t.Plays, uri: t.URI, track: tr, queue: queue})
			}
		}
		if len(v.Releases) > 0 {
			section("artist.releases", "")
			for _, c := range v.Releases {
				out = append(out, cardAlbum(c, artistRef))
			}
		}
		if len(v.Featuring) > 0 {
			section("", F("artist.featuring", a.name))
			for _, c := range v.Featuring {
				out = append(out, searchItem{kind: "playlist", name: c.Name, sub: truncateRunes(c.Sub, 60), uri: c.URI, act: "play",
					entry: libEntry{kind: kindPlaylist, key: spotify.IDFromURI(c.URI), uri: c.URI, name: c.Name, image: c.Image}})
			}
		}
	case artDisco:
		groups := []struct{ key, title string }{
			{"album", "artist.g.album"}, {"single", "artist.g.single"}, {"compilation", "artist.g.compilation"}, {"appears_on", "artist.g.appears"},
		}
		for _, g := range groups {
			first := true
			for _, al := range a.albums {
				if al.Group(a.id) != g.key {
					continue
				}
				if first {
					section(g.title, "")
					first = false
				}
				out = append(out, albumItem(al.SimpleAlbum, g.key == "appears_on"))
			}
		}
		// Diskografi gelmediyse (Web API sınırı) iç yoldaki "yer aldığı" ve popüler çıkanlar.
		if len(a.albums) == 0 && !a.albumsLoading && a.view != nil {
			if len(a.view.Releases) > 0 {
				section("artist.releases", "")
				for _, c := range a.view.Releases {
					out = append(out, cardAlbum(c, artistRef))
				}
			}
			if len(a.view.AppearsOn) > 0 {
				section("artist.g.appears", "")
				for _, c := range a.view.AppearsOn {
					out = append(out, cardAlbum(c, spotify.Artist{}))
				}
			}
		}
	case artRelated:
		if a.view == nil {
			return nil
		}
		for _, c := range a.view.Related {
			out = append(out, searchItem{kind: "artist", name: c.Name, sub: T("kind.artist"), uri: c.URI})
		}
	case artAbout:
		return m.artistAbout(a)
	}
	return out
}

// artistAbout, Hakkında bölümü: sayılar, türler ve biyografi (sarılmış düz
// metin). Fotoğraf gösteriliyorsa sayılar fotoğrafın yanında durur.
func (m Model) artistAbout(a *artistState) []searchItem {
	var out []searchItem
	if url, _, _ := m.extraPhoto(); url == "" {
		out = m.artistFacts(a)
	}
	return append(out, m.artistBio(a, len(out) > 0)...)
}

// artistFacts, aylık dinleyici, onay, takipçi, türler ("ad ···· değer").
func (m Model) artistFacts(a *artistState) []searchItem {
	var out []searchItem
	kv := func(k, v string) {
		if v != "" {
			out = append(out, searchItem{kind: "text", name: T(k), sub: v})
		}
	}
	if v := a.view; v != nil {
		if v.Listeners != "" {
			out = append(out, searchItem{kind: "text", name: v.Listeners})
		}
		if v.Verified {
			out = append(out, searchItem{kind: "text", name: "✓ " + T("artist.verified")})
		}
	}
	if in := a.info; in != nil {
		if in.Followers != nil && in.Followers.Total > 0 {
			kv("artist.followers", groupDigits(in.Followers.Total))
		}
		if len(in.Genres) > 0 {
			kv("artist.genres", strings.Join(in.Genres, ", "))
		}
	}
	return out
}

// artistBio, biyografi satırları. sotto'nun okuyabildiği kaynakta (metadata)
// yalnız bazı sanatçıların biyografisi var; sanatçının kendi yazdığı biyografi
// yalnız web oynatıcının GraphQL'inde. Biyografi gelmediyse hiçbir şey
// yazılmaz ("biyografisi yok" yanlış olurdu). Ekstralar kapalıysa nasıl
// açılacağı söylenir.
func (m Model) artistBio(a *artistState, gap bool) []searchItem {
	w := max(m.contentWidth()-2, 20)
	var body []searchItem
	switch {
	case cleanDesc(a.bio) != "":
		body = append([]searchItem{{kind: "header", section: "artist.bio"}}, paragraphs(stripTags(a.bio), w)...)
	case !m.cfg.Extras.Enabled && !a.viewLoading:
		body = paragraphs(T("artist.nobio.extras"), w)
	}
	if gap && len(body) > 0 {
		body = append([]searchItem{{kind: "space"}}, body...)
	}
	return body
}

// paragraphs, metni w genişliğinde satırlara böler; paragraflar arası boş satır.
func paragraphs(s string, w int) []searchItem {
	var out []searchItem
	for _, para := range strings.Split(strings.ReplaceAll(s, "\r", ""), "\n") {
		para = strings.Join(strings.Fields(para), " ")
		if para == "" {
			continue
		}
		if len(out) > 0 {
			out = append(out, searchItem{kind: "space"})
		}
		for _, l := range wrapText(para, w) {
			out = append(out, searchItem{kind: "text", name: l})
		}
	}
	return out
}

func langTR() bool { return i18n.Lang() == "tr" }

var rankRE = regexp.MustCompile(`^\s*\d+\s+`)

// plainName, satır adındaki sıra numarasını atar (" 1  Athena" → "Athena").
func plainName(s string) string { return rankRE.ReplaceAllString(s, "") }

// cardAlbum, iç yoldan gelen bir albüm kartını açılabilir satıra çevirir.
func cardAlbum(c spclient.Card, artist spotify.Artist) searchItem {
	id := spotify.IDFromURI(c.URI)
	al := spotify.Album{ID: id, Name: c.Name, URI: c.URI}
	if c.Image != "" {
		al.Images = []spotify.Image{{URL: c.Image}}
	}
	owner := ""
	if artist.URI != "" {
		al.Artists = []spotify.Artist{artist}
		owner = artist.Name
	}
	e := libEntry{kind: kindAlbum, key: id, uri: c.URI, name: c.Name, owner: owner, desc: c.Sub, image: c.Image, album: al}
	return searchItem{kind: "album", name: c.Name, sub: c.Sub, uri: c.URI, entry: e}
}

// albumItem, bir albümü (yıl ve şarkı sayısıyla) açılabilir satır yapar.
func albumItem(a spotify.SimpleAlbum, withArtist bool) searchItem {
	year := a.ReleaseDate
	if len(year) > 4 {
		year = year[:4]
	}
	artist := artistNames(a.Artists)
	e := libEntry{kind: kindAlbum, key: a.ID, uri: a.URI, name: a.Name, owner: artist, desc: year,
		image: pickImage(a.Images), total: a.TotalTracks, album: a.Album}
	sub := year
	if a.TotalTracks > 0 {
		sub = joinDot(sub, F("lib.songs", a.TotalTracks))
	}
	if withArtist && artist != "" {
		sub = joinDot(artist, sub)
	}
	return searchItem{kind: "album", name: a.Name, sub: sub, uri: a.URI, entry: e}
}

func joinDot(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + " · " + b
}

// groupDigits: 1234567 → "1.234.567" (Türkçe) ya da "1,234,567".
func groupDigits(n int) string {
	s := fmt.Sprint(n)
	sep := ","
	if langTR() {
		sep = "."
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteString(sep)
		}
		b.WriteRune(c)
	}
	return b.String()
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// stripTags, HTML etiketlerini atar, kaçışları çözer; satır sonlarını korur.
func stripTags(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = cleanDesc(l)
	}
	return strings.Join(lines, "\n")
}

// artistHeader, sayfanın başlığı: ad, ✓, aylık dinleyici, bölümler.
func (m Model) artistHeader(w int) []string {
	a := m.extra.artist
	name := a.name
	if name == "" {
		name = T("kind.artist")
	}
	title := theme.Title.Render(name)
	var sub []string
	if v := a.view; v != nil {
		if v.Verified {
			title += theme.Hi.Render(" ✓")
		}
		if v.Listeners != "" {
			sub = append(sub, v.Listeners)
		}
	}
	if in := a.info; in != nil && in.Followers != nil && in.Followers.Total > 0 {
		sub = append(sub, F("artist.followers.n", groupDigits(in.Followers.Total)))
	}
	if !m.cfg.Extras.Enabled {
		sub = append(sub, T("artist.extras"))
	}
	var chips []string
	for _, s := range a.sections() {
		label := T(artistSectionKeys[s])
		if s == a.section {
			chips = append(chips, theme.Hi.Underline(true).Render(label))
		} else {
			chips = append(chips, theme.Dim.Render(label))
		}
	}
	return []string{title, theme.Dim.Render(truncate(strings.Join(sub, " · "), w)), strings.Join(chips, theme.Faint.Render(" · ")), ""}
}

// artistEmpty, bölümde satır yokken söylenecek.
func (m Model) artistEmpty() (string, bool) {
	a := m.extra.artist
	switch {
	case a.section == artDisco && (a.albumsLoading || !a.albumsAsked):
		return T("lib.loading"), false
	case a.section == artDisco && a.albumsErr != nil:
		return a.albumsErr.Error(), true
	case a.viewLoading && a.section != artDisco:
		return T("lib.loading"), false
	case a.viewErr != nil && a.view == nil && a.info == nil:
		return a.viewErr.Error(), true
	}
	return T("extra.empty"), false
}

// playArtistTrack, popüler şarkılardan birini çalar: sanatçı bağlamı bir
// şarkıdan başlatılamadığı için popüler şarkılar sırayla verilir.
func (m Model) playQueue(queue []string, uri string) tea.Cmd {
	return m.playOn(func(c context.Context, sp *spotify.Client, id string) error {
		return sp.PlayTracks(c, id, queue, uri)
	})
}
