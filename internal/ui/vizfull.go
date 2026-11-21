package ui

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// Tam ekran görselleştirme: V açar ve kapatır; ← → ya da tab stili değiştirir,
// esc kapatır. Gövdenin bütün genişliğini alır (menü dahil), başlık, şerit ve
// kısayollar yerinde kalır. Stil [look] viz_style'da saklanır. Ses yalnız bu
// bilgisayarda (Soloist) çalan müzikten gelir; kumandada bir not gösterilir.

// VizStyles, sırasıyla dolaşılan stiller.
var VizStyles = []string{"bars", "winamp", "scope", "vu", "cassette"}

func (m Model) vizStyle() string {
	for _, s := range VizStyles {
		if s == m.cfg.Look.VizStyle {
			return s
		}
	}
	return "bars"
}

func (m Model) toggleVizFull() (Model, tea.Cmd) {
	m.vizFull = !m.vizFull
	if m.vizFull {
		m.rt.viz.unused = time.Time{}
		mm, cmd := m.vizTick() // hemen başlasın
		return mm, cmd
	}
	m.rt.gfx.drawn = nil // kapak resimleri yeniden çizilsin
	return m, m.afterResize()
}

// vizFullKey: tam ekran açıkken tuşlar. ok=false: genel kısayola düşer
// (boşluk, n, p, ses tuşları çalışmaya devam eder).
func (m Model) vizFullKey(k string) (Model, tea.Cmd, bool) {
	switch k {
	case "esc", "V", "q":
		mm, cmd := m.toggleVizFull()
		return mm, cmd, true
	case "right", "tab", "l":
		m.cfg.Look.VizStyle = cycleOf(VizStyles, m.vizStyle(), 1)
	case "left", "shift+tab", "h":
		m.cfg.Look.VizStyle = cycleOf(VizStyles, m.vizStyle(), -1)
	default:
		return m, nil, false
	}
	_ = m.cfg.Save()
	return m, nil, true
}

// vizFullView, gövdenin yerine çizilen w×h alan.
func (m Model) vizFullView(w, h int) []string {
	style := m.vizStyle()
	name, artists, _ := m.describe(m.state.Item)
	title := theme.Title.Render(T("viz.title")) + theme.Faint.Render("  ·  ") + theme.Hi.Render(T("viz.style."+style))
	sub := theme.Dim.Render(truncate(strings.TrimSpace(name+"  "+artists), w))
	z := m.rt.viz
	if z.v != nil && m.soloistActive() {
		title = spread(title, m.meterText(), w)
	}
	rows := []string{title, sub, ""}
	hint := theme.Faint.Render(truncate(T("viz.keys"), w))
	room := h - len(rows) - 2
	switch {
	case !m.soloistActive():
		return append(rows, theme.Dim.Render(truncate(T("viz.remote"), w)), "", hint)
	case z.err != nil && z.v == nil:
		return append(rows, theme.Error.Render(truncate(z.err.Error(), w)), "", hint)
	case z.v == nil:
		return append(rows, theme.Faint.Render(T("viz.starting")), "", hint)
	case room < 4 || w < 20:
		return append(rows, hint)
	}
	var art []string
	switch style {
	case "winamp":
		art = m.vizWinamp(w, room)
	case "scope":
		art = m.vizScope(w, room)
	case "vu":
		art = m.vizVU(w, room)
	case "cassette":
		art = m.vizCassette(w, room)
	default:
		art = m.vizBarsPeaks(w, room)
	}
	for len(art) < room {
		art = append(art, "")
	}
	rows = append(rows, art[:room]...)
	return append(rows, "", hint)
}

// peakHold, bantların tepelerini günceller: yükselince hemen çıkar,
// peakHoldFor boyunca asılı kalır, sonra saniyede peakFall kadar iner.
const (
	peakHoldFor = 600 * time.Millisecond
	peakFall    = 0.6
)

func (m Model) peakHold(bands []float64) []float64 {
	z := &m.rt.viz
	n := len(bands)
	if len(z.peaks) != n || len(z.held) != n {
		z.peaks, z.held = make([]float64, n), make([]time.Time, n)
	}
	dt := m.now.Sub(z.peakAt).Seconds()
	if dt < 0 || dt > 1 {
		dt = 0
	}
	z.peakAt = m.now
	for i, v := range bands {
		switch {
		case v >= z.peaks[i]:
			z.peaks[i], z.held[i] = v, m.now
		case m.now.Sub(z.held[i]) > peakHoldFor:
			z.peaks[i] = math.Max(v, z.peaks[i]-peakFall*dt)
		}
	}
	return z.peaks
}

// peakRow: tepe çizgisi h satırlık alanda hangi satırda (0 üst).
func peakRow(p float64, h int) int {
	return max(0, min(h-1, h-1-int(p*float64(h)+0.0001)))
}

// peakHoldDB: tepe değeri bu kadar süre yazılı kalır, sonra yenisine bırakır.
const peakHoldDB = 2 * time.Second

// meterText: "seviye −14.2 dB · tepe −3.1 dB". Tepe, son iki saniyenin en
// yükseği; aşmaya (0 dB) yaklaşınca kırmızı.
func (m Model) meterText() string {
	z := &m.rt.viz
	rms, peak := z.v.Meter()
	if peak >= z.peakDB || m.now.Sub(z.peakDBAt) > peakHoldDB {
		z.peakDB, z.peakDBAt = peak, m.now
	}
	db := func(v float64) string {
		if v <= -95 {
			return "−∞ dB"
		}
		return strings.Replace(fmt.Sprintf("%.1f dB", v), "-", "−", 1)
	}
	peakStyle := theme.GoldText
	if z.peakDB > -1 {
		peakStyle = theme.Error
	}
	return theme.Faint.Render(T("viz.level")+" ") + theme.Plain.Render(db(rms)) +
		theme.Faint.Render("  ·  "+T("viz.peak")+" ") + peakStyle.Render(db(z.peakDB))
}

// vizWinamp: ince çubuklar, üstlerinde asılı kalıp yavaşça düşen tepe çizgileri.
func (m Model) vizWinamp(w, h int) []string {
	z := &m.rt.viz
	n := max(1, w/3) // çubuk (2) + boşluk
	bands := z.v.BandsFast(n)
	peaks := m.peakHold(bands)
	rows := make([]string, h)
	for r := 0; r < h; r++ {
		var b strings.Builder
		level := float64(h-r) / float64(h) // bu satırın üst sınırı
		lowEdge := float64(h-1-r) / float64(h)
		for i, v := range bands {
			filled := v >= level-0.5/float64(h)
			switch {
			case r == peakRow(peaks[i], h) && peaks[i] > 0.02 && (!filled || peaks[i] > v+0.5/float64(h)):
				b.WriteString(theme.GoldText.Render("▀▀"))
			case filled:
				b.WriteString(heatStyle(lowEdge).Render("██"))
			default:
				b.WriteString("  ")
			}
			b.WriteByte(' ')
		}
		rows[r] = b.String()
	}
	return rows
}

// vizBarsPeaks: tam ekran çubuklar (yarım bloklarla ince geçişli) ve
// her çubuğun üstünde asılı kalan tepe noktası.
func (m Model) vizBarsPeaks(w, h int) []string {
	n := max(1, w/2)
	bands := m.rt.viz.v.BandsFast(n)
	peaks := m.peakHold(bands)
	rows := make([]string, h)
	for r := 0; r < h; r++ {
		var b strings.Builder
		floor := float64(h-1-r) / float64(h)
		for i, v := range bands {
			level := (v - floor) * float64(h)
			k := max(0, min(8, int(level*8+0.5)))
			if k == 0 && r == peakRow(peaks[i], h) && peaks[i] > 0.02 {
				b.WriteString(theme.GoldText.Render("▔"))
			} else if k == 0 {
				b.WriteByte(' ')
			} else {
				style := theme.Hi
				if r >= h/2 {
					style = theme.ProgressDone
				}
				b.WriteString(style.Render(string(vizLevels[k])))
			}
			b.WriteByte(' ')
		}
		rows[r] = b.String()
	}
	return rows
}

// heatStyle: aşağıdan yukarı vurgu → altın → kırmızı (Winamp'ın renk geçişi gibi).
func heatStyle(y float64) lipgloss.Style {
	switch {
	case y > 0.8:
		return theme.Error
	case y > 0.55:
		return theme.GoldText
	}
	return theme.Hi
}

// vizScope: osiloskop, Braille noktalarıyla (hücre başına 2×4 nokta).
func (m Model) vizScope(w, h int) []string {
	cols, dots := w, h*4
	wave := m.rt.viz.v.Wave(cols * 2)
	grid := make([][]rune, h)
	for i := range grid {
		grid[i] = make([]rune, cols)
		for j := range grid[i] {
			grid[i][j] = 0x2800
		}
	}
	bit := [2][4]rune{{0x01, 0x02, 0x04, 0x40}, {0x08, 0x10, 0x20, 0x80}}
	plot := func(x, y int) {
		if x < 0 || y < 0 || x >= cols*2 || y >= dots {
			return
		}
		grid[y/4][x/2] |= bit[x%2][y%4]
	}
	prev := -1
	for x, s := range wave {
		y := int((1 - (s*1.6+1)/2) * float64(dots-1)) // biraz büyüt
		y = max(0, min(dots-1, y))
		if prev >= 0 { // dikey boşlukları doldur: çizgi kopmasın
			for yy := min(prev, y); yy <= max(prev, y); yy++ {
				plot(x, yy)
			}
		}
		plot(x, y)
		prev = y
	}
	rows := make([]string, h)
	for i, line := range grid {
		rows[i] = theme.Hi.Render(string(line))
	}
	return rows
}

// vizVU: büyük yatay ses göstergesi, dB ölçeği ve tepe işaretiyle.
func (m Model) vizVU(w, h int) []string {
	z := &m.rt.viz
	level := z.v.Level()
	dt := m.now.Sub(z.peakAt).Seconds()
	if dt < 0 || dt > 1 {
		dt = 0
	}
	z.peakAt = m.now
	if len(z.peaks) != 1 {
		z.peaks = []float64{0}
	}
	z.peaks[0] = math.Max(level, z.peaks[0]-0.25*dt)
	bw := max(10, w-8)
	fill := int(level * float64(bw))
	peak := min(bw-1, int(z.peaks[0]*float64(bw)))
	var bar strings.Builder
	for i := 0; i < bw; i++ {
		y := float64(i) / float64(bw)
		switch {
		case i < fill:
			bar.WriteString(heatStyle(y).Render("█"))
		case i == peak:
			bar.WriteString(theme.GoldText.Render("▌"))
		default:
			bar.WriteString(theme.Faint.Render("░"))
		}
	}
	// Ölçek: -48 dB … 0 dB.
	scale := make([]byte, bw)
	for i := range scale {
		scale[i] = ' '
	}
	for _, db := range []int{-48, -36, -24, -12, -6, 0} {
		x := int(float64(db+48) / 48 * float64(bw-1))
		label := fmt.Sprint(db)
		for j := 0; j < len(label) && x+j < bw; j++ {
			if x+len(label) > bw {
				x = bw - len(label)
			}
			scale[x+j] = label[j]
		}
	}
	thick := max(1, min(5, (h-4)/2))
	var rows []string
	top := (h - thick - 3) / 2
	for i := 0; i < top; i++ {
		rows = append(rows, "")
	}
	rows = append(rows, "    "+theme.Dim.Render("VU"))
	for i := 0; i < thick; i++ {
		rows = append(rows, "    "+bar.String())
	}
	rows = append(rows, "    "+theme.Faint.Render(string(scale)+" dB"))
	// Tepe değeri (son iki saniyenin en yükseği) ölçeğin altında, sağda.
	label := strings.Replace(fmt.Sprintf("%s %.1f dB", T("viz.peak"), z.peakDB), "-", "−", 1)
	if z.peakDB <= -95 {
		label = T("viz.peak") + " −∞ dB"
	}
	px := max(0, bw-lipgloss.Width(label))
	rows = append(rows, "    "+strings.Repeat(" ", px)+theme.GoldText.Render(label))
	return rows
}

// vizCassette: dönen makaralı bir kaset; bant soldan sağa şarkının ilerleyişiyle
// sarılır, makaralar ses yükseldikçe hızlanır.
func (m Model) vizCassette(w, h int) []string {
	pos, dur := m.position(), m.state.Item.Duration()
	frac := 0.0
	if dur > 0 {
		frac = math.Max(0, math.Min(1, float64(pos)/float64(dur)))
	}
	level := m.rt.viz.v.Level()
	spin := []string{"|", "/", "─", "\\"}
	speed := 140 - int(level*90) // ms; yüksek seste hızlı
	f := int(m.now.UnixMilli()/int64(max(speed, 40))) % 4
	// Bant miktarı makaranın kalınlığında: sol azalır, sağ çoğalır.
	reel := func(amount float64, frame int) [3]string {
		r := int(amount*2 + 0.5) // 0–2 halka
		ring := []string{"   ", " ( ", "(( "}[r]
		ringR := []string{"   ", " ) ", " ))"}[r]
		return [3]string{
			" .---. ",
			ring[:2] + "(" + spin[frame] + ")" + ringR[1:],
			" '---' ",
		}
	}
	l, r := reel(1-frac, f), reel(frac, (f+2)%4)
	name, _, _ := m.describe(m.state.Item)
	label := truncate(name, 26)
	label += strings.Repeat(" ", max(0, 26-lipgloss.Width(label)))
	body := []string{
		"╭────────────────────────────────────╮",
		"│  ╭──────────────────────────────╮  │",
		"│  │ " + label + "   │  │",
		"│  ╰──────────────────────────────╯  │",
		"│      " + l[0] + "          " + r[0] + "      │",
		"│      " + l[1] + "  ══════  " + r[1] + "      │",
		"│      " + l[2] + "          " + r[2] + "      │",
		"│   ╭────────────────────────────╮   │",
		"╰───┴────────────────────────────┴───╯",
	}
	cw := lipgloss.Width(body[0])
	if w < cw || h < len(body) {
		return m.vizBars(w, h)
	}
	left := strings.Repeat(" ", (w-cw)/2)
	var rows []string
	for i := 0; i < (h-len(body))/2; i++ {
		rows = append(rows, "")
	}
	for i, line := range body {
		style := theme.Dim
		if i == 2 {
			style = theme.Plain
		}
		if i >= 4 && i <= 6 {
			style = theme.Hi
		}
		rows = append(rows, left+style.Render(line))
	}
	return rows
}
