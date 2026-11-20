package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/soloist"
	"github.com/Ehtisam/sotto/internal/spotify"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// V · Sıra: çalan şarkı, sıraya eklenenler, bağlamdan sıradakiler ve son
// çalınanlar. Enter seçili şarkıya atlar (aradakiler geçilir), a eylem
// menüsünü açar.

// Kumanda modunda Web API sırayı kendiliğinden bildirmiyor; sayfa açıkken
// bu aralıkla yeniden sorulur.
const queueRefresh = 5 * time.Second

// Bir seferde en çok bu kadar şarkı atlanır (Soloist de Web API de sıranın
// ilk 20 kadarını veriyor).
const maxSkip = 20

type queueState struct {
	cursor  int
	scroll  int
	polling bool      // kumanda: sıra soruluyor
	fetched time.Time // kumanda: sıranın en son sorulduğu an
}

// queueItems, sayfanın satırları. Her çizimde durumdan yeniden kurulur;
// böylece sıra değişince sayfa kendiliğinden güncellenir.
func (m Model) queueItems() []searchItem {
	var out []searchItem
	section := func(key, title string, items []searchItem) {
		if len(items) == 0 {
			return
		}
		if len(out) > 0 {
			out = append(out, searchItem{kind: "space"})
		}
		out = append(out, searchItem{kind: "header", section: key, name: title})
		out = append(out, items...)
	}

	if it := m.state.Item; it != nil && it.URI != "" {
		item := m.entityItem(it)
		item.skip = -1 // çalan şarkı: enter Şimdi çalıyor sayfasına götürür
		section("queue.now", "", []searchItem{item})
	}

	// Soloist sıranın nereden geldiğini söylüyor; Web API söylemiyor, o
	// zaman hepsi tek bölümde görünür.
	var queued, next, auto []searchItem
	for i, q := range m.queue.Upcoming {
		if q.Item == nil || q.Item.URI == "" || i >= maxSkip {
			continue
		}
		item := m.entityItem(q.Item)
		item.skip = i + 1
		switch {
		case m.remote.active:
			next = append(next, item)
		case q.Source == "queue":
			queued = append(queued, item)
		case q.Source == "autoplay":
			auto = append(auto, item)
		default:
			next = append(next, item)
		}
	}
	section("queue.queued", "", queued)
	nextTitle := ""
	if ctx := m.state.Context; ctx != nil && ctx.Name() != "" && !m.remote.active {
		nextTitle = F("queue.from", ctx.Name())
	}
	section("queue.next", nextTitle, next)
	section("queue.auto", "", auto)

	// Son çalınanlar: ana sayfanın verisi, art arda aynı şarkı bir kez.
	var recent []searchItem
	last := ""
	for _, r := range m.home.recent {
		t := r.Track
		if t.URI == last || len(recent) >= 10 {
			continue
		}
		last = t.URI
		ctx := ""
		if r.Context != nil && r.Context.Type != "collection" {
			ctx = r.Context.URI
		}
		tt := t
		recent = append(recent, searchItem{kind: trackKind(t), name: t.Name, sub: t.ArtistNames() + "  " + ago(m.now, r.PlayedAt), uri: t.URI, track: &tt, ctx: ctx})
	}
	section("home.recent", "", recent)
	return out
}

// entityItem, Soloist'in (ya da Web API'den çevrilmiş) bir öğesini satıra
// çevirir; adı henüz bilinmiyorsa yükleniyor yazar.
func (m Model) entityItem(e *soloist.Entity) searchItem {
	t := m.trackFromEntity(e)
	name, artists, _ := m.describe(e)
	item := searchItem{kind: trackKind(t), name: name, sub: artists, uri: e.URI, track: &t}
	if item.name == "" {
		item.name = T("lib.loading")
	}
	if d := e.Duration(); d > 0 {
		if item.sub != "" {
			item.sub += "  "
		}
		item.sub += clock(d)
	}
	return item
}

// trackFromEntity, eylem menüsü için öğeden bir şarkı kurar; Web API'den
// gelen bilgi varsa onu kullanır.
func (m Model) trackFromEntity(e *soloist.Entity) spotify.Track {
	if t, ok := m.rt.known[e.URI]; ok {
		return t
	}
	name, _, album := m.describe(e)
	t := spotify.Track{URI: e.URI, Name: name, Type: e.EntityType}
	if p := e.Decorations.Parent; p != nil {
		t.Album = spotify.Album{ID: spotify.IDFromURI(p.Entity.URI), URI: p.Entity.URI, Name: album}
	}
	for _, c := range e.Decorations.Creators {
		t.Artists = append(t.Artists, spotify.Artist{Name: c.Entity.Name(), URI: c.Entity.URI})
	}
	return t
}

func trackKind(t spotify.Track) string {
	if t.Type == "episode" {
		return "episode"
	}
	return "track"
}

// refreshQueue, sırayı yeniden ister: Soloist'ten ya da (kumandada) Web API'den.
func (m Model) refreshQueue() (Model, tea.Cmd) {
	if m.soloistActive() {
		return m, m.do(func(ctx context.Context, c *soloist.Client) error { return c.RequestQueue(ctx, maxSkip) })
	}
	if !m.shouldPoll() || m.qs.polling {
		return m, nil
	}
	m.qs.polling = true
	return m, m.pollQueue()
}

// maybeRefreshQueue, kumanda modunda sayfa açıkken sırayı arada bir tazeler.
func (m Model) maybeRefreshQueue() (Model, tea.Cmd) {
	if m.page != pageQueue || !m.shouldPoll() || m.qs.polling || m.now.Sub(m.qs.fetched) < queueRefresh {
		return m, nil
	}
	return m.refreshQueue()
}

// skipTo, n şarkı ileri atlar: seçili şarkı çalmaya başlar, aradakiler geçilir.
func (m Model) skipTo(n int) tea.Cmd {
	n = max(1, min(n, maxSkip))
	return m.control(
		func(ctx context.Context, c *soloist.Client) error {
			for range n {
				if err := c.Next(ctx); err != nil {
					return err
				}
			}
			return nil
		},
		func(ctx context.Context, s *spotify.Client) error {
			for range n {
				if err := s.Next(ctx); err != nil {
					return err
				}
			}
			return nil
		})
}

func (m Model) queueKey(k string) (Model, tea.Cmd, bool) {
	items := m.queueItems()
	q := &m.qs
	lh := m.listHeight()
	move := func(d int) {
		q.cursor = nextSelectable(items, m.queueCursor(items)+d, d)
		q.cursor, q.scroll = fitScroll(q.cursor, q.scroll, len(items), lh)
		if q.cursor == nextSelectable(items, 0, 1) {
			q.scroll = 0 // ilk bölüm başlığı da görünsün
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
	case "enter", "right":
		if len(items) == 0 {
			return m, nil, true
		}
		it := items[m.queueCursor(items)]
		switch {
		case it.skip < 0:
			m.page, m.focus = pageNow, focusMenu
			return m, nil, true
		case it.skip > 0:
			q.cursor, q.scroll = 0, 0 // atlayınca liste baştan kurulur
			return m, m.skipTo(it.skip), true
		}
		_, cmd := m.actOn(it) // son çalınanlar: kendi bağlamında çalar
		return m, cmd, true
	case "R":
		m.home.fetched = time.Time{}
		mm, cmd := m.refreshQueue()
		hcmd := mm.maybeLoadHome()
		return mm, tea.Batch(cmd, hcmd), true
	case "esc", "left", "backspace":
		m.focus = focusMenu
	default:
		return m, nil, false
	}
	return m, nil, true
}

// queueCursor, imleci satırlar değişmiş olsa bile seçilebilir bir satıra oturtur.
func (m Model) queueCursor(items []searchItem) int {
	return nextSelectable(items, m.qs.cursor, 1)
}

func (m Model) queueView(w int) []string {
	title := theme.Title.Render(T("page.queue"))
	if m.spState != spReady && !m.soloistLive() {
		return []string{title, "", theme.Dim.Render(T("flash.noaccount"))}
	}
	sub := T("queue.sub")
	if m.remote.active && m.remote.device != "" {
		sub = F("queue.sub.remote", m.remote.device)
	}
	rows := []string{title, theme.Dim.Render(sub), "", ""}
	items := m.queueItems()
	if len(items) == 0 {
		return append(rows, theme.Dim.Render(T("queue.empty")))
	}
	cursor := m.queueCursor(items)
	_, scroll := fitScroll(cursor, m.qs.scroll, len(items), m.listHeight())
	return append(rows, m.itemRows(items, cursor, scroll, w, m.focus == focusContent)...)
}
