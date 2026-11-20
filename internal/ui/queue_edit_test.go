package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/Ehtisam/sotto/internal/connect"
)

func upcomingNames(m Model) string {
	var out []string
	for _, q := range m.queue.Upcoming {
		out = append(out, q.Item.Name())
	}
	return strings.Join(out, " ")
}

// queueAt, imleci adı verilen satıra koyar.
func queueAt(t *testing.T, m Model, name string) Model {
	t.Helper()
	for i, it := range m.queueItems() {
		if it.name == name {
			m.qs.cursor = i
			return m
		}
	}
	t.Fatalf("no row %q", name)
	return m
}

func TestQueueEditNeedsExtras(t *testing.T) {
	m := queueAt(t, queueModel(t), "c1")
	m.cfg.Extras.Enabled = false
	mm, cmd, ok := m.queueKey("x")
	if !ok || cmd != nil || !strings.Contains(mm.flash, "extras") || upcomingNames(mm) != upcomingNames(m) {
		t.Fatalf("flash %q, queue %q", mm.flash, upcomingNames(mm))
	}
}

func TestQueueRemove(t *testing.T) {
	m := queueAt(t, queueModel(t), "c1")
	m.cfg.Extras.Enabled = true
	mm, cmd, _ := m.queueKey("x")
	if cmd == nil || !mm.qs.editing {
		t.Fatal("no command sent")
	}
	if got := upcomingNames(mm); got != "q1  a1" { // adsız şarkı ortada
		t.Fatalf("queue = %q", got)
	}
	// Komut yoldayken ikinci düzenleme beklemez, yok sayılır.
	if _, cmd2, _ := mm.queueKey("x"); cmd2 != nil {
		t.Fatal("second edit while busy")
	}
	// Cevap Update üzerinden gelir.
	res, _ := mm.Update(queueEditMsg{remove: true, name: "c1"})
	r := res.(Model)
	if r.qs.editing || !strings.Contains(r.flash, "c1") {
		t.Fatalf("after reply: editing %v flash %q", r.qs.editing, r.flash)
	}
	res, _ = mm.Update(queueEditMsg{err: connect.ErrNotFound})
	if r := res.(Model); !strings.Contains(r.flash, "changed") {
		t.Fatalf("error flash = %q", r.flash)
	}
	res, _ = mm.Update(queueEditMsg{err: errors.New("boom")})
	if r := res.(Model); r.flash != "boom" {
		t.Fatalf("error flash = %q", r.flash)
	}
}

func TestQueueMove(t *testing.T) {
	m := queueAt(t, queueModel(t), "c1")
	m.cfg.Extras.Enabled = true
	mm, cmd, _ := m.queueKey("J")
	if cmd == nil || upcomingNames(mm) != "q1  c1 a1" {
		t.Fatalf("move down: %q", upcomingNames(mm))
	}
	// İmleç taşınan şarkıyla gider.
	items := mm.queueItems()
	if it := items[mm.queueCursor(items)]; it.uri != "spotify:track:c1" {
		t.Fatalf("cursor on %q", it.uri)
	}
	// Bölüm sınırı: listeden gelen şarkı sıraya eklenenlerin üstüne çıkmaz.
	if mm, cmd, _ := m.queueKey("K"); cmd != nil || upcomingNames(mm) != upcomingNames(m) {
		t.Fatalf("crossed into the queue: %q", upcomingNames(mm))
	}
	// Kumandada tek bölüm: geçebilir.
	m.remote.active = true
	if mm, cmd, _ := queueAt(t, m, "c1").queueKey("K"); cmd == nil || upcomingNames(mm) != "c1 q1  a1" {
		t.Fatalf("remote move up: %q", upcomingNames(mm))
	}
}

func TestQueueEditSkipsNowPlaying(t *testing.T) {
	m := queueAt(t, queueModel(t), "now")
	m.cfg.Extras.Enabled = true
	if _, cmd, _ := m.queueKey("x"); cmd != nil {
		t.Fatal("tried to remove the playing song")
	}
}
