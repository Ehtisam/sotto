package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Ehtisam/sotto/internal/lyrics"
	"github.com/Ehtisam/sotto/internal/soloist"
)

// panelModel: geniş pencere, çalan şarkı, sözler ve sıra.
func panelModel(t *testing.T, w, h int) Model {
	t.Helper()
	_, m := newFakeWeb(t)
	m.width, m.height = w, h
	playingState(&m)
	var lines []lyrics.Line
	for i := 0; i < 30; i++ {
		lines = append(lines, lyrics.Line{Start: time.Duration(i) * 4 * time.Second, Text: fmt.Sprintf("lyric line %d", i)})
	}
	m.rt.lyrics["spotify:track:t1"] = &lyrics.Lyrics{Synced: true, Lines: lines, Source: "LRCLIB"}
	for i := 2; i < 8; i++ {
		e := &soloist.Entity{URI: fmt.Sprintf("spotify:track:t%d", i), EntityType: "track"}
		e.Decorations.Identity.Name = fmt.Sprintf("Next Song %d", i)
		m.queue.Upcoming = append(m.queue.Upcoming, soloist.QueueEntry{Item: e})
	}
	m.page, m.focus = pageLibrary, focusContent
	return m
}

func TestSidePanel(t *testing.T) {
	for _, c := range []struct {
		name   string
		w      int
		layout string
		menu   string
		page   page
		want   bool
	}{
		{"wide plain", 170, "plain", "left", pageLibrary, true},
		{"wide boxed", 170, "boxed", "left", pageLibrary, true},
		{"wide menu right", 170, "plain", "right", pageQueue, true},
		{"narrow", 120, "plain", "left", pageLibrary, false},
		{"now playing page", 170, "plain", "left", pageNow, false},
	} {
		m := panelModel(t, c.w, 40)
		m.cfg.Look.Layout, m.cfg.Look.Menu, m.page = c.layout, c.menu, c.page
		screen := m.render()
		plain := ansi.Strip(screen)
		has := strings.Contains(plain, "Next Song 2") && strings.Contains(plain, "lyric line")
		if m.page == pageNow {
			has = m.layout().panel
		}
		if has != c.want || m.layout().panel != c.want {
			t.Errorf("%s: panel %v (layout %v), want %v\n%s", c.name, has, m.layout().panel, c.want, plain)
			continue
		}
		for i, line := range strings.Split(screen, "\n") {
			if lipgloss.Width(line) > c.w {
				t.Errorf("%s: line %d is %d wide: %q", c.name, i, lipgloss.Width(line), ansi.Strip(line))
			}
		}
		if lines := strings.Count(screen, "\n") + 1; lines != 40 {
			t.Errorf("%s: %d lines, want 40", c.name, lines)
		}
	}
	// Ayardan kapatılınca yok.
	m := panelModel(t, 170, 40)
	m.cfg.Look.Panel = "off"
	if m.layout().panel {
		t.Fatal("side_panel = off still shows the panel")
	}
	// Hiçbir şey çalmıyorsa yok.
	m = panelModel(t, 170, 40)
	m.state.Item = nil
	if m.layout().panel {
		t.Fatal("panel with nothing playing")
	}
}
