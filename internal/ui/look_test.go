package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Ehtisam/sotto/internal/soloist"
	"github.com/Ehtisam/sotto/internal/spotify"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

func lookModel(t *testing.T) Model {
	_, m := longList(t)
	m.state.Item = &soloist.Entity{URI: "spotify:track:t3", EntityType: "track"}
	m.rt.known["spotify:track:t3"] = spotify.Track{Name: "Song 03", Artists: []spotify.Artist{{Name: "Ada"}}, Album: spotify.Album{Name: "First"}}
	return m
}

func TestStripFormatAndWindowTitle(t *testing.T) {
	m := lookModel(t)
	m.cfg.Look.StripFormat = "{artist} | {title} | {album} | {context}"
	got := ansi.Strip(m.stripLine())
	if got != "Ada | Song 03 | First" { // bağlam boş: ayracıyla birlikte düşer
		t.Fatalf("strip %q", got)
	}
	m.cfg.Look.WindowTitle = "♪ {title}"
	if m.windowTitleText() != "♪ Song 03" {
		t.Fatalf("title %q", m.windowTitleText())
	}
	m.cfg.Look.WindowTitle = ""
	if m.windowTitleText() != "sotto" {
		t.Fatal("empty window_title should leave sotto")
	}
}

func TestStripOnTop(t *testing.T) {
	m := lookModel(t)
	m.width, m.height = 100, 30
	m.cfg.Look.Strip = "top"
	lines := strings.Split(ansi.Strip(m.render()), "\n")
	if !strings.Contains(lines[2], "Song 03") {
		t.Fatalf("strip not on line 3:\n%s", strings.Join(lines[:5], "\n"))
	}
	if strings.Contains(lines[len(lines)-3], "Song 03") {
		t.Fatal("strip still at the bottom")
	}
}

func TestRelativeLineNumbers(t *testing.T) {
	m := lookModel(t)
	m.lib.open.cursor = 4
	m.cfg.Look.LineNumbers = "relative"
	rows := ansi.Strip(strings.Join(m.listView(m.lib.open, 90, true), "\n"))
	for _, want := range []string{"2  Song 02", "5    Song 04", "1  Song 05", "3  Song 07"} {
		if !strings.Contains(rows, want) {
			t.Fatalf("missing %q in:\n%s", want, rows)
		}
	}
	m.cfg.Look.LineNumbers = "off"
	rows = ansi.Strip(strings.Join(m.listView(m.lib.open, 90, true), "\n"))
	if strings.Contains(rows, " 1  Song") || !strings.Contains(rows, "  Song 00") {
		t.Fatalf("numbers still shown:\n%s", rows)
	}
}

func TestSettingsLookRowsApply(t *testing.T) {
	m := lookModel(t)
	t.Cleanup(func() { applyLook(m.cfg) })
	m.cfg.Look.Icons, m.cfg.Look.Border = "unicode", "line"
	m, _ = m.change(setIcons, 1)
	if m.cfg.Look.Icons != "ascii" || theme.Icon.Pause != "||" {
		t.Fatalf("icons %q", m.cfg.Look.Icons)
	}
	m, _ = m.change(setBorder, -1)
	if m.cfg.Look.Border != "none" {
		t.Fatalf("border %q", m.cfg.Look.Border)
	}
	m, _ = m.change(setLineNumbers, 1)
	if m.cfg.Look.LineNumbers != "relative" {
		t.Fatalf("line numbers %q", m.cfg.Look.LineNumbers)
	}
	if !strings.Contains(savedFile(t, m), `icons = "ascii"`) {
		t.Fatal("not saved")
	}
	m.cfg.Look.Icons = "unicode"
	applyLook(m.cfg)
}

// Kullanıcı buldu: tema değişince kapak resmi kayboluyordu (ekran yeni
// renklerle boyanıyor, resim yeniden gönderilmiyordu).
func TestThemeChangeRedrawsCovers(t *testing.T) {
	m := lookModel(t)
	t.Cleanup(func() { applyLook(m.cfg) })
	m.gfx.sixel, m.cfg.Look.Covers = true, "image"
	m.rt.gfx.drawn = []gfxPlace{{}}
	m, cmd := m.change(setTheme, 1)
	if m.rt.gfx.drawn != nil || cmd == nil {
		t.Fatalf("theme change: drawn %v cmd nil %v", m.rt.gfx.drawn, cmd == nil)
	}
	if !slicesContainsRedraw(runAll(cmd)) {
		t.Fatal("no redraw scheduled")
	}
}

func slicesContainsRedraw(msgs []tea.Msg) bool {
	for _, msg := range msgs {
		if _, ok := msg.(gfxRedrawMsg); ok {
			return true
		}
	}
	return false
}
