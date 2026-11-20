package ui

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/Ehtisam/sotto/internal/audio"
	"github.com/Ehtisam/sotto/internal/soloist"
)

// soloistPlaying: Soloist bağlı, etkin ve çalıyor (gerçek süreç yok).
func soloistPlaying(t *testing.T) Model {
	_, m := playingModel(t)
	m.remote.active = false
	m.rt.client = &soloist.Client{}
	m.engine = engineReady
	m.auth = soloist.AuthState{LoggedIn: true, IsActive: true}
	m.width, m.height = 120, 40
	return m
}

func TestVisualizerStartsOnlyWhenSeen(t *testing.T) {
	started := 0
	old := startViz
	startViz = func() (*audio.Visualizer, error) { started++; return nil, errors.New("no device") }
	t.Cleanup(func() { startViz = old })

	m := soloistPlaying(t)
	m.cfg.Look.Visualizer = "now"
	m.page = pageLibrary
	m = tickOnce(m)
	if started != 0 {
		t.Fatal("started while not on Now playing")
	}
	m.page = pageNow
	m = tickOnce(m)
	if started != 1 || m.rt.viz.err == nil {
		t.Fatalf("started %d err %v", started, m.rt.viz.err)
	}
	// Hata sonrası hemen yeniden denemez.
	m = tickOnce(m)
	if started != 1 {
		t.Fatal("retried at once after a failure")
	}
	// Uzaktaki cihazda çalarken (telefon) dinlemez.
	m.auth.IsActive = false
	if m.vizWanted() {
		t.Fatal("wanted while playing elsewhere")
	}
	m.cfg.Look.Visualizer = "off"
	m.auth.IsActive = true
	if m.vizWanted() {
		t.Fatal("wanted while off")
	}
}

func TestVisualizerBarsDraw(t *testing.T) {
	m := soloistPlaying(t)
	m.cfg.Look.Visualizer = "now"
	m.page = pageNow
	v := audioWithTone(t)
	m.rt.viz.v = v
	rows := m.vizBars(40, 4)
	if len(rows) != 4 {
		t.Fatalf("rows %d", len(rows))
	}
	joined := ansi.Strip(strings.Join(rows, "\n"))
	if !strings.ContainsAny(joined, "▁▂▃▄▅▆▇█") {
		t.Fatalf("no bars:\n%s", joined)
	}
	if !strings.ContainsAny(ansi.Strip(strings.Join(m.nowInfo(80, 34), "\n")), "█▇▆") {
		t.Fatal("bars missing from Now playing")
	}
	m.cfg.Look.Visualizer = "strip"
	if ansi.Strip(m.vizMini(16)) == "" || !strings.ContainsAny(ansi.Strip(m.strip(120)), "▁▂▃▄▅▆▇█") {
		t.Fatal("mini visualizer missing from the strip")
	}
}

func audioWithTone(t *testing.T) *audio.Visualizer {
	s := make([]float64, 4096)
	for i := range s {
		s[i] = 0.6*math.Sin(2*math.Pi*220*float64(i)/48000) + 0.3*math.Sin(2*math.Pi*3000*float64(i)/48000)
	}
	return audio.Fake(s, 48000)
}

// tickOnce, saatin bir vuruşunu işler (settle saat mesajlarını atlar).
func tickOnce(m Model) Model {
	out, cmd := m.Update(tickMsg(time.Now()))
	return settle(out.(Model), cmd)
}
