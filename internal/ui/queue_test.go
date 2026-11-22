package ui

import (
	"testing"

	"github.com/Ehtisam/sotto/internal/soloist"
	"github.com/Ehtisam/sotto/internal/spotify"
)

func entity(name string) *soloist.Entity {
	e := &soloist.Entity{URI: "spotify:track:" + name, EntityType: "track"}
	e.Decorations.Identity.Name = name
	return e
}

func queueModel(t *testing.T) Model {
	t.Helper()
	m := homeModel(t)
	m.spState = spReady
	m.page, m.focus = pageQueue, focusContent
	m.state.Item = entity("now")
	ctx := &soloist.Entity{URI: "spotify:playlist:pl", EntityType: "playlist"}
	ctx.Decorations.Identity.Name = "Road Trip"
	m.state.Context = ctx
	m.queue.Upcoming = []soloist.QueueEntry{
		{Source: "queue", Item: entity("q1")},
		{Source: "context", Item: entity("c1")},
		{Source: "context", Item: &soloist.Entity{URI: "spotify:track:noname", EntityType: "track"}},
		{Source: "autoplay", Item: entity("a1")},
	}
	return m
}

func kindsOf(items []searchItem) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.kind)
	}
	return out
}

func TestQueueSections(t *testing.T) {
	m := queueModel(t)
	items := m.queueItems()
	want := []string{
		"header", "track", "space", // şimdi çalıyor
		"header", "track", "space", // sıraya eklenenler
		"header", "track", "track", "space", // listeden sıradakiler
		"header", "track", // otomatik devam
	}
	got := kindsOf(items)
	if len(got) != len(want) {
		t.Fatalf("kinds = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("kinds = %v, want %v", got, want)
		}
	}
	if items[6].name != "Next from Road Trip" {
		t.Fatalf("context header = %q", items[6].name)
	}
	// Atlama sayıları sıradaki yerle aynı; çalan şarkı -1.
	skips := map[string]int{"now": -1, "q1": 1, "c1": 2, "a1": 4}
	for _, it := range items {
		if want, ok := skips[it.name]; ok && it.skip != want {
			t.Errorf("%s skip = %d, want %d", it.name, it.skip, want)
		}
	}
	// Adı henüz gelmemiş şarkı da yerini korur.
	if items[8].name != "loading…" || items[8].skip != 3 {
		t.Fatalf("unnamed row = %q skip %d", items[8].name, items[8].skip)
	}
}

func TestQueueRemoteIsOneSection(t *testing.T) {
	m := queueModel(t)
	m.remote.active, m.remote.device = true, "Phone"
	got := kindsOf(m.queueItems())
	// şimdi çalıyor + tek "sıradakiler" bölümü
	want := []string{"header", "track", "space", "header", "track", "track", "track", "track"}
	if len(got) != len(want) {
		t.Fatalf("kinds = %v, want %v", got, want)
	}
}

func TestQueueRecentlyPlayed(t *testing.T) {
	m := queueModel(t)
	m.queue.Upcoming = nil
	at := m.now.Add(-10 * 60e9)
	m.home.recent = []spotify.PlayHistory{
		play("x", "spotify:playlist:pl", "playlist", at),
		play("x", "spotify:playlist:pl", "playlist", at),
		play("y", "", "", at),
	}
	items := m.queueItems()
	got := kindsOf(items)
	want := []string{"header", "track", "space", "header", "track", "track"}
	if len(got) != len(want) {
		t.Fatalf("kinds = %v, want %v", got, want)
	}
	if items[4].skip != 0 || items[4].ctx != "spotify:playlist:pl" {
		t.Fatalf("recent row: skip %d ctx %q", items[4].skip, items[4].ctx)
	}
}

func TestQueueCursorAndActions(t *testing.T) {
	m := queueModel(t)
	// İmleç başlıkta kalmaz: ilk seçilebilir satır çalan şarkı.
	if it := m.queueItems()[m.queueCursor(m.queueItems())]; it.name != "now" {
		t.Fatalf("first row = %q", it.name)
	}
	m, _, _ = m.queueKey("down")
	m, _, _ = m.queueKey("down")
	items := m.queueItems()
	if it := items[m.queueCursor(items)]; it.name != "c1" {
		t.Fatalf("after two downs = %q", it.name)
	}
	target, from, ok := m.actionTarget()
	if !ok || target.uri != "spotify:track:c1" || target.kind != "track" || from != nil {
		t.Fatalf("action target = %+v %v %v", target, from, ok)
	}
}
