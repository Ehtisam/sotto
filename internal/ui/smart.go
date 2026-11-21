package ui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/connect"
	"github.com/Ehtisam/sotto/internal/spclient"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// Akıllı karıştırma (✱): s üç durum arasında geçer, Spotify'daki gibi:
// kapalı → karıştır → akıllı karıştır → kapalı. Akıllıyken çalan bağlamın
// (liste, albüm, sanatçı; yoksa şarkının) radyosundan öneriler alınır ve her
// üç şarkıda bir öneri resmî "sıraya ekle" ile sıraya girer (aynı anda en
// çok bir tane bekler). Kapatınca bekleyen öneri sıradan çıkarılır.
// Ekstralar kapalıysa s yalnızca karıştırmayı açıp kapatır.

const smartEvery = 3

type smartState struct {
	on      bool
	seed    string   // önerilerin tohumu (bağlam ya da şarkı)
	recs    []string // öneriler
	loading bool
	since   int    // son öneriden bu yana çalan şarkı
	pending string // sıraya eklenmiş, henüz çalmamış öneri
	lastURI string
	added   map[string]bool // eklenen öneriler (bir daha eklenmez; Sıra'da işaret)
}

type smartRecsMsg struct {
	seed  string
	recs  []string
	login string
	err   error
}

type smartAddedMsg struct {
	uri string
	err error
}

func (m Model) smartOn() bool { return m.rt.smart.on && m.cfg.Extras.Enabled }

// cycleShuffle: s tuşu.
func (m Model) cycleShuffle() (Model, tea.Cmd) {
	z := &m.rt.smart
	shuffle := m.state.Options.Shuffle
	if !m.cfg.Extras.Enabled || m.spState != spReady {
		z.on = false
		return m, m.setShuffle(!shuffle)
	}
	switch {
	case !shuffle:
		z.on = false
		return m, m.setShuffle(true)
	case !z.on:
		z.on, z.since, z.lastURI = true, 0, m.curURI()
		if z.added == nil {
			z.added = map[string]bool{}
		}
		m = m.note(T("smart.on"))
		return m, m.smartFetch()
	default:
		z.on = false
		pending := z.pending
		z.pending = ""
		m = m.note(T("smart.off"))
		return m, tea.Batch(m.setShuffle(false), m.smartRemove(pending))
	}
}

func (m Model) curURI() string {
	if it := m.state.Item; it != nil {
		return it.URI
	}
	return ""
}

// smartSeed: önerilerin tohumu.
func (m Model) smartSeed() string {
	if ctx := m.state.Context; ctx != nil {
		for _, p := range []string{"spotify:playlist:", "spotify:album:", "spotify:artist:"} {
			if strings.HasPrefix(ctx.URI, p) {
				return ctx.URI
			}
		}
	}
	if u := m.curURI(); strings.HasPrefix(u, "spotify:track:") {
		return u
	}
	return ""
}

func (m Model) smartFetch() tea.Cmd {
	z := &m.rt.smart
	seed := m.smartSeed()
	if seed == "" || z.loading {
		return nil
	}
	z.loading, z.seed = true, seed
	br := m.rt.bridge
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		msg := smartRecsMsg{seed: seed}
		msg.login, msg.err = withBridge(ctx, br, func(tok string) error {
			var err error
			msg.recs, err = spclient.Recommendations(ctx, tok, seed)
			return err
		})
		return msg
	}
}

// smartTick, saat tıkında: şarkı değiştiyse sayar, sırası gelince öneri ekler.
func (m Model) smartTick() tea.Cmd {
	z := &m.rt.smart
	if !m.smartOn() {
		return nil
	}
	if !m.state.Options.Shuffle && m.state.Item != nil { // karıştırma başka yerden kapatıldı
		z.on = false
		return nil
	}
	var cmds []tea.Cmd
	if seed := m.smartSeed(); seed != "" && seed != z.seed && !z.loading &&
		!strings.HasPrefix(seed, "spotify:track:") { // bağlam değişti: yeni öneriler
		z.recs = nil
		cmds = append(cmds, m.smartFetch())
	}
	cur := m.curURI()
	if cur == "" || cur == z.lastURI {
		return tea.Batch(cmds...)
	}
	z.lastURI = cur
	if cur == z.pending {
		z.pending = ""
	}
	if !z.added[cur] {
		z.since++
	}
	if z.since < smartEvery || z.pending != "" {
		return tea.Batch(cmds...)
	}
	for _, u := range z.recs {
		if !z.added[u] && u != cur {
			z.added[u], z.pending, z.since = true, u, 0
			cmds = append(cmds, m.smartAdd(u))
			break
		}
	}
	return tea.Batch(cmds...)
}

func (m Model) smartAdd(uri string) tea.Cmd {
	c, sp, solo := m.rt.client, m.sp, m.soloistActive()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var err error
		if solo {
			err = c.AddToQueue(ctx, uri)
		} else {
			err = sp.AddToQueue(ctx, uri)
		}
		return smartAddedMsg{uri, err}
	}
}

// smartRemove, bekleyen öneriyi sıradan çıkarır (connect-state, ✱).
func (m Model) smartRemove(uri string) tea.Cmd {
	if uri == "" {
		return nil
	}
	br := m.rt.bridge
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_, err := withBridge(ctx, br, func(tok string) error {
			s, err := connect.Open(ctx, tok)
			if err != nil {
				return err
			}
			defer s.Close()
			next := s.Cluster().PlayerState.NextTracks
			for k, t := range next {
				if t.URI == uri && t.Queued() {
					return s.SetQueue(ctx, connect.Remove(next, k))
				}
			}
			return nil
		})
		if err != nil {
			return flashMsg(err.Error())
		}
		return pollSoonMsg{}
	}
}

func (m Model) updateSmart(msg tea.Msg) (Model, tea.Cmd) {
	z := &m.rt.smart
	switch msg := msg.(type) {
	case smartRecsMsg:
		z.loading = false
		if msg.seed != z.seed {
			return m, nil
		}
		switch {
		case msg.login != "":
			z.on = false
			return m.say(T("smart.login")), nil
		case msg.err != nil:
			m.rt.logs.add(logError, "smart shuffle: "+msg.err.Error(), m.now)
			return m.say(T("smart.norecs")), nil
		}
		z.recs = msg.recs
	case smartAddedMsg:
		if msg.err != nil {
			if z.pending == msg.uri {
				z.pending = ""
			}
			m.rt.logs.add(logError, "smart shuffle: "+msg.err.Error(), m.now)
			return m, nil
		}
		return m, m.refreshQueueSoon()
	}
	return m, nil
}

// refreshQueueSoon: Sıra açıksa yeniden okunsun.
func (m Model) refreshQueueSoon() tea.Cmd {
	return func() tea.Msg { return pollSoonMsg{} }
}

// smartMark: şeritte karıştırma işaretinin yanında.
func (m Model) smartMark() string {
	if m.smartOn() && m.state.Options.Shuffle {
		return theme.GoldText.Render("✦")
	}
	return ""
}
