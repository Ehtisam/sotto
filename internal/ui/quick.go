package ui

import (
	"context"
	"math/rand/v2"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/cli"
	"github.com/Ehtisam/sotto/internal/desktop"
	"github.com/Ehtisam/sotto/internal/soloist"
	"github.com/Ehtisam/sotto/internal/spotify"
)

// Tek tuşla yapılanlar: sessize al (m), başa sar (0), karıştırarak çal
// (shift+S), beğen / beğenmekten vazgeç (h), sıraya ekle (u), panodaki
// Spotify bağlantısını aç (v; yapıştırınca da), çalanı listede bul (c).

// ---- sessize alma ----

// toggleMute: ses açıksa 0'a iner, kapalıysa eski sesine döner. Eski ses
// komut satırıyla ortak dosyada (cli.SaveMuted) tutulur.
func (m Model) toggleMute(want string) (Model, tea.Cmd) {
	if c := m.refused("volume"); c != nil {
		return m, c
	}
	v, save, muted := cli.MuteTarget(m.state.Volume, cli.MutedVolume(m.cfg.LocalDir), want)
	cli.SaveMuted(m.cfg.LocalDir, save)
	if muted {
		m = m.note(T("mute.on"))
	} else {
		m = m.note(F("mute.off", v))
	}
	if v == m.state.Volume {
		return m, nil
	}
	return m, m.setVolume(v)
}

// muted: ses sessize alma tuşuyla kısıldı (eski ses saklı).
func (m Model) muted() bool {
	return m.state.Volume == 0 && cli.MutedVolume(m.cfg.LocalDir) > 0
}

// ---- seçili öğe ----

// selectedItem, içerikte seçili satır (yoksa ok=false; çalan şarkıya düşmez).
func (m Model) selectedItem() (searchItem, *libEntry, bool) {
	if m.focus != focusContent {
		return searchItem{}, nil, false
	}
	it, from, ok := m.actionTarget()
	if !ok {
		return it, from, false
	}
	if cur := m.state.Item; cur != nil && it.uri == cur.URI && from == nil && m.page == pageNow {
		return searchItem{}, nil, false
	}
	return it, from, it.kind != "header" && it.kind != "space"
}

// ---- beğen (h) ----

// toggleLike, seçili öğeyi (yoksa çalan şarkıyı) kitaplığa ekler ya da
// çıkarır: şarkı beğenilenlere, albüm ve podcast kitaplığa, liste ve sanatçı
// takibe.
func (m Model) toggleLike() (Model, tea.Cmd) {
	if m.spState != spReady {
		return m.say(T("flash.noaccount")), nil
	}
	it, _, ok := m.actionTarget()
	if !ok || it.uri == "" {
		return m.note(T("act.nothing")), nil
	}
	switch it.kind {
	case "liked", kindFolder:
		return m.note(T("like.cannot")), nil
	case "playlist":
		if m.ownList(it.entry) {
			return m.note(T("like.own")), nil
		}
	}
	sp, uri, kind, name := m.sp, it.uri, it.kind, it.name
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		has, err := sp.InLibrary(ctx, []string{uri})
		if err != nil || len(has) == 0 {
			return actionDoneMsg{err: err}
		}
		if has[0] {
			return actionDoneMsg{note: likeNote(kind, false, name), err: sp.Unsave(ctx, []string{uri})}
		}
		return actionDoneMsg{note: likeNote(kind, true, name), err: sp.Save(ctx, []string{uri})}
	}
}

func likeNote(kind string, on bool, name string) string {
	key := "act.unliked"
	switch {
	case kind == "track" || kind == "episode":
		if on {
			key = "act.liked"
		}
	case kind == "playlist" || kind == "artist":
		key = "act.unfollowed"
		if on {
			key = "act.followed"
		}
	default:
		key = "act.unsaved"
		if on {
			key = "act.saved"
		}
	}
	return T(key) + ": " + name
}

// ---- sıraya ekle (u) ----

// maxQueueAdd: bir albüm ya da listeden sıraya en çok bu kadar şarkı eklenir.
const maxQueueAdd = 50

// queueSelected, seçili şarkıyı (ya da albümün, listenin şarkılarını) sıraya ekler.
func (m Model) queueSelected() (Model, tea.Cmd) {
	it, _, ok := m.selectedItem()
	if !ok {
		return m.note(T("queue.select")), nil
	}
	return m.queueItem(it)
}

func (m Model) queueItem(it searchItem) (Model, tea.Cmd) {
	switch it.kind {
	case "track", "episode":
		uri := it.uri
		if m.soloistActive() && it.kind == "track" {
			return m, m.soloistAction(func(ctx context.Context, c *soloist.Client) error { return c.AddToQueue(ctx, uri) }, T("act.queued")+": "+it.name)
		}
		return m, m.spotifyAction(func(ctx context.Context, sp *spotify.Client) error { return sp.AddToQueue(ctx, uri) }, T("act.queued")+": "+it.name)
	case "album", "playlist", "liked", "show":
	default:
		return m.note(T("queue.only")), nil
	}
	if m.spState != spReady {
		return m.say(T("flash.noaccount")), nil
	}
	e := it.entry
	var have []string // açık listede yüklü şarkılar
	if tl := m.shownList(); tl != nil && tl.entry.key == e.key {
		for _, t := range tl.tracks {
			have = append(have, t.URI)
		}
	}
	sp, client, solo := m.sp, m.rt.client, m.soloistActive() && e.kind != kindShow
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		uris := have
		if len(uris) == 0 {
			var err error
			if uris, err = entryURIs(ctx, sp, e, maxQueueAdd); err != nil {
				return actionDoneMsg{err: err}
			}
		}
		if len(uris) > maxQueueAdd {
			uris = uris[:maxQueueAdd]
		}
		for i, u := range uris {
			var err error
			if solo {
				err = client.AddToQueue(ctx, u)
			} else {
				err = sp.AddToQueue(ctx, u)
			}
			if err != nil {
				if i == 0 {
					return actionDoneMsg{err: err}
				}
				return actionDoneMsg{note: F("queue.added.some", i, len(uris))}
			}
		}
		return actionDoneMsg{note: F("queue.added", len(uris), e.name)}
	}
}

// entryURIs, bir topluluğun ilk n şarkısının adresi.
func entryURIs(ctx context.Context, sp *spotify.Client, e libEntry, n int) ([]string, error) {
	var out []string
	for offset := 0; len(out) < n; offset += pageSize {
		var got []string
		total := 0
		switch e.kind {
		case kindLiked:
			p, err := sp.LikedTracks(ctx, offset, pageSize)
			if err != nil {
				return out, err
			}
			total = p.Total
			for _, it := range p.Items {
				got = append(got, it.Track.URI)
			}
		case kindAlbum:
			p, err := sp.AlbumTracks(ctx, e.album, offset, pageSize)
			if err != nil {
				return out, err
			}
			total = p.Total
			for _, t := range p.Items {
				got = append(got, t.URI)
			}
		case kindShow:
			p, err := sp.ShowEpisodes(ctx, e.key, offset, pageSize)
			if err != nil {
				return out, err
			}
			total = p.Total
			for _, t := range p.Items {
				if t != nil {
					got = append(got, t.URI)
				}
			}
		default:
			p, err := sp.PlaylistItems(ctx, e.key, offset, pageSize)
			if err != nil {
				return out, err
			}
			total = p.Total
			for _, it := range p.Items {
				if t := it.T(); t != nil && t.URI != "" && !t.IsLocal {
					got = append(got, t.URI)
				}
			}
		}
		out = append(out, got...)
		if len(got) == 0 || offset+pageSize >= total {
			break
		}
	}
	if len(out) > n {
		out = out[:n]
	}
	return out, nil
}

// ---- karıştırarak çal (shift+S) ----

// shufflePlay, seçili listeyi, albümü ya da sanatçıyı (şarkı seçiliyse
// içinde bulunduğu listeyi; seçili bir şey yoksa çalan bağlamı) karıştırma
// açık olarak rastgele bir şarkıdan başlatır.
func (m Model) shufflePlay() (Model, tea.Cmd) {
	if m.spState != spReady {
		return m.say(T("flash.noaccount")), nil
	}
	it, from, ok := m.selectedItem()
	if !ok {
		it, from = searchItem{}, nil
	}
	return m.shuffleTarget(it, from)
}

// shuffleTarget, karıştırarak çalınacak topluluğu seçer ve başlatır.
func (m Model) shuffleTarget(it searchItem, from *libEntry) (Model, tea.Cmd) {
	var e libEntry
	ok := it.uri != "" || it.kind == "liked"
	switch {
	case ok && from != nil:
		e = *from
	case ok && (it.kind == "album" || it.kind == "playlist" || it.kind == "liked" || it.kind == "show"):
		e = it.entry
	case ok && it.kind == "artist":
		e = libEntry{kind: "artist", uri: it.uri, name: it.name}
	case ok && it.kind == "track" && it.track != nil && it.track.Album.URI != "":
		a := it.track.Album
		e = libEntry{kind: kindAlbum, key: a.ID, uri: a.URI, name: a.Name, album: a}
	case m.state.Context != nil && m.state.Context.URI != "":
		c := m.state.Context
		e = libEntry{kind: c.EntityType, uri: c.URI, name: c.Name()}
		if strings.HasSuffix(c.URI, ":collection") {
			e = m.likedEntry()
		}
	default:
		return m.note(T("shuffle.select")), nil
	}
	if e.kind == kindShow {
		return m.note(T("shuffle.noshow")), nil
	}
	var have []string
	if tl := m.shownList(); tl != nil && tl.entry.key == e.key && e.kind == kindLiked {
		for _, t := range tl.tracks {
			have = append(have, t.URI)
		}
	}
	total := e.total
	play := m.playOn(func(ctx context.Context, sp *spotify.Client, id string) error {
		if e.kind == kindLiked {
			uris := have
			if len(uris) == 0 {
				var err error
				if uris, err = entryURIs(ctx, sp, e, 100); err != nil {
					return err
				}
			}
			if len(uris) == 0 {
				return nil
			}
			return sp.PlayTracks(ctx, id, uris, uris[rand.IntN(len(uris))])
		}
		pos := 0
		if total > 1 && e.kind != "artist" {
			pos = rand.IntN(total)
		}
		return sp.PlayContextAt(ctx, id, e.uri, pos)
	})
	m = m.note(T("shuffle.playing") + ": " + e.name)
	if m.state.Options.Shuffle {
		return m, play
	}
	return m, tea.Sequence(m.setShuffle(true), play)
}

// likedEntry, kitaplıktaki "Beğenilenler" girişi.
func (m Model) likedEntry() libEntry {
	for _, e := range m.lib.entries {
		if e.kind == kindLiked {
			return e
		}
	}
	return libEntry{kind: kindLiked, key: likedKey, name: T("lib.liked")}
}

// ---- bağlantı aç (v, yapıştırma) ----

// playURI, bir adresi çalar: Soloist etkinse ona, değilse etkin cihaza.
func (m Model) playURI(uri string) tea.Cmd {
	return m.control(
		func(ctx context.Context, c *soloist.Client) error { return c.Play(ctx, uri) },
		func(ctx context.Context, s *spotify.Client) error { return cli.PlayURI(ctx, s, "", uri) })
}

// readClipboard, işletim sisteminin panosundaki metni okur (testler değiştirir).
var readClipboard = desktop.ReadClipboard

type clipboardMsg struct {
	text string
	err  error
}

func (m Model) openClipboard() (Model, tea.Cmd) {
	return m, func() tea.Msg {
		s, err := readClipboard()
		return clipboardMsg{s, err}
	}
}

// spotifyLink, metindeki ilk Spotify bağlantısı ya da adresi ("" yoksa).
func spotifyLink(text string) string {
	for _, f := range strings.Fields(text) {
		f = strings.Trim(f, "<>\"'()[],")
		if !strings.Contains(f, "spotify") {
			continue
		}
		if uri, err := cli.ToURI(f); err == nil {
			return uri
		}
	}
	return ""
}

// openLink, bir Spotify adresini açar: şarkı ve bölüm çalınır; albüm, liste
// ve podcast açılır; sanatçı çalınır.
func (m Model) openLink(text string) (Model, tea.Cmd) {
	uri := spotifyLink(text)
	if uri == "" {
		return m.note(T("link.none")), nil
	}
	if m.spState != spReady {
		return m.say(T("flash.noaccount")), nil
	}
	switch cli.Kind(uri) {
	case "track", "episode", "artist":
		m = m.note(T("link.playing"))
		return m, m.playURI(uri)
	}
	sp := m.sp
	return m.note(T("link.opening")), func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		e, err := entryFor(ctx, sp, uri)
		return linkEntryMsg{e, err}
	}
}

type linkEntryMsg struct {
	entry libEntry
	err   error
}

// entryFor, bir albüm, liste ya da podcast adresinin açılabilir girişi.
func entryFor(ctx context.Context, sp *spotify.Client, uri string) (libEntry, error) {
	id := spotify.IDFromURI(uri)
	switch cli.Kind(uri) {
	case "album":
		a, total, err := sp.AlbumInfo(ctx, id)
		if err != nil {
			return libEntry{}, err
		}
		return libEntry{kind: kindAlbum, key: a.ID, uri: a.URI, name: a.Name, owner: artistNames(a.Artists),
			image: pickImage(a.Images), total: total, album: a}, nil
	case "show":
		s, err := sp.Show(ctx, id)
		if err != nil {
			return libEntry{}, err
		}
		return libEntry{kind: kindShow, key: s.ID, uri: s.URI, name: s.Name, desc: cleanDesc(s.Description),
			image: pickImage(s.Images), total: s.TotalEpisodes}, nil
	}
	pl, err := sp.Playlist(ctx, id)
	if err != nil {
		return libEntry{}, err
	}
	return playlistEntry(pl), nil
}

func (m Model) updateQuick(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case clipboardMsg:
		if msg.err != nil {
			return m.say(msg.err.Error()), nil
		}
		return m.openLink(msg.text)
	case linkEntryMsg:
		if msg.err != nil {
			return m.spotifyFailed(msg.err), nil
		}
		m.flash = ""
		if m.page == pageNow || m.page == pageQueue || m.page == pageDevices || m.page == pageSettings || m.page == pageProfile {
			m.page = pageSearch
		}
		return m.goTo(msg.entry)
	}
	return m, nil
}

// ---- çalanı bul (c) ----

// findPlaying, çalan şarkıyı gösterir: açık listede varsa imleci üstüne
// götürür (gerekirse sonraki sayfaları yükleyerek), yoksa çalan bağlamı
// (liste, albüm, beğenilenler) açar ve şarkıyı orada seçer.
func (m Model) findPlaying() (Model, tea.Cmd) {
	cur := m.state.Item
	if cur == nil || cur.URI == "" {
		return m.note(T("sleep.nothing")), nil
	}
	uri := cur.URI
	ctxURI := m.state.Context.URIOrEmpty()

	// Açık liste: şarkı orada mı?
	if tl := m.shownList(); tl != nil {
		if i := indexOfTrack(tl.tracks, uri); i >= 0 {
			tl.cursor = i
			tl.clamp(m.listHeight())
			m.focus = focusContent
			return m, nil
		}
		if tl.entry.uri != "" && tl.entry.uri == ctxURI && len(tl.tracks) < tl.total {
			m.focus = focusContent
			return m, m.seekTrack(tl, uri)
		}
	}
	// Sıra sayfası: çalan şarkı en üstte.
	if m.page == pageQueue {
		m.qs.cursor, m.qs.scroll, m.focus = 0, 0, focusContent
		return m, nil
	}
	if m.spState != spReady {
		return m.say(T("flash.noaccount")), nil
	}

	// Bağlamı aç.
	var e libEntry
	found := false
	switch {
	case strings.HasSuffix(ctxURI, ":collection"):
		e, found = m.likedEntry(), true
	case strings.HasPrefix(ctxURI, "spotify:playlist:"):
		for _, le := range m.lib.entries {
			if le.uri == ctxURI {
				e, found = le, true
			}
		}
		if !found {
			m.page, m.focus = pageSearch, focusContent
			return m.note(T("link.opening")), m.openForSeek(ctxURI, uri)
		}
	}
	if !found {
		// Albüm, sanatçı ya da bağlam yok: şarkının albümü.
		t := m.trackFromEntity(cur)
		if t.Album.URI == "" {
			if strings.HasPrefix(ctxURI, "spotify:album:") || strings.HasPrefix(ctxURI, "spotify:show:") {
				m.page, m.focus = pageSearch, focusContent
				return m.note(T("link.opening")), m.openForSeek(ctxURI, uri)
			}
			return m.note(T("find.nolist")), nil
		}
		a := t.Album
		e = libEntry{kind: kindAlbum, key: a.ID, uri: a.URI, name: a.Name, owner: artistNames(a.Artists),
			image: pickImage(a.Images), album: a}
	}
	if e.kind == kindPlaylist || e.kind == kindLiked {
		m.page, m.focus, m.lib.filter = pageLibrary, focusContent, libAll
	} else if m.page != pageLibrary && m.page != pageHome {
		m.page, m.focus = pageSearch, focusContent
	}
	tl, cmd := m.openList(e)
	tl.seek = uri
	return m.showList(tl), cmd
}

// openForSeek: adresin girişini Spotify'dan alıp açar ve şarkıyı arar.
func (m Model) openForSeek(ctxURI, uri string) tea.Cmd {
	sp := m.sp
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		e, err := entryFor(ctx, sp, ctxURI)
		return seekEntryMsg{e, uri, err}
	}
}

type seekEntryMsg struct {
	entry libEntry
	uri   string
	err   error
}

func (m Model) updateSeekEntry(msg seekEntryMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		return m.spotifyFailed(msg.err), nil
	}
	m.flash = ""
	tl, cmd := m.openList(msg.entry)
	tl.seek = msg.uri
	return m.showList(tl), cmd
}

func indexOfTrack(ts []spotify.Track, uri string) int {
	for i, t := range ts {
		if t.URI == uri {
			return i
		}
	}
	return -1
}

// seekTrack, listenin sonraki sayfalarını şarkı bulunana kadar yükler.
func (m Model) seekTrack(tl *trackList, uri string) tea.Cmd {
	tl.seek = uri
	if tl.loading {
		return nil
	}
	tl.loading = true
	return m.loadTracks(tl.entry, len(tl.tracks))
}

// continueSeek, yeni sayfa gelince aranan şarkıya bakar; yoksa sonrakini ister.
func (m Model) continueSeek(tl *trackList) tea.Cmd {
	if tl == nil || tl.seek == "" || tl.err != nil {
		return nil
	}
	if i := indexOfTrack(tl.tracks, tl.seek); i >= 0 {
		tl.seek = ""
		tl.cursor = i
		tl.clamp(m.listHeight())
		return nil
	}
	if len(tl.tracks) >= tl.total || tl.loading {
		if !tl.loading {
			tl.seek = ""
		}
		return nil
	}
	tl.loading = true
	return m.loadTracks(tl.entry, len(tl.tracks))
}
