package ui

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/bridge"
	"github.com/Ehtisam/sotto/internal/connect"
	"github.com/Ehtisam/sotto/internal/soloist"
	"github.com/Ehtisam/sotto/internal/spclient"
)

// Sıradan silme ve sıra içinde taşıma (✱). Soloist'in API'si de resmî Web
// API de bunu bilmiyor; Spotify'ın kendi uygulamaları gibi connect-state
// üzerinden çalan cihaza yeni sırayı gönderiyoruz. Sayfa anında güncellenir,
// komut arkadan gider; olmazsa sıra yeniden istenip gerçeği gösterilir.

type queueEditMsg struct {
	remove bool
	clear  bool // sırayı temizle: cleared kaç şarkı çıktığı
	clearN int
	name   string
	login  string
	err    error
}

var errNoMove = errors.New("queue: can't move past its section")

// editQueue, imleçteki şarkıyı siler (d == 0) ya da d yönünde bir sıra taşır.
func (m Model) editQueue(d int) (Model, tea.Cmd) {
	items := m.queueItems()
	if len(items) == 0 {
		return m, nil
	}
	cursor := m.queueCursor(items)
	it := items[cursor]
	if it.skip <= 0 || it.skip > len(m.queue.Upcoming) {
		return m, nil // çalan şarkı ya da son çalınanlar
	}
	if !m.cfg.Extras.Enabled {
		return m.say(T("queue.edit.extras")), nil
	}
	if m.qs.editing {
		return m, nil
	}
	i := it.skip - 1
	entry := m.queue.Upcoming[i]
	if entry.Item == nil {
		return m, nil
	}

	up := append([]soloist.QueueEntry(nil), m.queue.Upcoming...)
	if d == 0 {
		up = append(up[:i], up[i+1:]...)
	} else {
		j := i + d
		if j < 0 || j >= len(up) || j >= maxSkip || !m.sameSection(up[i], up[j]) {
			return m, nil
		}
		up[i], up[j] = up[j], up[i]
		m.qs.cursor = cursor + d
		m.qs.cursor, m.qs.scroll = fitScroll(m.qs.cursor, m.qs.scroll, len(items), m.listHeight())
	}
	m.queue.Upcoming = up
	m.qs.editing = true

	br, uri, uid, name := m.rt.bridge, entry.Item.URI, entry.UID, it.name
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		login, err := sendQueueEdit(ctx, br, i, uri, uid, d)
		return queueEditMsg{remove: d == 0, name: name, login: login, err: err}
	}
}

// sameSection, iki şarkı Sıra sayfasında aynı bölümde mi.
func (m Model) sameSection(a, b soloist.QueueEntry) bool {
	if m.remote.active {
		return true // Web API kaynağı söylemiyor, tek bölüm
	}
	sec := func(s string) string {
		if s == "queue" || s == "autoplay" {
			return s
		}
		return "context"
	}
	return sec(a.Source) == sec(b.Source)
}

// sendQueueEdit, gözlemci olarak bağlanıp kümedeki sırayı okur, pos'uncu
// şarkıyı bulur, değiştirir ve yeni sırayı çalan cihaza gönderir.
func sendQueueEdit(ctx context.Context, br *bridge.Bridge, pos int, uri, uid string, d int) (string, error) {
	return withBridge(ctx, br, func(tok string) error {
		s, err := connect.Open(ctx, tok)
		if errors.Is(err, connect.ErrUnauthorized) {
			return spclient.ErrUnauthorized
		}
		if err != nil {
			return err
		}
		defer s.Close()
		next := s.Cluster().PlayerState.NextTracks
		k := connect.Locate(next, pos, uri, uid)
		if k < 0 {
			return connect.ErrNotFound
		}
		if d == 0 {
			next = connect.Remove(next, k)
		} else {
			var ok bool
			if next, ok = connect.Move(next, k, d); !ok {
				return errNoMove
			}
		}
		return s.SetQueue(ctx, next)
	})
}

func (m Model) updateQueueEdit(msg queueEditMsg) (Model, tea.Cmd) {
	m.qs.editing = false
	switch {
	case msg.login != "":
		m = m.say(T("queue.edit.login"))
	case errors.Is(msg.err, connect.ErrNoPlayer):
		m = m.say(T("queue.edit.noplayer"))
	case errors.Is(msg.err, connect.ErrNotFound), errors.Is(msg.err, errNoMove):
		m = m.say(T("queue.edit.changed"))
	case msg.err != nil:
		m = m.say(msg.err.Error())
	case msg.clear && msg.clearN == 0:
		m = m.say(T("queue.clear.none"))
	case msg.clear:
		m = m.note(F("queue.cleared", msg.clearN))
	case msg.remove:
		m = m.note(F("queue.removed", msg.name))
	}
	// Soloist yeni sırayı kendisi bildirir; yine de sor ki hata olduysa
	// sayfadaki tahmini sıra düzelsin.
	m.qs.fetched = time.Time{}
	m.qs.polling = false
	return m.refreshQueue()
}

// queueClearable, eylem menüsünde "Sırayı temizle" görünsün mü: Sıra
// sayfasındayken ve sırada eklenmiş bir şey varken (kumandada kaynak
// bilinmez, o zaman sırada bir şey olması yeter).
func (m Model) queueClearable() bool {
	if m.page != pageQueue || m.focus != focusContent || !m.cfg.Extras.Enabled || m.extraOn() {
		return false
	}
	for _, q := range m.queue.Upcoming {
		if m.remote.active || q.Source == "queue" {
			return true
		}
	}
	return false
}

// clearQueue, kullanıcının sıraya eklediklerini siler (✱, connect set_queue);
// listeden gelenler kalır. Sayfa hemen güncellenir.
func (m Model) clearQueue() (Model, tea.Cmd) {
	if !m.cfg.Extras.Enabled {
		return m.say(T("queue.edit.extras")), nil
	}
	if m.qs.editing {
		return m, nil
	}
	if !m.remote.active {
		var keep []soloist.QueueEntry
		for _, q := range m.queue.Upcoming {
			if q.Source != "queue" {
				keep = append(keep, q)
			}
		}
		if len(keep) == len(m.queue.Upcoming) {
			return m.say(T("queue.clear.none")), nil
		}
		m.queue.Upcoming = keep
		m.qs.cursor, m.qs.scroll = 0, 0
	}
	m.qs.editing = true
	br := m.rt.bridge
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		n := 0
		login, err := withBridge(ctx, br, func(tok string) error {
			s, err := connect.Open(ctx, tok)
			if errors.Is(err, connect.ErrUnauthorized) {
				return spclient.ErrUnauthorized
			}
			if err != nil {
				return err
			}
			defer s.Close()
			var next []connect.Track
			next, n = connect.ClearQueued(s.Cluster().PlayerState.NextTracks)
			if n == 0 {
				return nil
			}
			return s.SetQueue(ctx, next)
		})
		return queueEditMsg{clear: true, clearN: n, login: login, err: err}
	}
}
