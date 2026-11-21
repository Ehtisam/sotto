package ui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Ehtisam/sotto/internal/cli"
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
	v.ReportFocus = true // bildirimler yalnızca pencere arkadayken (ayara göre)
	if m.cfg.Look.Mouse {
		v.MouseMode = tea.MouseModeCellMotion // mouse.go
	}
	v.BackgroundColor = theme.Ground
	v.ForegroundColor = theme.Text
	v.WindowTitle = m.windowTitleText()
	// Terminalden tuşların basılma/bırakılma bilgisini iste (kitty klavye
	// protokolü): böylece shift'e tek başına basıldığı duyulur ve alt satır
	// shift'le çalışan tuşları gösterir. Desteklemeyen terminal yok sayar.
	v.KeyboardEnhancements.ReportEventTypes = true
	v.KeyboardEnhancements.ReportAllKeysAsEscapeCodes = true
	v.KeyboardEnhancements.ReportAssociatedText = true
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
	m.rt.gfx.slots = nil // coverBox resim alanlarını yeniden ayırır
	if m.miniMode() {    // pencere çok küçük: yalnızca çalan şarkı (mini.go)
		screen := m.miniView()
		m.rt.gfx.placed = findPlaces(screen, m.rt.gfx.slots)
		m.rt.out.setAfter(m.stripsAfter(screen))
		return screen
	}
	l := m.layout()
	var parts []string
	if l.boxed {
		// Kutulu: başlık, gövde kutuları, şerit kutusu, kısayollar.
		strip := strings.Join(theme.Box(strings.Split(m.strip(w-2), "\n"), w, 4, "", false), "\n")
		m.rt.bar.x++ // şerit kutunun içinde bir sütun sağda
		body := m.bodyBoxed(w, l)
		parts = []string{m.header(w), body, strip, m.hints(w)}
		if m.stripTop() {
			parts = []string{m.header(w), strip, body, m.hints(w)}
		}
	} else {
		// Sabit satırlar: başlık, çizgi, çizgi, şerit (2), boş, kısayollar.
		rule := theme.Rule(w)
		parts = []string{m.header(w), rule, m.body(w, l), rule, m.strip(w), "", m.hints(w)}
		if m.stripTop() { // şerit üstte: başlığın altında
			parts = []string{m.header(w), rule, m.strip(w), rule, m.body(w, l), "", m.hints(w)}
		}
	}
	screen := strings.Join(parts, "\n")
	m.rt.gfx.placed = findPlaces(screen, m.rt.gfx.slots)
	m.rt.out.setAfter(m.stripsAfter(screen))
	return screen
}

// ---- başlık ----

func (m Model) header(w int) string {
	left := strings.Repeat(" ", margin) + theme.Brand.Render("sotto") + theme.Faint.Render("  ·  ")
	if m.me.DisplayName != "" {
		left += theme.Dim.Render(m.me.DisplayName)
	} else {
		left += theme.Dim.Render(T("tagline"))
	}
	return spread(left, m.privateMark()+m.deviceStatus()+" ", w)
}

func (m Model) deviceStatus() string {
	// Kumanda: başka bir cihazda çalıyor.
	if m.remote.active && m.remote.device != "" {
		return theme.Playing.Render(theme.Icon.Note+" ") + theme.Plain.Render(m.remote.device) + theme.Dim.Render(T("dev.remote"))
	}
	if s := m.switchingText(); s != "" {
		return theme.GoldText.Render(s)
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
		return theme.Playing.Render(theme.Icon.Note+" ") + theme.Plain.Render(name) + theme.Dim.Render(T("dev.light"))
	}
	return theme.Dim.Render(name + T("dev.inactive"))
}

// ---- gövde ----

func (m Model) body(w int, l layout) string {
	h := l.bodyH
	var block string
	switch {
	case !l.menu: // dar pencere ya da menü gizli
		block = lipgloss.NewStyle().MarginLeft(margin).Render(m.content(l.contentW, h))
	case l.menuRight:
		block = lipgloss.JoinHorizontal(lipgloss.Top,
			strings.Repeat(" ", margin),
			lipgloss.NewStyle().Width(l.contentW).Render(m.content(l.contentW, h)),
			strings.Repeat(" ", gutter),
			m.menu(h),
		)
	default:
		block = lipgloss.JoinHorizontal(lipgloss.Top,
			strings.Repeat(" ", margin),
			m.menu(h),
			strings.Repeat(" ", gutter),
			m.content(l.contentW, h),
		)
	}
	return lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).Render(block)
}

// bodyBoxed: gövde kutuları. Menü ve içerik ayrı kutularda; odaktaki vurgulu.
// İçerik ve menü sade düzendeki sütunlarında kalır (kutu kenarları boşluklara düşer).
func (m Model) bodyBoxed(w int, l layout) string {
	h := l.bodyH
	padded := func(s string) []string {
		lines := strings.Split(s, "\n")
		for i := range lines {
			lines[i] = " " + lines[i]
		}
		return lines
	}
	contentBox := theme.Box(padded(m.content(l.contentW, h)), l.contentW+4, h+2, "", m.focus == focusContent)
	var rows []string
	if !l.menu {
		rows = contentBox
	} else {
		menuBox := theme.Box(padded(m.menu(h)), menuWidth+4, h+2, "", m.focus == focusMenu)
		left, right := menuBox, contentBox
		if l.menuRight {
			left, right = contentBox, menuBox
		}
		for i := range left {
			rows = append(rows, left[i]+" "+right[i])
		}
	}
	return lipgloss.NewStyle().Width(w).MaxHeight(h + 2).Render(strings.Join(rows, "\n"))
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
	// Yer azsa kapak küçülür (en çok 18×9, en az 8×4); albüm adı sığarsa altında.
	ch := menuCoverRows(h, len(rows))
	if url := m.nowCoverURL(); url != "" && m.page != pageNow && !m.help && ch > 0 {
		rows = append(rows, "")
		for _, l := range strings.Split(m.coverBox(url, ch*2, ch), "\n") {
			rows = append(rows, " "+l)
		}
		if _, _, album := m.describe(m.state.Item); album != "" && len(rows) < h {
			rows = append(rows, " "+theme.Dim.Italic(true).Render(truncate(album, menuWidth-2)))
		}
	}
	return lipgloss.NewStyle().Width(menuWidth).Height(h).MaxHeight(h).Render(strings.Join(rows, "\n"))
}

// menuCoverRows, menünün altındaki kapağın satır sayısı: used satırdan sonra
// kalan yere sığan en büyüğü (en çok 9), 4'ten azsa 0 (kapak yok).
func menuCoverRows(h, used int) int {
	ch := min((menuWidth-2)/2, h-used-1)
	if ch < 4 {
		return 0
	}
	return ch
}

func (m Model) content(w, h int) string {
	var rows []string
	switch {
	case m.wiz.on:
		rows = m.wizardView(w)
	case m.form.on:
		rows = m.playlistFormView(w)
	case m.actions.open:
		rows = m.actionsView(w)
	case m.sortm.open:
		rows = m.sortView(w)
	case m.pick.open:
		rows = m.devPickView(w)
	case m.vizFull && !m.help:
		rows = m.vizFullView(w, h)
	case m.help:
		rows = m.helpView(w, h)
	case m.extraOn():
		rows = m.extraView(w)
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
	case m.page == pageSettings:
		rows = m.settingsView(w, h)
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
	if name == "" && m.state.Item == nil && welcomeFits(w, h-len(head)) {
		return append(head, m.welcomeView(w, h-len(head))...)
	}
	if name == "" {
		if msg := m.engineMessage(); len(msg) > 0 && !m.remote.active {
			return append(head, msg...)
		}
		rows := append(head,
			theme.Dim.Render(T("now.nothing")),
			theme.Faint.Render(T("now.nothing.how")),
		)
		if l := m.rt.ev.last; l.Track != "" && l.Title != "" && m.spState == spReady {
			what := l.Title
			if l.Artist != "" {
				what += " — " + l.Artist
			}
			rows = append(rows, "", theme.Faint.Render(T("now.resume"))+theme.Plain.Render(what))
		}
		return rows
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

	// Ses görselleştirmesi (Soloist çalarken).
	if m.cfg.Look.Visualizer == "now" && m.vizWanted() {
		vh := 0
		switch {
		case h-len(rows) > 14:
			vh = 4
		case h-len(rows) > 9:
			vh = 2
		}
		if bars := m.vizBars(min(w, 64), vh); len(bars) > 0 {
			rows = append(rows, "")
			rows = append(rows, bars...)
		}
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
	// Spotify'daki gibi düğme: basınca ne olacağını gösterir (çalarken duraklat).
	case soloist.StatusPlaying:
		icon = theme.Playing.Render(theme.Icon.Pause)
	case soloist.StatusBuffering:
		icon = theme.Dim.Render(theme.Icon.Buffering)
	case soloist.StatusPaused:
		icon = theme.Dim.Render(theme.Icon.Play)
	default:
		icon = theme.Faint.Render(theme.Icon.Idle)
	}

	name, _, _ := m.describe(it)
	var btns []btnZone
	lead := icon
	if m.cfg.Look.Buttons {
		// Önceki · çal/durdur · sonraki; yerleri fare için saklanır.
		x := margin
		add := func(s string, action string, row int) string {
			btns = append(btns, btnZone{x: x, w: lipgloss.Width(s), row: row, action: action})
			x += lipgloss.Width(s) + 2
			return s
		}
		lead = add(theme.Dim.Render(theme.Icon.Prev), "previous", 0) + "  " +
			add(icon, "play_pause", 0) + "  " +
			add(theme.Dim.Render(theme.Icon.Next), "next", 0)
		lead += " "
	}
	var line1 string
	if name == "" {
		line1 = lead + "  " + theme.Faint.Render(T("strip.nothing"))
	} else {
		line1 = lead + "  " + m.stripLine()
	}
	right := ""
	if s := m.switchingText(); s != "" && (m.flash == "" || m.flashInfo) {
		right = theme.GoldText.Render(truncate(s, inner*2/3)) // hata notu öne geçer
	} else if m.flash != "" {
		style := theme.Error
		if m.flashInfo {
			style = theme.GoldText
		}
		right = style.Render(truncate(m.flash, inner*2/3))
	} else if v := m.vizMini(16); v != "" {
		right = v
	} else if ctx := m.state.Context; ctx != nil && ctx.Name() != "" {
		right = theme.Faint.Render(ctx.Name())
	}
	line1 = spread(ansi.Truncate(line1, inner-lipgloss.Width(right)-2, "…"), right, inner)

	// İlerleme çubuğu, süre, ses, karıştır, tekrar.
	pos, dur := m.position(), it.Duration()
	vol := theme.Plain.Render(fmt.Sprint(m.state.Volume))
	if m.muted() {
		vol = theme.Dim.Render(theme.Icon.Muted + " " + T("strip.muted"))
	}
	if m.remote.active && cli.Disallowed(m.remote.player, "volume", "") {
		vol = theme.Faint.Render(fmt.Sprint(m.state.Volume)) // cihaz uzaktan ses değiştirtmiyor
	}
	extras := theme.Faint.Render(T("strip.vol")) + vol + "  " + m.shuffleMark() + " " + m.repeatMark()
	if s := m.sleepMark(); s != "" {
		extras += "  " + s
	}
	left := theme.Dim.Render(clock(pos)) + " "
	rightTime := " " + theme.Dim.Render(clock(dur))
	barW := inner - lipgloss.Width(left) - lipgloss.Width(rightTime) - lipgloss.Width(extras)
	m.rt.bar = barZone{x: margin + lipgloss.Width(left), w: max(barW, 4)}
	line2 := left + bar(pos, dur, max(barW, 4)) + rightTime + extras
	if m.cfg.Look.Buttons {
		x := margin + lipgloss.Width(left) + max(barW, 4) + lipgloss.Width(rightTime) +
			lipgloss.Width(theme.Faint.Render(T("strip.vol"))+vol+"  ")
		sw := lipgloss.Width(m.shuffleMark())
		btns = append(btns, btnZone{x: x, w: sw, row: 1, action: "shuffle"},
			btnZone{x: x + sw + 1, w: lipgloss.Width(m.repeatMark()), row: 1, action: "repeat"})
	}
	m.rt.btns = btns

	pad := strings.Repeat(" ", margin)
	return pad + line1 + "\n" + pad + line2
}

func (m Model) shuffleMark() string {
	if m.state.Options.Shuffle {
		return theme.Hi.Render(theme.Icon.Shuffle) + m.smartMark()
	}
	return theme.Faint.Render(theme.Icon.Shuffle)
}

func (m Model) repeatMark() string {
	switch m.state.Options.Repeat {
	case "context":
		return theme.Hi.Render(theme.Icon.Repeat)
	case "track":
		return theme.Hi.Render(theme.Icon.RepeatOne)
	}
	return theme.Faint.Render(theme.Icon.Repeat)
}

func bar(pos, dur time.Duration, w int) string {
	filled := 0
	if dur > 0 {
		filled = int(float64(w) * float64(pos) / float64(dur))
	}
	return theme.Bar(filled, w)
}

// ---- kısayollar ----

// hints, en alttaki kısayol satırı. Önemli olanlardan başlayıp sığdığı
// kadarını gösterir; tam liste her zaman ? ile açılır.
// hint, alt satırdaki bir kısayol: tuş ve ne yaptığı.
type hint struct{ key, verb string }

func (m Model) hints(w int) string {
	var hs []hint
	if m.shiftHeld() {
		r := func(h hint) string { return theme.Hi.Render(h.key) + " " + theme.Dim.Render(h.verb) }
		if row := m.shiftHintRow(r); row != "" {
			return ansi.Truncate(strings.Repeat(" ", margin)+row, w, "…")
		}
	}
	if m.wiz.on {
		parts := []string{}
		for _, h := range m.wizardHints() {
			parts = append(parts, theme.Dim.Render(h.key)+" "+theme.Faint.Render(h.verb))
		}
		return ansi.Truncate(strings.Repeat(" ", margin)+strings.Join(parts, theme.Faint.Render(" · ")), w, "…")
	}
	if m.form.on {
		// Formda harfler yazıya gider; çalma kısayolları burada çalışmaz.
		hs = []hint{{"↑ ↓", T("hint.select")}, {T("hint.type"), T("hint.edit")}, {"← →", T("hint.change")}, {"ctrl+s", T("pl.save")}, {"esc", T("hint.close")}}
		parts := make([]string, len(hs))
		for i, h := range hs {
			parts[i] = theme.HintKey.Render(h.key) + " " + theme.HintText.Render(h.verb)
		}
		return ansi.Truncate(strings.Repeat(" ", margin)+strings.Join(parts, theme.Faint.Render(" · ")), w, "…")
	}
	if f := m.shownFinder(); f != nil && f.typing && !m.actions.open && !m.sortm.open && !m.help {
		// Sayfa içi aramaya yazarken.
		hs = []hint{{T("hint.type"), T("find.hint")}, {"enter ↓", T("hint.results")}, {"ctrl+u", T("find.clear")}, {"esc", T("hint.close")}}
		parts := make([]string, len(hs))
		for i, h := range hs {
			parts[i] = theme.HintKey.Render(h.key) + " " + theme.HintText.Render(h.verb)
		}
		return ansi.Truncate(strings.Repeat(" ", margin)+strings.Join(parts, theme.Faint.Render(" · ")), w, "…")
	}
	if m.actions.open || m.sortm.open || m.pick.open {
		hs = []hint{{"↑ ↓", T("hint.select")}, {"enter", T("hint.do")}, {"esc", T("hint.close")}}
	} else if m.help && m.helpFind.typing {
		hs = []hint{{T("hint.type"), T("find.hint")}, {"enter", T("hint.done")}, {"ctrl+u", T("find.clear")}, {"esc", T("hint.close")}}
	} else if m.help {
		hs = []hint{{"↑ ↓", T("hint.scroll")}, {"/", T("find.hint")}, {"esc", T("hint.close")}}
	} else if m.focus == focusContent && m.page == pageSearch && m.search.typing && m.search.open == nil {
		// Yazarken kısayollar çalışmaz; sadece aramaya dair olanlar.
		hs = []hint{{T("hint.type"), T("hint.tosearch")}, {"tab", T("hint.filter")}, {"enter ↓", T("hint.results")}, {"esc", T("hint.back")}}
		parts := make([]string, len(hs))
		for i, h := range hs {
			parts[i] = theme.HintKey.Render(h.key) + " " + theme.HintText.Render(h.verb)
		}
		return ansi.Truncate(strings.Repeat(" ", margin)+strings.Join(parts, theme.Faint.Render(" · ")), w, "…")
	} else if m.focus == focusContent && m.page == pageNow && !m.extraOn() && !m.vizFull {
		hs = []hint{{"↑ ↓", T("hint.lyrline")}, {"enter", T("hint.lyrjump")}, {"esc", T("hint.back")}}
	} else if m.focus == focusContent {
		hs = []hint{{"↑ ↓", T("hint.select")}, {"enter", T("hint.play")}}
		switch {
		case m.page == pageLibrary && m.lib.open == nil:
			hs = append(hs, hint{"tab", T("hint.filter")}, hint{"n", T("hint.newlist")})
			if m.cfg.Extras.Enabled {
				hs = append(hs, hint{"f", T("folder.hint")})
			}
		case m.extraOn() && m.extra.open == nil && (m.extra.page == extraArtist || m.extra.page == extraNews || m.extra.page == extraPeople):
			hs = append(hs, hint{"tab", T("hint.section")})
		case m.page == pageProfile && m.prof.open == nil:
			hs = append(hs, hint{"tab", T("hint.nextlist")})
			if m.cfg.Extras.Enabled {
				hs = append(hs, hint{"f", T("hint.people")})
			}
		case m.page == pageSettings:
			hs = []hint{{"↑ ↓", T("hint.select")}, {"← →", T("hint.change")}}
		}
		if m.shownFinder() != nil {
			hs = append(hs, hint{"/", T("find.hint")})
			if m.page != pageSettings {
				hs = append(hs, hint{"⇧O", T("sort.hint")})
			}
		}
		if m.page != pageSettings {
			hs = append(hs, hint{m.kn("info"), T("hint.info")})
		}
		hs = append(hs, hint{"esc", T("hint.back")}, hint{"1–8", T("hint.pages")})
	} else {
		hs = []hint{{"↑ ↓", T("hint.page")}, {"1–8", T("hint.pages")}}
		if m.page.hasContent() {
			hs = append(hs, hint{"enter", T("hint.enter")})
		} else if m.page == pageNow && m.lyricsPickable() {
			hs = append(hs, hint{"enter", T("hint.lyrenter")})
		}
	}
	pair := func(a, b string) string { return strings.TrimSpace(m.kn(a) + " " + m.kn(b)) }
	hs = append(hs,
		hint{m.kn("play_pause"), T("hint.pause")}, hint{pair("next", "previous"), T("hint.skip")},
		hint{m.kn("switch_speaker"), T("hint.switch")}, hint{m.kn("soloist"), T("hint.solo")},
		hint{pair("seek_back", "seek_forward"), T("hint.seek")}, hint{pair("volume_up", "volume_down"), T("hint.vol")},
		hint{m.kn("mute"), T("hint.mute")}, hint{m.kn("like"), T("hint.like")},
		hint{m.kn("shuffle"), T("hint.shuf")}, hint{m.kn("repeat"), T("hint.rep")}, hint{m.kn("quit"), T("hint.quit")},
	)
	render := func(h hint) string { return theme.HintKey.Render(h.key) + " " + theme.HintText.Render(h.verb) }
	sep := theme.Faint.Render(" · ")
	tail := render(hint{m.kn("help"), T("hint.keys")})
	if p := m.pendingKeys(); p != "" {
		tail = theme.Hi.Render(p) + sep + tail
	}
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

// contentSize, içerik alanının genişliği ve yüksekliği (layout'tan).
func (m Model) contentSize() (int, int) {
	l := m.layout()
	return l.contentW, l.bodyH
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
