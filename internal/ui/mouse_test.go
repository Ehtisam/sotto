package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/soloist"
)

func mouseModel(t *testing.T) (*fakeWeb, Model) {
	f, m := longList(t)
	m.width, m.height = 120, 30
	m.cfg.Look.Mouse = true
	it := &soloist.Entity{URI: "spotify:track:t1", EntityType: "track"}
	it.Decorations.Playback.DurationMS = 200000
	m.state.Item = it
	m.render() // ilerleme çubuğunun yeri
	return f, m
}

func click(m Model, x, y int) Model {
	out, cmd := m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	return settle(out.(Model), cmd)
}

func wheel(m Model, x, y int, up bool) Model {
	b := tea.MouseWheelDown
	if up {
		b = tea.MouseWheelUp
	}
	out, cmd := m.Update(tea.MouseWheelMsg{X: x, Y: y, Button: b})
	return settle(out.(Model), cmd)
}

func TestMouseRowsSelectThenPlay(t *testing.T) {
	f, m := mouseModel(t)
	l := m.layout()
	sel := m.selectedContentRow()
	if sel < 0 {
		t.Fatal("no selected row found")
	}
	m = click(m, l.contentX+5, l.bodyTop+sel+3) // üç satır aşağı
	if m.lib.open.cursor != 3 {
		t.Fatalf("cursor %d", m.lib.open.cursor)
	}
	f.on("GET me/player/devices", `{"devices":[]}`)
	m = click(m, l.contentX+5, l.bodyTop+sel+3) // seçiliye yeniden: çalar
	play := f.sent("PUT me/player/play")
	if len(play) != 1 || !strings.Contains(f.bodyOf(play[0]), "spotify:track:t3") {
		t.Fatalf("play %v", play)
	}
	m = wheel(m, l.contentX+5, l.bodyTop+5, false)
	if m.lib.open.cursor != 4 {
		t.Fatalf("wheel cursor %d", m.lib.open.cursor)
	}
}

func TestMouseMenuStripAndBar(t *testing.T) {
	f, m := mouseModel(t)
	l := m.layout()
	m = click(m, margin+3, l.bodyTop+2+4) // V · Sıra
	if m.page != pageQueue {
		t.Fatalf("menu click → page %v", m.page)
	}
	m = wheel(m, 10, l.stripY, true)
	if got := f.sent("PUT me/player/volume"); len(got) != 1 || !strings.Contains(got[0], "volume_percent=45") {
		t.Fatalf("wheel volume %v", got)
	}
	m.render()
	z := m.rt.bar
	m = click(m, z.x+z.w/2, l.stripY+1)
	seek := f.sent("PUT me/player/seek")
	if len(seek) != 1 || !strings.Contains(seek[0], "position_ms=") {
		t.Fatalf("bar click %v", seek)
	}
	m = click(m, margin, l.stripY) // düğme: duraklat
	if len(f.sent("PUT me/player/pause")) != 1 {
		t.Fatalf("button %v", f.sent("PUT"))
	}
	m = click(m, 30, l.stripY) // şarkının adı
	if m.page != pageNow {
		t.Fatal("title click should open Now playing")
	}
}

func TestMouseOffDoesNothing(t *testing.T) {
	_, m := mouseModel(t)
	m.cfg.Look.Mouse = false
	l := m.layout()
	m = click(m, margin+3, l.bodyTop+2+4)
	if m.page != pageLibrary {
		t.Fatal("mouse off but click worked")
	}
	if v := m.View(); v.MouseMode != tea.MouseModeNone {
		t.Fatal("mouse mode still on")
	}
}
