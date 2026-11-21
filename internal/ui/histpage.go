package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/history"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// Dinleme geçmişi ve istatistik (g y): yerel kayıttan (history.jsonl) özet,
// saatlere ve günlere göre dağılım, en çok dinlenen şarkılar ve sanatçılar,
// son dinlemeler. tab aralığı değiştirir (hafta, ay, yıl, hepsi); enter
// şarkıyı çalar ya da sanatçıyı açar; R dosyayı yeniden okur.

type histState struct {
	rng   int
	plays []history.Play
	stats history.Stats
	err   error
}

const histTop = 15

func (m Model) openHistory() (tea.Model, tea.Cmd) {
	rng := m.extra.rng
	m.extra = extraState{page: extraHistory, host: m.page, rng: rng, hist: &histState{rng: 0}}
	m.focus = focusContent
	m.loadHistory()
	m.rebuildExtra()
	return m, nil
}

// loadHistory, kaydı okuyup seçili aralığın özetini çıkarır.
func (m Model) loadHistory() {
	h := m.extra.hist
	if h == nil {
		return
	}
	h.plays, h.err = history.Load(m.cfg.LocalDir)
	h.stats = history.Compute(h.plays, history.Ranges[h.rng], m.now, histTop)
}

func (m Model) switchHistRange(d int) (Model, tea.Cmd) {
	h := m.extra.hist
	h.rng = cycle(h.rng, d, len(history.Ranges))
	h.stats = history.Compute(h.plays, history.Ranges[h.rng], m.now, histTop)
	m.extra.cursor, m.extra.scroll = 0, 0
	m.rebuildExtra()
	return m, nil
}

func histDuration(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	h, mi := int(d.Hours()), int(d.Minutes())%60
	if h > 0 {
		return F("hist.hm", h, mi)
	}
	return F("hist.m", mi)
}

func (m Model) histHeader(w int) []string {
	h := m.extra.hist
	rng := T("hist.range." + string(history.Ranges[h.rng]))
	tabs := make([]string, len(history.Ranges))
	for i, r := range history.Ranges {
		label := T("hist.range." + string(r))
		if i == h.rng {
			tabs[i] = theme.Hi.Render(label)
		} else {
			tabs[i] = theme.Faint.Render(label)
		}
	}
	s := h.stats
	summary := theme.Plain.Render(F("hist.plays", s.Plays)) + theme.Faint.Render("  ·  ") +
		theme.Plain.Render(histDuration(s.MS)) + theme.Faint.Render("  ·  ") +
		theme.Dim.Render(F("hist.counts", s.Songs, s.Artists, s.Days))
	_ = rng
	return []string{
		theme.Title.Render(T("hist.title")),
		truncate(strings.Join(tabs, theme.Faint.Render(" · "))+theme.Faint.Render("   tab"), w),
		truncate(summary, w),
		"",
	}
}

func (m Model) histItems() []searchItem {
	h := m.extra.hist
	if h == nil {
		return nil
	}
	s := h.stats
	if s.Plays == 0 {
		return nil
	}
	var out []searchItem
	space := func() { out = append(out, searchItem{kind: "space"}) }
	text := func(t string) { out = append(out, searchItem{kind: "text", name: t}) }

	// Saatlere göre: üç satırlık çubuklar, altında saatler.
	out = append(out, searchItem{kind: "header", section: "hist.byhour"})
	peak := 1
	for _, n := range s.ByHour {
		peak = max(peak, n)
	}
	const rows = 3
	for r := 0; r < rows; r++ {
		var b strings.Builder
		floor := float64(rows-1-r) / rows
		for _, n := range s.ByHour {
			v := float64(n) / float64(peak)
			k := max(0, min(8, int((v-floor)*rows*8+0.5)))
			ch := string(vizLevels[k])
			b.WriteString(ch + ch + " ")
		}
		text(b.String())
	}
	axis := ""
	for hr := 0; hr < 24; hr += 6 {
		axis += fmt.Sprintf("%-18d", hr)
	}
	text(strings.TrimRight(axis, " "))
	space()

	// Günlere göre.
	out = append(out, searchItem{kind: "header", section: "hist.byday"})
	dpeak := 1
	for _, n := range s.ByDay {
		dpeak = max(dpeak, n)
	}
	for d, n := range s.ByDay {
		bar := strings.Repeat("█", n*24/dpeak)
		if n > 0 && bar == "" {
			bar = "▏"
		}
		text(fmt.Sprintf("%-4s %-24s %d", T(fmt.Sprintf("day.%d", d)), bar, n))
	}
	space()

	// En çok dinlenen şarkılar.
	out = append(out, searchItem{kind: "header", section: "hist.topsongs"})
	for i, c := range s.Top {
		out = append(out, searchItem{kind: "track", uri: c.Key, name: fmt.Sprintf("%2d  %s", i+1, c.Name),
			sub: c.Sub + "  " + F("hist.n", c.Plays)})
	}
	space()
	out = append(out, searchItem{kind: "header", section: "hist.topartists"})
	for i, c := range s.TopArt {
		it := searchItem{kind: "text", name: fmt.Sprintf("%2d  %s", i+1, c.Name), sub: F("hist.n", c.Plays) + "  " + histDuration(c.MS)}
		if strings.HasPrefix(c.Key, "spotify:artist:") {
			it.kind, it.uri = "artist", c.Key
		}
		out = append(out, it)
	}
	space()

	// Son dinlemeler, günlere bölünmüş (en çok 100).
	out = append(out, searchItem{kind: "header", section: "hist.recent"})
	day := ""
	for i, p := range s.Recent {
		if i == 100 {
			break
		}
		if d := dayName(m.now, p.At); d != day {
			out = append(out, searchItem{kind: "header", name: d})
			day = d
		}
		out = append(out, searchItem{kind: "track", uri: p.URI, name: p.Title, ctx: p.Context, history: true,
			sub: p.Artist + "  " + p.At.Local().Format("15:04")})
	}
	return out
}
