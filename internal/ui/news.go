package ui

import (
	"sort"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/spotify"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// Yenilikler (ek sayfa, g w): takip ettiğin sanatçıların son 8 haftadaki
// albüm ve single'ları, kitaplığındaki podcast'lerin son 4 haftadaki
// bölümleri. Spotify'ın "Yenilikler" akışı resmî Web API'de yok; sotto onu
// takip ettiklerinden hesaplar (sanatçı başına bir iki istek, 24 saat önbellekte).

const (
	newsReleaseDays = 56
	newsEpisodeDays = 28
	newsMaxArtists  = 200
)

type newsSection int

const (
	newsReleases newsSection = iota
	newsEpisodes
)

type newsEpisode struct {
	ep   spotify.Track
	show spotify.Show
}

type newsState struct {
	loading  bool
	err      error
	section  newsSection
	artists  int // takip edilen sanatçı sayısı
	shows    int
	releases []spotify.ArtistAlbum
	episodes []newsEpisode
}

type newsMsg struct {
	artists, shows int
	releases       []spotify.ArtistAlbum
	episodes       []newsEpisode
	err            error
}

func (m Model) openNews() (tea.Model, tea.Cmd) {
	if m.spState != spReady {
		return m.say(T("flash.noaccount")), nil
	}
	var prev *extraState
	if m.extraOn() {
		p := m.extra
		prev = &p
	}
	m.extra = extraState{page: extraNews, host: m.page, rng: m.extra.rng, prev: prev, news: &newsState{loading: true}}
	m.focus = focusContent
	return m, m.loadNews()
}

func (m Model) loadNews() tea.Cmd {
	sp, rt := m.sp, m.rt
	now := time.Now()
	return func() tea.Msg {
		ctx, cancel := apiCtx(rt, 90*time.Second)
		defer cancel()
		var msg newsMsg
		artists, err := sp.LibraryArtists(ctx, newsMaxArtists)
		if err != nil {
			return newsMsg{err: err}
		}
		msg.artists = len(artists)
		cutoff := now.AddDate(0, 0, -newsReleaseDays).Format("2006-01-02")
		var mu sync.Mutex
		seen := map[string]bool{}
		work := make(chan string)
		var wg sync.WaitGroup
		for w := 0; w < 4; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for id := range work {
					albums, err := sp.ArtistLatest(ctx, id)
					mu.Lock()
					if err != nil && msg.err == nil {
						msg.err = err
					}
					for _, a := range albums {
						if len(a.ReleaseDate) == 10 && a.ReleaseDate >= cutoff && !seen[a.ID] {
							seen[a.ID] = true
							msg.releases = append(msg.releases, a)
						}
					}
					mu.Unlock()
				}
			}()
		}
		for _, a := range artists {
			work <- a.ID
		}
		close(work)
		wg.Wait()
		sort.SliceStable(msg.releases, func(i, j int) bool { return msg.releases[i].ReleaseDate > msg.releases[j].ReleaseDate })

		// Podcast'ler: her birinin son birkaç bölümü.
		ecut := now.AddDate(0, 0, -newsEpisodeDays).Format("2006-01-02")
		for offset := 0; offset < 200; offset += 50 {
			p, err := sp.LibraryShows(ctx, offset, 50)
			if err != nil {
				if msg.err == nil {
					msg.err = err
				}
				break
			}
			for _, s := range p.Items {
				msg.shows++
				eps, err := sp.ShowEpisodes(ctx, s.Show.ID, 0, 5)
				if err != nil {
					continue
				}
				for _, e := range eps.Items {
					if e != nil && e.URI != "" && e.ReleaseDate >= ecut {
						msg.episodes = append(msg.episodes, newsEpisode{ep: *e, show: s.Show})
					}
				}
			}
			if p.Next == "" || len(p.Items) == 0 {
				break
			}
		}
		sort.SliceStable(msg.episodes, func(i, j int) bool { return msg.episodes[i].ep.ReleaseDate > msg.episodes[j].ep.ReleaseDate })
		if len(msg.releases) > 0 || len(msg.episodes) > 0 {
			msg.err = nil // birkaç sanatçının hatası sayfayı boşaltmasın
		}
		return msg
	}
}

func (m Model) updateNews(msg newsMsg) (Model, tea.Cmd) {
	for x := &m.extra; x != nil; x = x.prev {
		if n := x.news; n != nil {
			n.loading, n.err = false, msg.err
			n.artists, n.shows, n.releases, n.episodes = msg.artists, msg.shows, msg.releases, msg.episodes
			if n.section == newsEpisodes && len(n.episodes) == 0 && len(n.releases) > 0 {
				n.section = newsReleases
			}
		}
	}
	if msg.err != nil {
		m = m.spotifyFailed(msg.err)
	}
	m.rebuildExtra()
	return m, nil
}

func (n *newsState) sections() []newsSection {
	if n.shows > 0 {
		return []newsSection{newsReleases, newsEpisodes}
	}
	return []newsSection{newsReleases}
}

func (m Model) switchNewsSection(d int) (Model, tea.Cmd) {
	n := m.extra.news
	secs := n.sections()
	i := 0
	for j, s := range secs {
		if s == n.section {
			i = j
		}
	}
	n.section = secs[(i+d+len(secs))%len(secs)]
	m.extra.cursor, m.extra.scroll = 0, 0
	m.rebuildExtra()
	return m, nil
}

func (m Model) newsItems() []searchItem {
	n := m.extra.news
	if n == nil {
		return nil
	}
	var out []searchItem
	group := ""
	head := func(date string) {
		g := m.newsGroup(date)
		if g != group {
			if len(out) > 0 {
				out = append(out, searchItem{kind: "space"})
			}
			out = append(out, searchItem{kind: "header", name: g})
			group = g
		}
	}
	switch n.section {
	case newsReleases:
		for _, a := range n.releases {
			head(a.ReleaseDate)
			it := albumItem(a.SimpleAlbum, false)
			it.sub = joinDot(artistNames(a.Artists), joinDot(T("info.type."+a.AlbumType), shortDate(a.ReleaseDate)))
			out = append(out, it)
		}
	case newsEpisodes:
		for i := range n.episodes {
			e := n.episodes[i]
			head(e.ep.ReleaseDate)
			ep := e.ep
			ep.Type = "episode"
			sub := joinDot(e.show.Name, shortDate(ep.ReleaseDate))
			if ep.DurationMS > 0 {
				sub = joinDot(sub, longDuration(time.Duration(ep.DurationMS)*time.Millisecond))
			}
			out = append(out, searchItem{kind: "episode", name: ep.Name, sub: sub, uri: ep.URI, track: &ep})
		}
	}
	return out
}

// newsGroup: "Bu hafta", "Geçen hafta", "2 hafta önce"…
func (m Model) newsGroup(date string) string {
	t, err := time.ParseInLocation("2006-01-02", date, time.Local)
	if err != nil {
		return date
	}
	now := m.now
	if now.IsZero() {
		now = time.Now()
	}
	days := int(now.Sub(t).Hours() / 24)
	switch w := days / 7; {
	case w <= 0:
		return T("news.thisweek")
	case w == 1:
		return T("news.lastweek")
	default:
		return F("news.weeksago", w)
	}
}

// shortDate: "2026-11-14" → "14 Kas" / "Nov 14".
func shortDate(s string) string {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return s
	}
	mon := T("mon." + t.Format("1"))
	if langTR() {
		return t.Format("2") + " " + mon
	}
	return mon + " " + t.Format("2")
}

func (m Model) newsHeader(w int) []string {
	n := m.extra.news
	sub := F("news.sub", n.artists, newsReleaseDays/7)
	if n.loading && n.artists == 0 {
		sub = T("news.loading")
	}
	var chips []string
	for _, s := range n.sections() {
		label := T("news.releases")
		count := len(n.releases)
		if s == newsEpisodes {
			label, count = T("news.episodes"), len(n.episodes)
		}
		if !n.loading {
			label += " " + groupDigits(count)
		}
		if s == n.section {
			chips = append(chips, theme.Hi.Underline(true).Render(label))
		} else {
			chips = append(chips, theme.Dim.Render(label))
		}
	}
	return []string{theme.Title.Render(T("news.title")), theme.Dim.Render(truncate(sub, w)), joinChips(chips), ""}
}

func (m Model) newsEmpty() (string, bool) {
	n := m.extra.news
	switch {
	case n.loading:
		return T("news.loading"), false
	case n.err != nil:
		return n.err.Error(), true
	case n.artists == 0 && n.section == newsReleases:
		return T("news.noartists"), false
	}
	return T("news.none"), false
}

func joinChips(chips []string) string {
	out := ""
	for i, c := range chips {
		if i > 0 {
			out += theme.Faint.Render(" · ")
		}
		out += c
	}
	return out
}

// chipLine, süzgeç/bölüm satırı: sığmazsa önce sayılar düşer, yine sığmazsa
// seçilinin çevresi gösterilir (iki yanda "…").
func chipLine(names, counts []string, sel, w int) string {
	render := func(withCounts bool, lo, hi int) string {
		var chips []string
		for i := lo; i < hi; i++ {
			label := names[i]
			if withCounts && counts[i] != "" {
				label += " " + counts[i]
			}
			if i == sel {
				chips = append(chips, theme.Hi.Underline(true).Render(label))
			} else {
				chips = append(chips, theme.Dim.Render(label))
			}
		}
		out := joinChips(chips)
		if lo > 0 {
			out = theme.Faint.Render("… ") + out
		}
		if hi < len(names) {
			out += theme.Faint.Render(" …")
		}
		return out
	}
	if s := render(true, 0, len(names)); textWidth(s) <= w {
		return s
	}
	if s := render(false, 0, len(names)); textWidth(s) <= w {
		return s
	}
	lo, hi := sel, sel+1
	for {
		grew := false
		if hi < len(names) && textWidth(render(false, lo, hi+1)) <= w {
			hi, grew = hi+1, true
		}
		if lo > 0 && textWidth(render(false, lo-1, hi)) <= w {
			lo, grew = lo-1, true
		}
		if !grew {
			return render(false, lo, hi)
		}
	}
}
