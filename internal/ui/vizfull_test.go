package ui

import (
	"math"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Ehtisam/sotto/internal/audio"
	"github.com/Ehtisam/sotto/internal/soloist"
)

// Her stil sahte bir sesle çizilir: boş değil, alan dışına taşmıyor; ekranın
// tamamı pencereye sığıyor ve menü görünmüyor.
func TestVizFullStylesDraw(t *testing.T) {
	m := settingsModel(t)
	m.width, m.height = 110, 32
	var s []float64
	for i := 0; i < 4096; i++ {
		s = append(s, 0.4*math.Sin(2*math.Pi*220*float64(i)/48000)+0.2*math.Sin(2*math.Pi*3000*float64(i)/48000))
	}
	m.rt.viz.v = audio.Fake(s, 48000)
	m.rt.proc, m.rt.client, m.engine = &soloist.Process{}, &soloist.Client{}, engineReady
	m.auth = soloist.AuthState{LoggedIn: true, IsActive: true}
	m.state.Status = soloist.StatusPlaying
	it := &soloist.Entity{URI: "spotify:track:t1", EntityType: "track"}
	it.Decorations.Identity.Name = "Song 01"
	it.Decorations.Playback.DurationMS = 200000
	m.state.Item = it
	m.now = time.Now()
	m.vizFull = true
	for _, style := range VizStyles {
		m.cfg.Look.VizStyle = style
		rows := m.vizFullView(100, 24)
		if head := ansi.Strip(rows[0]); !strings.Contains(head, "peak −") || !strings.Contains(head, "level −") {
			t.Fatalf("%s: no level/peak values in the title: %q", style, head)
		}
		if len(rows) > 24 {
			t.Fatalf("%s: %d rows > 24", style, len(rows))
		}
		ink := 0
		for _, r := range rows {
			if lipgloss.Width(r) > 100 {
				t.Fatalf("%s: row too wide (%d): %q", style, lipgloss.Width(r), ansi.Strip(r))
			}
			ink += len(strings.TrimSpace(strings.Trim(ansi.Strip(r), "\u2800")))
		}
		if ink < 40 {
			t.Fatalf("%s: almost nothing drawn:\n%s", style, ansi.Strip(strings.Join(rows, "\n")))
		}
		screen := m.render()
		if strings.Contains(screen, "VIII Settings") {
			t.Fatalf("%s: menu should give way to the visualizer", style)
		}
		for _, l := range strings.Split(screen, "\n") {
			if lipgloss.Width(l) > 110 {
				t.Fatalf("%s: screen line too wide", style)
			}
		}
		t.Logf("%s:\n%s", style, ansi.Strip(strings.Join(rows, "\n")))
	}
}

// Tepeler: ses kesilince çubuklar hızla iner, tepe çizgileri bir süre
// yukarıda asılı kalır (eskiden çubukla aynı hücrede kalıp görünmüyordu).
func TestVizPeaksHang(t *testing.T) {
	m := settingsModel(t)
	var loud []float64
	for i := 0; i < 4096; i++ {
		loud = append(loud, 0.8*math.Sin(2*math.Pi*300*float64(i)/48000)+0.5*math.Sin(2*math.Pi*2500*float64(i)/48000))
	}
	v := audio.Fake(loud, 48000)
	m.rt.viz.v = v
	m.now = time.Now()
	for _, style := range []string{"winamp", "bars"} {
		m.rt.viz.peaks, m.rt.viz.held = nil, nil
		draw := func() string {
			if style == "winamp" {
				return ansi.Strip(strings.Join(m.vizWinamp(90, 16), "\n"))
			}
			return ansi.Strip(strings.Join(m.vizBarsPeaks(90, 16), "\n"))
		}
		v.Push(loud, 48000)
		draw()
		v.Push(make([]float64, 4096), 48000) // sessizlik
		for i := 0; i < 4; i++ {
			m.now = m.now.Add(80 * time.Millisecond)
			draw()
		}
		out := draw()
		mark := map[string]string{"winamp": "▀▀", "bars": "▔"}[style]
		if !strings.Contains(out, mark) {
			t.Fatalf("%s: no hanging peaks after the sound stopped:\n%s", style, out)
		}
		// Bir süre sonra tepeler de iner.
		for i := 0; i < 40; i++ {
			m.now = m.now.Add(80 * time.Millisecond)
			out = draw()
		}
		if strings.Count(out, mark) > 3 {
			t.Fatalf("%s: peaks never fall:\n%s", style, out)
		}
	}
}
