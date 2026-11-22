package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Ehtisam/sotto/internal/soloist"
	"github.com/Ehtisam/sotto/internal/spotify"
)

func sized(m Model, w, h int) Model {
	out, _ := m.update(tea.WindowSizeMsg{Width: w, Height: h})
	return out.(Model)
}

func TestMiniViewSizes(t *testing.T) {
	m := eventsModel(t)
	m.state.Item.Decorations.Creators = append(m.state.Item.Decorations.Creators, struct {
		Entity soloist.Entity `json:"entity"`
	}{})
	m.state.Item.Decorations.Creators[0].Entity.Decorations.Identity.Name = "Seksendört"
	m.state.Item.Decorations.Playback.DurationMS = 264000
	for _, sz := range [][2]int{{110, 10}, {50, 30}, {45, 12}, {30, 4}, {24, 2}, {20, 1}} {
		mm := sized(m, sz[0], sz[1])
		if !mm.miniMode() {
			t.Fatalf("%v: not mini", sz)
		}
		s := mm.render()
		lines := strings.Split(s, "\n")
		if len(lines) != sz[1] {
			t.Errorf("%v: %d lines", sz, len(lines))
		}
		for i, l := range lines {
			if w := ansi.StringWidth(l); w > sz[0] {
				t.Errorf("%v: line %d is %d wide", sz, i, w)
			}
		}
		plain := ansi.Strip(s)
		if sz[1] >= 2 && !strings.Contains(plain, "now") {
			t.Errorf("%v: no title:\n%s", sz, plain)
		}
		if sz[1] >= 2 && !strings.Contains(plain, "❚❚") {
			t.Errorf("%v: no status:\n%s", sz, plain)
		}
		if sz[1] >= 3 && !strings.Contains(plain, "Seksendört") {
			t.Errorf("%v: no artist:\n%s", sz, plain)
		}
	}
	// Büyük pencerede normal görünüm.
	if sized(m, 110, 32).miniMode() {
		t.Fatal("mini at full size")
	}
	// Mini'de de tuşlar çalışır.
	mm := sized(m, 40, 8)
	if _, cmd := mm.Update(tea.KeyPressMsg{Code: ' ', Text: " "}); cmd == nil {
		t.Fatal("space did nothing in mini view")
	}
}

func TestMiniNothingPlaying(t *testing.T) {
	m := sized(eventsModel(t), 40, 6)
	m.state.Item = nil
	m.rt.ev.last = lastPlay{Track: "spotify:track:x", Title: "Neyin Var"}
	plain := ansi.Strip(m.render())
	if !strings.Contains(plain, "Nothing is playing") || !strings.Contains(plain, "Neyin Var") {
		t.Fatalf("%s", plain)
	}
}

// Telefon sesini ve tekrarı uzaktan değiştirtmiyorsa komut gönderilmez,
// nedeni söylenir (Spotify'ın 403'ü yerine).
func TestRemoteRestrictions(t *testing.T) {
	m := eventsModel(t)
	no := false
	st := &spotify.PlayerState{}
	st.Device.Name, st.Device.SupportsVolume = "Pixel 8", &no
	st.Actions.Disallows = map[string]bool{"toggling_repeat_context": true, "toggling_shuffle": true}
	m.remote.active, m.remote.device, m.remote.player = true, "Pixel 8", st
	for _, c := range []struct {
		cmd  tea.Cmd
		want string
	}{
		{m.setVolume(50), "volume"},
		{m.setRepeat("context"), "doesn't allow"},
		{m.setShuffle(true), "doesn't allow"},
	} {
		msg := c.cmd()
		f, ok := msg.(flashMsg)
		if !ok || !strings.Contains(string(f), c.want) || !strings.Contains(string(f), "Pixel 8") {
			t.Errorf("got %#v, want flash with %q", msg, c.want)
		}
	}
	// İzin verilenler kısıta takılmaz.
	if c := m.refused("repeat:track"); c != nil {
		t.Error("repeat:track refused")
	}
	// Alt şeritteki ses sönük ama görünür.
	m = sized(m, 110, 32)
	if !strings.Contains(ansi.Strip(m.render()), "vol") {
		t.Error("no volume in strip")
	}
	_ = time.Second
}

// Menünün altındaki kapak pencereye göre küçülür; alçak pencerede de görünür.
func TestMenuCoverShrinksToFit(t *testing.T) {
	const used = 10 // başlık, boşluk, 8 sayfa
	for h, want := range map[int]int{19: 8, 25: 9, 15: 4, 14: 0, 40: 9} {
		if got := menuCoverRows(h, used); got != want {
			t.Errorf("body %d: cover %d rows, want %d", h, got, want)
		}
	}
	// Gövde 19 satır (26 satırlık pencere): menü taşmaz.
	m := eventsModel(t)
	m.page = pageSettings
	m.state.Item.Decorations.VisualIdentity.Cover = []soloist.Image{{URL: "https://example.com/c.jpg", Size: "default"}}
	m.cfg.Look.Covers = "pixel"
	m = sized(m, 110, 26)
	if rows := strings.Split(m.menu(19), string(rune(10))); len(rows) != 19 {
		t.Fatalf("menu %d rows", len(rows))
	}
}

// Ayarlar kesilince en altta altı boş bir bölüm başlığı kalmaz.
func TestSettingsCutHasNoLoneHeader(t *testing.T) {
	m := eventsModel(t)
	m.page, m.focus = pageSettings, focusMenu
	sections := map[string]bool{}
	for _, r := range settingRows {
		if r.id < 0 {
			sections[T(r.section)] = true
		}
	}
	for h := 8; h <= 30; h++ {
		for cur := 0; cur < len(settingIDs()); cur++ {
			m.set.cursor = cur
			rows := m.settingsView(80, h)
			// açıklama satırlarından önceki son dolu satır bir başlık olmamalı
			body := rows
			if len(body) > 2 && strings.TrimSpace(ansi.Strip(body[len(body)-2])) == "" {
				body = body[:len(body)-2]
			}
			last := strings.TrimSpace(ansi.Strip(body[len(body)-1]))
			if sections[last] {
				t.Fatalf("h=%d cursor=%d: ends with header %q", h, cur, last)
			}
		}
	}
}
