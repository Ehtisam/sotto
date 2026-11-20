package ui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/audio"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// Ses görselleştirmesi (config [look] visualizer): bilgisayarın çaldığı ses
// (internal/audio, WASAPI loopback) frekans bantlarına ayrılıp Şimdi
// çalıyor'da çubuklar ("now") ya da şeritte küçük bir sıra ("strip") olarak
// çizilir. Yalnızca müzik bu bilgisayarda (Soloist) çalarken ve görünürken
// dinlenir; görünmeyince birkaç saniye sonra durur.

// vizFrame: görselleştirme açıkken ekran bu sıklıkla yenilenir.
const vizFrame = 80 * time.Millisecond

type vizState struct {
	v       *audio.Visualizer
	err     error
	tried   time.Time // son başlatma denemesi
	unused  time.Time // görünmez olduğu an (bir süre sonra kapatılır)
	ticking bool
}

type vizStartedMsg struct {
	v   *audio.Visualizer
	err error
}

type vizTickMsg struct{}

// startViz, yakalamayı başlatır (testler değiştirir).
var startViz = audio.Start

// vizWanted: görselleştirme şu an ekranda.
func (m Model) vizWanted() bool {
	mode := m.cfg.Look.Visualizer
	if mode != "now" && mode != "strip" {
		return false
	}
	if !m.soloistActive() || !m.playing() || m.miniMode() {
		return false
	}
	return mode == "strip" || (m.page == pageNow && !m.help && !m.actions.open && !m.sortm.open && !m.form.on && !m.wiz.on && !m.extraOn())
}

// vizTick, yarım saniyelik saatte: gerekiyorsa başlatır ya da kapatır.
func (m Model) vizTick() (Model, tea.Cmd) {
	z := &m.rt.viz
	want := m.vizWanted()
	switch {
	case want && z.v == nil && m.now.Sub(z.tried) > 30*time.Second:
		z.tried = m.now
		return m, func() tea.Msg {
			v, err := startViz()
			return vizStartedMsg{v, err}
		}
	case want && z.v != nil:
		z.unused = time.Time{}
		if !z.ticking {
			z.ticking = true
			return m, vizNext()
		}
	case !want && z.v != nil:
		if z.unused.IsZero() {
			z.unused = m.now
		} else if m.now.Sub(z.unused) > 10*time.Second {
			v := z.v
			z.v, z.unused = nil, time.Time{}
			go v.Close()
		}
	}
	return m, nil
}

func vizNext() tea.Cmd {
	return tea.Tick(vizFrame, func(time.Time) tea.Msg { return vizTickMsg{} })
}

func (m Model) updateViz(msg tea.Msg) (Model, tea.Cmd) {
	z := &m.rt.viz
	switch msg := msg.(type) {
	case vizStartedMsg:
		z.err = msg.err
		if msg.err != nil {
			m.rt.logs.add(logError, "visualizer: "+msg.err.Error(), m.now)
			return m, nil
		}
		if z.v != nil {
			go z.v.Close()
		}
		z.v = msg.v
		z.ticking = true
		return m, vizNext()
	case vizTickMsg:
		if z.v == nil || !m.vizWanted() {
			z.ticking = false
			return m, nil
		}
		m.now = time.Now() // ilerleme çubuğu da akıcı ilerlesin
		return m, vizNext()
	}
	return m, nil
}

func (m Model) closeViz() {
	if v := m.rt.viz.v; v != nil {
		m.rt.viz.v = nil
		v.Close()
	}
}

var vizLevels = []rune(" ▁▂▃▄▅▆▇█")

// vizBars, w genişliğinde, h satır yüksekliğinde çubuklar (Şimdi çalıyor).
func (m Model) vizBars(w, h int) []string {
	z := m.rt.viz
	if z.v == nil || w < 8 || h < 1 {
		return nil
	}
	n := w / 2 // çubuk + boşluk
	bands := z.v.Bands(n)
	rows := make([]string, h)
	for r := 0; r < h; r++ {
		var b strings.Builder
		floor := float64(h-1-r) / float64(h) // bu satırın altı
		for _, v := range bands {
			level := (v - floor) * float64(h) // bu satırda ne kadar dolu (0–1)
			i := int(level*8 + 0.5)
			i = max(0, min(8, i))
			b.WriteRune(vizLevels[i])
			b.WriteByte(' ')
		}
		style := theme.Hi
		if r >= h/2 {
			style = theme.ProgressDone
		}
		rows[r] = style.Render(strings.TrimRight(b.String(), " "))
	}
	return rows
}

// vizMini, şeritteki tek satırlık görselleştirme (n çubuk).
func (m Model) vizMini(n int) string {
	z := m.rt.viz
	if z.v == nil || m.cfg.Look.Visualizer != "strip" || !m.vizWanted() {
		return ""
	}
	var b strings.Builder
	for _, v := range z.v.Bands(n) {
		b.WriteRune(vizLevels[max(1, min(8, int(v*8+0.5)))])
	}
	return theme.Hi.Render(b.String())
}
