package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/lucasb-eyer/go-colorful"

	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// Karşılama: Şimdi çalıyor'da hiçbir şey çalmıyorken büyük, gölgeli sotto
// yazısı (yukarıdan aşağı vurgudan altına renk geçişi), kısa tanım ve temel
// tuşlar; ortalanmış. Dar pencerede küçük yazı, çok küçükte yalnızca kısa yazı
// (nowPlaying'deki gibi).

// logoBig: gölgeli blok harfler. █ dolgu, öbürleri gölge.
var logoBig = []string{
	"███████╗ ██████╗ ████████╗████████╗ ██████╗ ",
	"██╔════╝██╔═══██╗╚══██╔══╝╚══██╔══╝██╔═══██╗",
	"███████╗██║   ██║   ██║      ██║   ██║   ██║",
	"╚════██║██║   ██║   ██║      ██║   ██║   ██║",
	"███████║╚██████╔╝   ██║      ██║   ╚██████╔╝",
	"╚══════╝ ╚═════╝    ╚═╝      ╚═╝    ╚═════╝ ",
}

var logoGlyphs = map[rune][3]string{
	's': {"▄▀▀▀", " ▀▀▄", "▀▀▀ "},
	'o': {"▄▀▀▄", "█  █", "▀▄▄▀"},
	't': {"▀█▀", " █ ", " █ "},
}

// logoSmall, "sotto"nun üç satırlık küçük yazısı (dar pencere).
func logoSmall() []string {
	var rows [3]string
	for i, r := range "sotto" {
		g := logoGlyphs[r]
		for j := range rows {
			if i > 0 {
				rows[j] += " "
			}
			rows[j] += g[j]
		}
	}
	return rows[:]
}

// logoColors: satır başına renk, vurgudan açık vurguya ve altına geçiş.
// Terminalin kendi renkleri seçiliyse (hex yok) temanın stilleri kullanılır.
func (m Model) logoColors(n int) []lipgloss.Style {
	p := theme.Resolve(m.cfg.Look.Theme, m.cfg.Look.Accent)
	a, errA := colorful.Hex(p.Accent)
	b, errB := colorful.Hex(p.AccentHi)
	c, errC := colorful.Hex(p.Gold)
	out := make([]lipgloss.Style, n)
	for i := range out {
		if errA != nil || errB != nil || errC != nil {
			out[i] = []lipgloss.Style{theme.Hi, theme.Hi, theme.Brand, theme.Brand, theme.GoldText, theme.GoldText}[min(i*6/max(n, 1), 5)]
			continue
		}
		t := float64(i) / float64(max(n-1, 1))
		var col colorful.Color
		if t < 0.5 {
			col = b.BlendLab(a, t*2)
		} else {
			col = a.BlendLab(c, (t-0.5)*2)
		}
		out[i] = lipgloss.NewStyle().Foreground(lipgloss.Color(col.Clamped().Hex()))
	}
	return out
}

// paintLogo: dolgu renkli, gölge soluk.
func (m Model) paintLogo(lines []string) []string {
	colors := m.logoColors(len(lines))
	out := make([]string, len(lines))
	for i, l := range lines {
		var b strings.Builder
		run, fill := "", false
		flush := func() {
			if run == "" {
				return
			}
			if fill {
				b.WriteString(colors[i].Render(run))
			} else {
				b.WriteString(theme.Faint.Render(run))
			}
			run = ""
		}
		for _, r := range l {
			f := r == '█' || r == '▀' || r == '▄'
			if r == ' ' {
				flush()
				b.WriteRune(r)
				continue
			}
			if f != fill {
				flush()
				fill = f
			}
			run += string(r)
		}
		flush()
		out[i] = b.String()
	}
	return out
}

// welcomeFits: karşılama ekranı bu alana sığar mı.
func welcomeFits(w, h int) bool { return w >= 40 && h >= 16 }

func (m Model) welcomeView(w, h int) []string {
	logo := logoBig
	if w < lipgloss.Width(logoBig[0])+4 || h < 22 {
		logo = logoSmall()
	}
	// Her grup kendi içinde ortalanır (logo, tanım, not, tuşlar, kaldığın yer).
	var groups [][]string
	groups = append(groups, m.paintLogo(logo), []string{""}, []string{theme.Dim.Italic(true).Render(T("welcome.tag"))}, []string{""})
	var block []string

	if msg := m.engineMessage(); len(msg) > 0 && !m.remote.active {
		for _, l := range msg {
			groups = append(groups, []string{l})
		}
	} else {
		groups = append(groups, []string{theme.Plain.Render(T("now.nothing")) + " " + theme.Faint.Render(T("now.nothing.how"))})
	}
	groups = append(groups, []string{""})

	type item struct{ key, verb string }
	items := []item{
		{m.kn("play_pause"), T("welcome.play")},
		{m.kn("search"), T("welcome.search")},
		{"1–8", T("welcome.pages")},
		{m.kn("device_picker"), T("welcome.devices")},
		{m.kn("switch_speaker"), T("hint.switch")},
		{m.kn("soloist"), T("hint.solo")},
		{m.kn("lyrics"), T("welcome.lyrics")},
		{m.kn("help"), T("welcome.keys")},
	}
	keyW, verbW := 0, 0
	for _, it := range items {
		keyW = max(keyW, lipgloss.Width(it.key))
		verbW = max(verbW, lipgloss.Width(it.verb))
	}
	cell := func(it item, vw int) string {
		return theme.Hi.Render(it.key) + strings.Repeat(" ", keyW-lipgloss.Width(it.key)+2) +
			theme.Dim.Render(truncate(it.verb, vw))
	}
	col := keyW + 2 + verbW
	if w >= 2*col+6 { // iki sütun
		for i := 0; i < len(items); i += 2 {
			left := cell(items[i], verbW)
			line := left + strings.Repeat(" ", max(0, col+6-lipgloss.Width(left)))
			if i+1 < len(items) {
				line += cell(items[i+1], verbW)
			}
			block = append(block, line)
		}
	} else {
		for _, it := range items {
			block = append(block, cell(it, max(8, w-keyW-4)))
		}
	}

	groups = append(groups, block)
	if l := m.rt.ev.last; l.Track != "" && l.Title != "" && m.spState == spReady {
		what := l.Title
		if l.Artist != "" {
			what += " — " + l.Artist
		}
		groups = append(groups, []string{""}, []string{theme.Faint.Render(T("now.resume")) + theme.Plain.Render(what)})
	}

	// Ortala: her grup kendi en geniş satırına göre yatayda; hepsi dikeyde
	// üstte üçte bir boşlukla.
	var lines []string
	for _, g := range groups {
		gw := 0
		for _, l := range g {
			gw = max(gw, lipgloss.Width(l))
		}
		left := strings.Repeat(" ", max(0, (w-min(gw, w))/2))
		for _, l := range g {
			lines = append(lines, truncate(left+l, w))
		}
	}
	var rows []string
	for i := 0; i < max(0, (h-len(lines))/3); i++ {
		rows = append(rows, "")
	}
	rows = append(rows, lines...)
	if len(rows) > h {
		rows = rows[:h]
	}
	return rows
}
