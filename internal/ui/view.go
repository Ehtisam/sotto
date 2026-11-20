package ui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Ehtisam/sotto/internal/i18n"
	"github.com/Ehtisam/sotto/internal/soloist"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

const (
	menuWidth = 20 // sol sütun
	gutter    = 3  // sütunlar arası boşluk
	margin    = 1  // sol kenar boşluğu
)

var T, F = i18n.T, i18n.F

func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.BackgroundColor = theme.Ground
	v.ForegroundColor = theme.Text
	v.WindowTitle = m.windowTitle()
	return v
}

func (m Model) windowTitle() string {
	name, artists, _ := m.describe(m.state.Item)
	switch {
	case name != "" && artists != "":
		return name + " — " + artists + " · sotto"
	case name != "":
		return name + " · sotto"
	}
	return "sotto"
}

func (m Model) render() string {
	w, h := m.width, m.height
	if w <= 0 || h <= 0 {
		return ""
	}
	rule := theme.Faint.Render(strings.Repeat("─", w))

	// Sabit satırlar: başlık, çizgi, çizgi, şerit (2), boş, kısayollar.
	bodyH := max(h-7, 1)

	return strings.Join([]string{
		m.header(w),
		rule,
		m.body(w, bodyH),
		rule,
		m.strip(w),
		"",
		m.hints(w),
	}, "\n")
}

// ---- başlık ----

func (m Model) header(w int) string {
	left := strings.Repeat(" ", margin) + theme.Brand.Render("sotto") + theme.Faint.Render("  ·  ")
	if m.me.DisplayName != "" {
		left += theme.Dim.Render(m.me.DisplayName)
	} else {
		left += theme.Dim.Render(T("tagline"))
	}
	return spread(left, m.deviceStatus()+" ", w)
}

func (m Model) deviceStatus() string {
	// Kumanda: başka bir cihazda çalıyor.
	if m.remote.active && m.remote.device != "" {
		return theme.GoldText.Render("♪ ") + theme.Plain.Render(m.remote.device) + theme.Dim.Render(T("dev.remote"))
	}
	switch m.engine {
	case engineOff:
		return theme.Dim.Render(T("dev.off"))
	case engineStarting:
		return theme.Dim.Render(T("dev.starting"))
	case engineFailed:
		return theme.Error.Render(T("dev.failed"))
	}
	if !m.haveAuth {
		return theme.Dim.Render(T("dev.connect"))
	}
	if !m.auth.LoggedIn {
		return theme.Dim.Render(T("dev.pairing"))
	}
	name := m.auth.DeviceName
	if name == "" {
		name = "sotto"
	}
	if m.auth.IsActive {
		return theme.GoldText.Render("♪ ") + theme.Plain.Render(name) + theme.Dim.Render(T("dev.light"))
	}
	return theme.Dim.Render(name + T("dev.inactive"))
}

// ---- gövde ----

func (m Model) body(w, h int) string {
	contentW := w - margin - menuWidth - gutter - 1
	var block string
	if contentW < 24 { // dar pencere: menüyü gizle
		block = lipgloss.NewStyle().MarginLeft(margin).Render(m.content(w-margin-1, h))
	} else {
		block = lipgloss.JoinHorizontal(lipgloss.Top,
			strings.Repeat(" ", margin),
			m.menu(h),
			strings.Repeat(" ", gutter),
			m.content(contentW, h),
		)
	}
	return lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).Render(block)
}

func (m Model) menu(h int) string {
	rows := []string{theme.Dim.Italic(true).Render(T("contents")), ""}
	for i, p := range pages {
		name := T(p.name)
		label := fmt.Sprintf(" %-5s%s", p.numeral, name)
		switch {
		case page(i) == m.page && m.focus == focusMenu:
			rows = append(rows, theme.SelectedRow.Width(menuWidth).Render(label))
		case page(i) == m.page:
			rows = append(rows, theme.Hi.Render(label))
		default:
			rows = append(rows, theme.Dim.Render(fmt.Sprintf(" %-5s", p.numeral))+theme.Plain.Render(name))
		}
	}
	// Çalan şarkının kapağı menünün altında; Şimdi çalıyor sayfasında zaten büyüğü var.
	const cw, ch = menuWidth - 2, (menuWidth - 2) / 2
	if url := m.nowCoverURL(); url != "" && m.page != pageNow && !m.help && h >= len(rows)+ch+3 {
		rows = append(rows, "")
		for _, l := range strings.Split(m.coverBox(url, cw, ch), "\n") {
			rows = append(rows, " "+l)
		}
		if _, _, album := m.describe(m.state.Item); album != "" {
			rows = append(rows, " "+theme.Dim.Italic(true).Render(truncate(album, cw)))
		}
	}
	return lipgloss.NewStyle().Width(menuWidth).Height(h).MaxHeight(h).Render(strings.Join(rows, "\n"))
}

func (m Model) content(w, h int) string {
	var rows []string
	switch {
	case m.actions.open:
		rows = m.actionsView(w)
	case m.help:
		rows = m.helpView(w)
	case m.page == pageNow:
		rows = m.nowPlaying(w, h)
	case m.page == pageLibrary:
		rows = m.libraryView(w, h)
	case m.page == pageSearch:
		rows = m.searchView(w)
	case m.page == pageHome:
		rows = m.homeView(w)
	case m.page == pageDevices:
		rows = m.devicesView(w)
	case m.page == pageQueue:
		rows = m.queueView(w)
	case m.page == pageProfile:
		rows = m.profileView(w)
	default:
		p := pages[m.page]
		rows = []string{
			theme.Title.Render(T(p.name)),
			"",
			theme.Dim.Render(T("page.unwritten")),
			theme.Faint.Render(T("page.coming")) + theme.Dim.Render(T(p.soon)),
		}
	}
	for i, r := range rows {
		rows[i] = ansi.Truncate(r, w, "…")
	}
	return lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).Render(strings.Join(rows, "\n"))
}

// nowPlaying, I · Şimdi çalıyor sayfası: solda büyük kapak, yanında şarkı
// bilgisi ve sıradakiler.
func (m Model) nowPlaying(w, h int) []string {
	head := []string{theme.Title.Render(T("page.now")), ""}

	name, _, _ := m.describe(m.state.Item)
	if name == "" {
		if msg := m.engineMessage(); len(msg) > 0 && !m.remote.active {
			return append(head, msg...)
		}
		return append(head,
			theme.Dim.Render(T("now.nothing")),
			theme.Faint.Render(T("now.nothing.how")),
		)
	}

	// Kapak kare: satırın iki katı sütun. Bilgiye en az 32 sütun bırak.
	ch := min(h-2, 16)
	cw := ch * 2
	if w-cw-3 < 32 {
		cw = max(w-3-32, 0)
		ch = cw / 2
		cw = ch * 2
	}
	url := m.nowCoverURL()
	if ch < 5 || url == "" {
		return append(head, m.nowInfo(w, h-2)...)
	}
	info := m.nowInfo(w-cw-3, h-2)
	block := lipgloss.JoinHorizontal(lipgloss.Top, m.coverBox(url, cw, ch), "   ", strings.Join(info, "\n"))
	return append(head, strings.Split(block, "\n")...)
}

// nowInfo, şarkı bilgisi ve sıradakiler (w genişliğinde, en çok h satır).
func (m Model) nowInfo(w, h int) []string {
	name, artists, album := m.describe(m.state.Item)
	var rows []string
	if ctx := m.state.Context; ctx != nil && ctx.Name() != "" {
		rows = append(rows, theme.Dim.Render(truncate(ctx.Name(), w-20))+theme.Faint.Render(" · "+entityKind(ctx.EntityType)))
	} else {
		rows = append(rows, "")
	}
	rows = append(rows, "")

	title := theme.Hi.Bold(true).Render(truncate(name, w-3))
	if m.state.Item.Explicit() {
		title += theme.Faint.Render("  E")
	}
	rows = append(rows, title)
	if artists != "" {
		rows = append(rows, theme.Plain.Render(truncate(artists, w)))
	}
	if album != "" {
		rows = append(rows, theme.Dim.Render(truncate(album, w)))
	}

	switch {
	case m.remote.active:
		rows = append(rows, "", theme.Faint.Render(truncate(F("now.remote", m.remote.device), w)))
	case m.soloistLive() && !m.auth.IsActive:
		rows = append(rows, "", theme.Dim.Render(truncate(T("now.inactive"), w)))
	}

	// Sözler (varsa) ya da sıradakiler; l ikisi arasında geçer.
	l, known := m.currentLyrics()
	if !m.lyr.showQueue && (l != nil || !known || m.lyr.login != "") && h-len(rows) > 4 {
		rows = append(rows, "")
		return append(rows, m.lyricsRows(w, h-len(rows))...)
	}

	if known && l == nil && m.state.Item != nil && m.lyr.login == "" {
		rows = append(rows, "", theme.Faint.Render(truncate(T("lyr.none"), w)))
	}

	// Sıradakiler.
	if len(m.queue.Upcoming) > 0 && h-len(rows) > 4 {
		rows = append(rows, "", theme.Dim.Italic(true).Render(T("now.upnext")))
		room := h - len(rows)
		shown, unnamed := 0, 0
		for _, q := range m.queue.Upcoming {
			qn, qa, _ := m.describe(q.Item)
			if qn == "" {
				unnamed++
				continue
			}
			if shown >= room-1 {
				continue
			}
			shown++
			num := theme.Faint.Render(fmt.Sprintf("%2d  ", shown))
			rows = append(rows, num+row(qn, qa, w-4, false))
		}
		if unnamed > 0 {
			rows = append(rows, theme.Faint.Render(F("now.more", unnamed)))
		}
	}
	return rows
}

// engineMessage, Soloist hazır değilse ne yapılması gerektiğini anlatır.
func (m Model) engineMessage() []string {
	switch m.engine {
	case engineOff:
		return []string{theme.Dim.Render(T("eng.off")), theme.Faint.Render(T("eng.retry"))}
	case engineStarting:
		return []string{theme.Dim.Render(T("eng.starting"))}
	case engineFailed:
		err := m.engineErr
		var rows []string
		switch {
		case errors.Is(err, soloist.ErrNotInstalled):
			rows = []string{
				theme.Plain.Render(T("eng.notfound")),
				theme.Faint.Render(T("eng.expected")) + theme.Dim.Render(m.cfg.SoloistPath()),
				theme.Faint.Render(T("eng.wizard")),
			}
		case errors.Is(err, soloist.ErrNoAPIKey):
			rows = []string{
				theme.Plain.Render(T("eng.nokey")),
				theme.Faint.Render(T("eng.savekey")) + theme.Dim.Render(m.cfg.ConfigFile()),
			}
		case errors.Is(err, soloist.ErrExpired):
			rows = []string{
				theme.Plain.Render(T("eng.expired")),
				theme.Faint.Render(T("eng.expired.how")),
			}
		case errors.Is(err, soloist.ErrAlreadyRunning):
			return []string{
				theme.Plain.Render(T("eng.busy")),
				theme.Faint.Render(T("eng.busy.how")),
			}
		case errors.Is(err, errDisconnected):
			rows = []string{theme.Plain.Render(T("eng.disconnected"))}
		default:
			msg := T("eng.unknown")
			if err != nil {
				msg = err.Error()
			}
			rows = []string{
				theme.Error.Render(T("eng.broken")),
				theme.Dim.Render(msg),
				theme.Faint.Render(T("eng.log")) + theme.Dim.Render(m.cfg.SoloistLog()),
			}
		}
		return append(rows, "", theme.Faint.Render(T("eng.retry")))
	}
	if !m.haveAuth {
		return []string{theme.Dim.Render(T("eng.connecting"))}
	}
	if !m.auth.LoggedIn {
		return []string{
			theme.Plain.Render(T("eng.pair")),
			theme.Dim.Render(T("eng.pair.how")),
			theme.Faint.Render(T("eng.pair.once")),
		}
	}
	return nil
}

// ---- alt şerit ----

func (m Model) strip(w int) string {
	it := m.state.Item
	inner := w - 2*margin

	var icon string
	switch m.state.Status {
	case soloist.StatusPlaying:
		icon = theme.GoldText.Render("▶")
	case soloist.StatusBuffering:
		icon = theme.Dim.Render("…")
	case soloist.StatusPaused:
		icon = theme.Dim.Render("❚❚")
	default:
		icon = theme.Faint.Render("·")
	}

	name, artists, _ := m.describe(it)
	var line1 string
	if name == "" {
		line1 = icon + "  " + theme.Faint.Render(T("strip.nothing"))
	} else {
		line1 = icon + "  " + theme.Title.Render(name)
		if artists != "" {
			line1 += theme.Dim.Render(" — " + artists)
		}
	}
	right := ""
	if m.flash != "" {
		style := theme.Error
		if m.flashInfo {
			style = theme.GoldText
		}
		right = style.Render(truncate(m.flash, inner*2/3))
	} else if ctx := m.state.Context; ctx != nil && ctx.Name() != "" {
		right = theme.Faint.Render(ctx.Name())
	}
	line1 = spread(ansi.Truncate(line1, inner-lipgloss.Width(right)-2, "…"), right, inner)

	// İlerleme çubuğu, süre, ses, karıştır, tekrar.
	pos, dur := m.position(), it.Duration()
	extras := theme.Faint.Render(T("strip.vol")) + theme.Plain.Render(fmt.Sprint(m.state.Volume)) + "  " + m.shuffleMark() + " " + m.repeatMark()
	left := theme.Dim.Render(clock(pos)) + " "
	rightTime := " " + theme.Dim.Render(clock(dur))
	barW := inner - lipgloss.Width(left) - lipgloss.Width(rightTime) - lipgloss.Width(extras)
	line2 := left + bar(pos, dur, max(barW, 4)) + rightTime + extras

	pad := strings.Repeat(" ", margin)
	return pad + line1 + "\n" + pad + line2
}

func (m Model) shuffleMark() string {
	if m.state.Options.Shuffle {
		return theme.Hi.Render("⇄")
	}
	return theme.Faint.Render("⇄")
}

func (m Model) repeatMark() string {
	switch m.state.Options.Repeat {
	case "context":
		return theme.Hi.Render("↻")
	case "track":
		return theme.Hi.Render("↻1")
	}
	return theme.Faint.Render("↻")
}

func bar(pos, dur time.Duration, w int) string {
	filled := 0
	if dur > 0 {
		filled = int(float64(w) * float64(pos) / float64(dur))
	}
	filled = max(0, min(w, filled))
	return theme.Hi.Render(strings.Repeat("━", filled)) + theme.Faint.Render(strings.Repeat("─", w-filled))
}

// ---- kısayollar ----

// hints, en alttaki kısayol satırı. Önemli olanlardan başlayıp sığdığı
// kadarını gösterir; tam liste her zaman ? ile açılır.
func (m Model) hints(w int) string {
	type hint struct{ key, verb string }
	var hs []hint
	if m.actions.open {
		hs = []hint{{"↑ ↓", T("hint.select")}, {"enter", T("hint.do")}, {"esc", T("hint.close")}}
	} else if m.help {
		hs = []hint{{"esc", T("hint.close")}}
	} else if m.focus == focusContent && m.page == pageSearch && m.search.typing && m.search.open == nil {
		// Yazarken kısayollar çalışmaz; sadece aramaya dair olanlar.
		hs = []hint{{T("hint.type"), T("hint.tosearch")}, {"tab", T("hint.filter")}, {"enter ↓", T("hint.results")}, {"esc", T("hint.back")}}
		parts := make([]string, len(hs))
		for i, h := range hs {
			parts[i] = theme.Dim.Render(h.key) + " " + theme.Faint.Render(h.verb)
		}
		return ansi.Truncate(strings.Repeat(" ", margin)+strings.Join(parts, theme.Faint.Render(" · ")), w, "…")
	} else if m.focus == focusContent {
		hs = []hint{{"↑ ↓", T("hint.select")}, {"enter", T("hint.play")}}
		switch {
		case m.page == pageLibrary && m.lib.open == nil:
			hs = append(hs, hint{"tab", T("hint.filter")})
		case m.page == pageProfile && m.prof.open == nil:
			hs = append(hs, hint{"tab", T("hint.nextlist")})
		}
		hs = append(hs, hint{"esc", T("hint.back")}, hint{"1–8", T("hint.pages")})
	} else {
		hs = []hint{{"↑ ↓", T("hint.page")}, {"1–8", T("hint.pages")}}
		if m.page.hasContent() {
			hs = append(hs, hint{"enter", T("hint.enter")})
		}
	}
	hs = append(hs,
		hint{T("key.space"), T("hint.pause")}, hint{"n p", T("hint.skip")},
		hint{"t", T("hint.switch")}, hint{"o", T("hint.solo")},
		hint{", .", T("hint.seek")}, hint{"+ −", T("hint.vol")},
		hint{"s", T("hint.shuf")}, hint{"r", T("hint.rep")}, hint{"q", T("hint.quit")},
	)
	render := func(h hint) string { return theme.Dim.Render(h.key) + " " + theme.Faint.Render(h.verb) }
	sep := theme.Faint.Render(" · ")
	tail := render(hint{"?", T("hint.keys")})
	room := w - 2*margin - lipgloss.Width(tail) - lipgloss.Width(sep)
	var parts []string
	used := 0
	for _, h := range hs {
		p := render(h)
		add := lipgloss.Width(p)
		if len(parts) > 0 {
			add += lipgloss.Width(sep)
		}
		if used+add > room {
			break
		}
		parts, used = append(parts, p), used+add
	}
	line := strings.Repeat(" ", margin) + strings.Join(append(parts, tail), sep)
	return ansi.Truncate(line, w, "…")
}

// helpView, ? ile açılan tam kısayol listesi.
func (m Model) helpView(w int) []string {
	keys := [][2]string{
		{"↑ ↓  j k", "help.move"}, {"enter  →", "help.open"}, {"esc  ←", "help.back"},
		{"tab", "help.tab"}, {"shift+tab", "help.backtab"}, {"1 – 8", "help.pages"}, {"", ""},
		{T("key.space"), "help.pause"}, {"n  p", "help.skip"}, {",  .", "help.seek"},
		{"+  −", "help.vol"}, {"s", "help.shuf"}, {"r", "help.rep"}, {"", ""},
		{"a", "help.actions"}, {"t", "help.switch"}, {"o", "help.solo"}, {"l", "help.lyrics"}, {"L", "help.lyrlogin"}, {"", ""},
		{"/", "help.search"}, {"?", "help.keys"}, {"q", "help.quit"},
	}
	rows := []string{theme.Title.Render(T("help.title")), ""}
	for _, k := range keys {
		if k[0] == "" {
			rows = append(rows, "")
			continue
		}
		rows = append(rows, theme.Plain.Render(fmt.Sprintf("%-12s", k[0]))+theme.Dim.Render(T(k[1])))
	}
	return rows
}

// ---- yardımcılar ----

// spread, sol ve sağ metni w genişliğinde iki uca yaslar.
func spread(left, right string, w int) string {
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return ansi.Truncate(left, w-lipgloss.Width(right)-1, "…") + " " + right
	}
	return left + strings.Repeat(" ", gap) + right
}

func truncate(s string, w int) string { return ansi.Truncate(s, max(w, 1), "…") }
func textWidth(s string) int          { return lipgloss.Width(s) }

func clock(d time.Duration) string {
	s := int(d.Seconds())
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

func entityKind(t string) string {
	switch t {
	case "playlist", "album", "artist", "show", "collection":
		return T("kind." + t)
	}
	return t
}
