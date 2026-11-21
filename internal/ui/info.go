package ui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/spclient"
	"github.com/Ehtisam/sotto/internal/spotify"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// Ayrıntılar (ek sayfa, i): şarkının künyesi ✱ ve albüm bilgisi, albümün
// çıkış tarihi/plak şirketi/telifleri, podcast ve bölüm ayrıntıları, listenin
// bilgisi. Sanatçıda i sanatçı sayfasının Hakkında bölümünü açar. Hedef,
// eylem menüsününkiyle aynı: seçili satır, yoksa çalan şarkı.

type infoState struct {
	target searchItem
	from   *libEntry

	loading bool
	err     error
	login   bool

	album   *spotify.AlbumDetails
	credits *spclient.Credits
	episode *spotify.Episode
	show    *spotify.ShowDetails
}

type infoMsg struct {
	uri     string
	album   *spotify.AlbumDetails
	credits *spclient.Credits
	episode *spotify.Episode
	show    *spotify.ShowDetails
	login   bool
	err     error
}

// openInfo, seçili öğenin (yoksa çalanın) ayrıntılarını açar.
func (m Model) openInfo() (Model, tea.Cmd) {
	if m.spState != spReady {
		return m.say(T("flash.noaccount")), nil
	}
	t, from, ok := m.actionTarget()
	if !ok {
		return m.note(T("act.nothing")), nil
	}
	return m.openInfoFor(t, from)
}

// openInfoFor, verilen öğenin ayrıntılarını açar.
func (m Model) openInfoFor(t searchItem, from *libEntry) (Model, tea.Cmd) {
	if t.uri == "" && t.kind != "playlist" {
		return m.note(T("act.nothing")), nil
	}
	switch t.kind {
	case "artist":
		mm, cmd := m.openArtist(t.uri, plainName(t.name))
		if a := mm.extra.artist; a != nil && mm.extra.page == extraArtist {
			a.section = artAbout
			mm.rebuildExtra()
		}
		return mm, cmd
	case "liked", kindFolder, "header", "space", "text", "more", "log", "logerr":
		return m.note(T("info.none")), nil
	}
	var prev *extraState
	if m.extraOn() {
		p := m.extra
		prev = &p
	}
	in := &infoState{target: t, from: from}
	m.extra = extraState{page: extraInfo, host: m.page, rng: m.extra.rng, prev: prev, info: in}
	m.focus = focusContent
	cmd := m.loadInfo(in)
	m.rebuildExtra()
	return m, cmd
}

// loadInfo, hedefin türüne göre gereken bilgiyi ister (liste için gerekmez).
func (m Model) loadInfo(in *infoState) tea.Cmd {
	t := in.target
	sp, br, rt, extras := m.sp, m.rt.bridge, m.rt, m.cfg.Extras.Enabled
	albumID := ""
	switch {
	case t.kind == "album":
		albumID = spotify.IDFromURI(t.uri)
	case t.kind == "track" && t.track != nil:
		albumID = t.track.Album.ID
		if albumID == "" {
			albumID = spotify.IDFromURI(t.track.Album.URI)
		}
	case t.kind == "playlist":
		return nil
	}
	in.loading = true
	uri, kind := t.uri, t.kind
	return func() tea.Msg {
		ctx, cancel := apiCtx(rt, 30*time.Second)
		defer cancel()
		msg := infoMsg{uri: uri}
		id := spotify.IDFromURI(uri)
		switch kind {
		case "track":
			if albumID != "" {
				if a, err := sp.AlbumDetail(ctx, albumID); err == nil && a.URI != "" {
					msg.album = &a
				} else {
					msg.err = err
				}
			}
			if extras {
				login, err := withBridge(ctx, br, func(tok string) error {
					c, err := spclient.TrackCredits(ctx, tok, id)
					if err == nil {
						msg.credits = &c
					}
					return err
				})
				msg.login = login != ""
				if err != nil && msg.err == nil {
					msg.err = err
				}
			}
		case "album":
			a, err := sp.AlbumDetail(ctx, albumID)
			if msg.err = err; err == nil {
				msg.album = &a
			}
		case "episode":
			e, err := sp.EpisodeInfo(ctx, id)
			if msg.err = err; err == nil {
				msg.episode = &e
			}
		case "show":
			s, err := sp.ShowInfo(ctx, id)
			if msg.err = err; err == nil {
				msg.show = &s
			}
		}
		return msg
	}
}

func (m Model) updateInfo(msg infoMsg) (Model, tea.Cmd) {
	for x := &m.extra; x != nil; x = x.prev {
		in := x.info
		if in == nil || in.target.uri != msg.uri {
			continue
		}
		in.loading, in.err, in.login = false, msg.err, msg.login
		in.album, in.credits, in.episode, in.show = msg.album, msg.credits, msg.episode, msg.show
	}
	m.rebuildExtra()
	return m, nil
}

// infoItems, ayrıntı satırları: açılabilenler (sanatçı, albüm, podcast)
// seçilebilir, bilgiler "ad ···· değer" yazısı.
func (m Model) infoItems() []searchItem {
	in := m.extra.info
	if in == nil {
		return nil
	}
	var out []searchItem
	w := max(m.contentWidth()-2, 20)
	kv := func(k, v string) {
		if strings.TrimSpace(v) != "" {
			out = append(out, searchItem{kind: "text", name: T(k), sub: v})
		}
	}
	section := func(key string) {
		if len(out) > 0 {
			out = append(out, searchItem{kind: "space"})
		}
		out = append(out, searchItem{kind: "header", section: key})
	}
	artistRows := func(as []spotify.Artist) {
		for _, a := range as {
			if a.URI != "" {
				out = append(out, searchItem{kind: "artist", name: a.Name, sub: T("kind.artist"), uri: a.URI})
			}
		}
	}
	copyrights := func(a *spotify.AlbumDetails) {
		if a == nil || len(a.Copyrights) == 0 {
			return
		}
		section("info.copyright")
		seen := map[string]bool{}
		for _, c := range a.Copyrights {
			text := strings.TrimSpace(c.Text)
			if text == "" || seen[text] {
				continue
			}
			seen[text] = true
			if c.Type == "P" && !strings.HasPrefix(text, "℗") && !strings.HasPrefix(text, "(P)") {
				text = "℗ " + text
			} else if c.Type == "C" && !strings.HasPrefix(text, "©") && !strings.HasPrefix(text, "(C)") {
				text = "© " + text
			}
			for _, l := range wrapText(text, w) {
				out = append(out, searchItem{kind: "text", name: l})
			}
		}
	}
	t := in.target
	switch t.kind {
	case "track":
		tr := t.track
		if tr == nil {
			tr = &spotify.Track{URI: t.uri, Name: t.name}
		}
		artistRows(tr.Artists)
		al := tr.Album
		if in.album != nil {
			al = in.album.Album
		}
		if al.URI != "" {
			e := libEntry{kind: kindAlbum, key: al.ID, uri: al.URI, name: al.Name, owner: artistNames(al.Artists), image: pickImage(al.Images), album: al}
			if in.album != nil {
				e.total = in.album.TotalTracks
			}
			name := al.Name
			if name == "" {
				name = T("kind.album")
			}
			out = append(out, searchItem{kind: "album", name: name, sub: T("kind.album"), uri: al.URI, entry: e})
		}
		if tr.DurationMS > 0 {
			kv("info.duration", clock(time.Duration(tr.DurationMS)*time.Millisecond))
		}
		kv("info.released", niceDate(al.ReleaseDate))
		if in.album != nil {
			kv("info.label", in.album.Label)
		}
		if tr.Explicit {
			kv("info.explicit", T("yes"))
		}
		if c := in.credits; c != nil {
			section("info.credits")
			for _, r := range c.Roles {
				if len(r.Artists) == 0 {
					continue
				}
				out = append(out, searchItem{kind: "text", name: r.Title})
				for _, p := range r.Artists {
					sub := strings.Join(p.Subroles, ", ")
					if p.URI != "" {
						out = append(out, searchItem{kind: "artist", name: "  " + p.Name, sub: sub, uri: p.URI})
					} else {
						out = append(out, searchItem{kind: "text", name: "  " + p.Name, sub: sub})
					}
				}
			}
			if len(c.Sources) > 0 {
				kv("info.source", strings.Join(c.Sources, ", "))
			}
		} else if !m.cfg.Extras.Enabled {
			section("info.credits")
			out = append(out, paragraphs(T("info.credits.extras"), w)...)
		}
		copyrights(in.album)
	case "album":
		a := in.album
		if a == nil {
			artistRows(t.entry.album.Artists)
			break
		}
		artistRows(a.Artists)
		kv("info.type", T("info.type."+a.AlbumType))
		kv("info.released", niceDate(a.ReleaseDate))
		if a.TotalTracks > 0 {
			kv("info.tracks", F("lib.songs", a.TotalTracks))
		}
		if a.Tracks.Total > 0 && len(a.Tracks.Items) >= a.Tracks.Total {
			total := 0
			for _, tr := range a.Tracks.Items {
				total += tr.DurationMS
			}
			kv("info.duration", longDuration(time.Duration(total)*time.Millisecond))
		}
		kv("info.label", a.Label)
		copyrights(a)
	case "show":
		s := in.show
		if s == nil {
			break
		}
		e := libEntry{kind: kindShow, key: s.ID, uri: s.URI, name: s.Name, owner: F("search.episodecount", s.TotalEpisodes),
			desc: cleanDesc(s.Description), image: pickImage(s.Images), total: s.TotalEpisodes}
		out = append(out, searchItem{kind: "show", name: T("info.episodes.open"), sub: F("search.episodecount", s.TotalEpisodes), uri: s.URI, entry: e})
		kv("info.languages", strings.Join(s.Languages, ", "))
		if s.Explicit {
			kv("info.explicit", T("yes"))
		}
		if d := cleanDesc(s.Description); d != "" {
			section("info.about")
			out = append(out, paragraphs(d, w)...)
		}
	case "episode":
		e := in.episode
		if e == nil {
			break
		}
		if e.Show.URI != "" {
			se := libEntry{kind: kindShow, key: e.Show.ID, uri: e.Show.URI, name: e.Show.Name, image: pickImage(e.Show.Images),
				owner: F("search.episodecount", e.Show.TotalEpisodes), total: e.Show.TotalEpisodes}
			out = append(out, searchItem{kind: "show", name: e.Show.Name, sub: T("kind.show"), uri: e.Show.URI, entry: se})
		}
		kv("info.released", niceDate(e.ReleaseDate))
		kv("info.duration", clock(time.Duration(e.DurationMS)*time.Millisecond))
		if r := e.ResumePoint; r != nil {
			switch {
			case r.FullyPlayed:
				kv("info.resume", T("info.played"))
			case r.ResumePositionMS > 0:
				kv("info.resume", clock(time.Duration(r.ResumePositionMS)*time.Millisecond))
			}
		}
		kv("info.languages", strings.Join(e.Languages, ", "))
		if d := cleanDesc(e.Description); d != "" {
			section("info.about")
			out = append(out, paragraphs(d, w)...)
		}
	case "playlist":
		le := t.entry
		out = append(out, searchItem{kind: "playlist", name: T("info.list.open"), sub: F("lib.songs", le.total), uri: le.uri, entry: le})
		kv("info.owner", le.owner)
		switch {
		case le.collaborative:
			kv("info.visibility", T("pl.vis.collab"))
		case le.public:
			kv("info.visibility", T("pl.vis.public"))
		case m.mine(le):
			kv("info.visibility", T("pl.vis.private"))
		}
		if tl := m.shownList(); tl != nil && tl.entry.key == le.key && len(tl.tracks) > 0 && len(tl.tracks) >= tl.total {
			total := 0
			for _, tr := range tl.tracks {
				total += tr.DurationMS
			}
			kv("info.duration", longDuration(time.Duration(total)*time.Millisecond))
		}
		if le.desc != "" {
			section("info.about")
			out = append(out, paragraphs(le.desc, w)...)
		}
	}
	return out
}

func (m Model) infoHeader(w int) []string {
	in := m.extra.info
	t := in.target
	name := t.name
	if t.track != nil && t.track.Name != "" {
		name = t.track.Name
	}
	if in.album != nil && t.kind == "album" {
		name = in.album.Name
	}
	if in.show != nil {
		name = in.show.Name
	}
	if in.episode != nil {
		name = in.episode.Name
	}
	kind := T("kind." + t.kind)
	if t.kind == "track" && t.track != nil && t.track.Type == "episode" {
		kind = T("kind.episode")
	}
	sub := T("info.title") + " · " + kind
	if t.kind == "track" && t.track != nil {
		sub += " · " + t.track.ArtistNames()
	}
	return []string{theme.Title.Render(truncate(plainName(name), w)), theme.Dim.Render(truncate(sub, w)), "", ""}
}

func (m Model) infoEmpty() (string, bool) {
	in := m.extra.info
	switch {
	case in.loading:
		return T("lib.loading"), false
	case in.err != nil:
		return in.err.Error(), true
	}
	return T("info.none"), false
}

// niceDate: "2006-01-02" → "2 Ocak 2006" (dile göre); yalnız yıl ya da ay da olur.
func niceDate(s string) string {
	for _, layout := range []string{"2006-01-02", "2006-01", "2006"} {
		t, err := time.Parse(layout, s)
		if err != nil {
			continue
		}
		month := T("month." + t.Format("1"))
		switch layout {
		case "2006-01-02":
			if langTR() {
				return t.Format("2") + " " + month + " " + t.Format("2006")
			}
			return month + " " + t.Format("2") + ", " + t.Format("2006")
		case "2006-01":
			return month + " " + t.Format("2006")
		}
		return s
	}
	return s
}

// longDuration: "1 sa 12 dk" ya da "42 dk".
func longDuration(d time.Duration) string {
	h, mi := int(d.Hours()), int(d.Minutes())%60
	if h > 0 {
		return F("dur.hm", h, mi)
	}
	return F("dur.m", max(mi, 1))
}
