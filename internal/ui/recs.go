package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/spclient"
	"github.com/Ehtisam/sotto/internal/spotify"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// Önerilen şarkılar (✱): kendi listenin sonunda, Spotify masaüstündeki gibi.
// Liste tohum olur (spclient.Recommendations: listenin radyosu); listede
// olanlar ve daha önce gösterilenler çıkar, ilk recsCount şarkı gösterilir.
// Adları iç uç vermiyor, Web API'den (önbellekli) tamamlanır. İstek yalnız
// listenin sonu göründüğünde gider. Son şarkıda ↓ önerilere iner; enter
// çalar, a listeye ekler, R yenilerini getirir, ↑ ilk öneride listeye döner.

const recsCount = 10

type recsState int

const (
	recsNone recsState = iota
	recsLoading
	recsReady
	recsFailed
)

type listRecs struct {
	state  recsState
	tracks []spotify.Track
	shown  map[string]bool // daha önce gösterilenler (R yenilerini getirsin)
	in     bool            // imleç önerilerde
	cursor int
	adding bool
}

type recsMsg struct {
	key    string
	tracks []spotify.Track
	login  string
	err    error
}

type recAddedMsg struct {
	key  string
	t    spotify.Track
	list string
	err  error
}

// recsWanted: bu listenin altında öneri olur mu.
func (m Model) recsWanted(tl *trackList) bool {
	return tl != nil && m.cfg.Extras.Enabled && m.spState == spReady &&
		tl.entry.kind == kindPlaylist && m.editable(tl.entry) && tl.entry.key != "" &&
		tl.find.query == "" && !tl.loading && len(tl.tracks) >= tl.total
}

// recsAtEnd: listenin sonu ekranda mı (öneriler orada başlar).
func (m Model) recsAtEnd(tl *trackList) bool {
	n := tl.rows()
	return n == 0 || tl.scroll+m.listHeight() >= n || max(tl.pos(), 0) >= n-1
}

// maybeRecs, liste sonu göründüyse ve henüz istenmediyse önerileri ister.
func (m Model) maybeRecs(tl *trackList) tea.Cmd {
	if !m.recsWanted(tl) || tl.recs.state != recsNone || !m.recsAtEnd(tl) {
		return nil
	}
	return m.fetchRecs(tl)
}

func (m Model) fetchRecs(tl *trackList) tea.Cmd {
	tl.recs.state = recsLoading
	if tl.recs.shown == nil {
		tl.recs.shown = map[string]bool{}
	}
	skip := map[string]bool{}
	for _, t := range tl.tracks {
		skip[t.URI] = true
	}
	for u := range tl.recs.shown {
		skip[u] = true
	}
	br, sp, key, seed := m.rt.bridge, m.sp, tl.entry.key, tl.entry.uri
	if seed == "" {
		seed = "spotify:playlist:" + key
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		msg := recsMsg{key: key}
		var uris []string
		msg.login, msg.err = withBridge(ctx, br, func(tok string) error {
			var err error
			uris, err = spclient.Recommendations(ctx, tok, seed)
			return err
		})
		if msg.login != "" || msg.err != nil {
			return msg
		}
		for _, u := range uris {
			if len(msg.tracks) >= recsCount {
				break
			}
			if skip[u] || spotify.IDFromURI(u) == "" || len(u) < 15 || u[:14] != "spotify:track:" {
				continue
			}
			skip[u] = true
			if t, err := sp.Track(ctx, spotify.IDFromURI(u)); err == nil {
				msg.tracks = append(msg.tracks, t)
			} else if spotify.IsStatus(err, 429) {
				msg.err = err
				break
			}
		}
		return msg
	}
}

func (m Model) updateRecs(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case recsMsg:
		for _, tl := range m.openLists() {
			if tl == nil || tl.entry.key != msg.key || tl.recs.state != recsLoading {
				continue
			}
			if msg.login != "" || msg.err != nil || len(msg.tracks) == 0 {
				tl.recs.state, tl.recs.tracks, tl.recs.in = recsFailed, nil, false
				continue
			}
			tl.recs.state, tl.recs.tracks = recsReady, msg.tracks
			tl.recs.cursor = min(tl.recs.cursor, len(msg.tracks)-1)
			for _, t := range msg.tracks {
				tl.recs.shown[t.URI] = true
			}
		}
		for _, t := range msg.tracks {
			m.rt.known[t.URI] = t
		}
	case recAddedMsg:
		for _, tl := range m.openLists() {
			if tl == nil || tl.entry.key != msg.key {
				continue
			}
			tl.recs.adding = false
			if msg.err == nil {
				tl.tracks = append(tl.tracks, msg.t)
				tl.total++
				tl.changed()
			}
		}
		if msg.err != nil {
			return m.say(msg.err.Error()), nil
		}
		return m.note(F("act.added", msg.list)), nil
	}
	return m, nil
}

// recsKey: imleç önerilerdeyken tuşlar (listKey'den önce). ok=false: listeye düşer.
func (m Model) recsKey(tl *trackList, k string) (cmd tea.Cmd, ok bool) {
	r := &tl.recs
	if !r.in {
		// Son şarkıda ↓: önerilere in.
		if (k == "down" || k == "j") && r.state == recsReady && len(r.tracks) > 0 &&
			m.recsWanted(tl) && tl.rows() > 0 && tl.pos() == tl.rows()-1 {
			r.in, r.cursor = true, 0
			return nil, true
		}
		return nil, false
	}
	if r.state != recsReady || len(r.tracks) == 0 {
		r.in = false
		return nil, false
	}
	switch k {
	case "up", "k":
		if r.cursor == 0 {
			r.in = false
			if n := tl.rows(); n > 0 { // eklenen şarkı listenin sonuna geçmiş olabilir
				tl.cursor = tl.at(n - 1)
				tl.clamp(m.listHeight())
			}
		} else {
			r.cursor--
		}
	case "down", "j":
		r.cursor = min(len(r.tracks)-1, r.cursor+1)
	case "end", "G", "pgdown":
		r.cursor = len(r.tracks) - 1
	case "pgup", "home", "g":
		r.in = false
		return nil, false // listede sayfa yukarı / başa
	case "enter", "right":
		uri := r.tracks[r.cursor].URI
		return m.playOn(func(ctx context.Context, sp *spotify.Client, id string) error {
			return sp.PlayTracks(ctx, id, []string{uri}, uri)
		}), true
	case "a":
		if r.adding {
			return nil, true
		}
		t := r.tracks[r.cursor]
		r.tracks = append(r.tracks[:r.cursor:r.cursor], r.tracks[r.cursor+1:]...)
		if len(r.tracks) == 0 {
			r.in, r.state = false, recsNone // yenileri istensin
		}
		r.cursor = max(0, min(r.cursor, len(r.tracks)-1))
		r.adding = true
		sp, key, list := m.sp, tl.entry.key, tl.entry.name
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			return recAddedMsg{key: key, t: t, list: list, err: sp.AddToPlaylist(ctx, key, []string{t.URI})}
		}, true
	case "R":
		r.tracks, r.cursor = nil, 0
		r.in = false
		return m.fetchRecs(tl), true
	default:
		return nil, false
	}
	return nil, true
}

// recsRows: listenin altına çizilen öneri satırları; room satıra sığar.
func (m Model) recsRows(tl *trackList, w, room int, active bool) []string {
	r := tl.recs
	if !m.recsWanted(tl) || room < 3 {
		return nil
	}
	head := theme.Dim.Italic(true).Render(T("recs.title"))
	switch r.state {
	case recsLoading:
		return []string{"", head + theme.Faint.Render("  "+T("lib.loading"))}
	case recsReady:
	default:
		return nil
	}
	if r.in {
		head += theme.Faint.Render(T("recs.keys"))
	}
	out := []string{"", head}
	start := 0
	if room-2 < len(r.tracks) && r.in {
		start = max(0, min(r.cursor-(room-3), len(r.tracks)-(room-2)))
	}
	for i := start; i < len(r.tracks) && len(out) < room; i++ {
		t := r.tracks[i]
		out = append(out, "  + "+row(t.Name, t.ArtistNames()+"  "+clock(time.Duration(t.DurationMS)*time.Millisecond), w-4, active && r.in && i == r.cursor))
	}
	return out
}
