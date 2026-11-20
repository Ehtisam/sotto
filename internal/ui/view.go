package ui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Ehtisam/sotto/internal/soloist"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

const (
	menuWidth = 20 // sol sütun
	gutter    = 3  // sütunlar arası boşluk
	margin    = 1  // sol kenar boşluğu
)

func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.BackgroundColor = theme.Ground
	v.ForegroundColor = theme.Text
	v.WindowTitle = m.windowTitle()
	return v
}

func (m Model) windowTitle() string {
	if n := m.state.Item.Name(); n != "" {
		if a := m.state.Item.Creators(); a != "" {
			return n + " — " + a + " · sotto"
		}
		return n + " · sotto"
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

	lines := []string{
		m.header(w),
		rule,
		m.body(w, bodyH),
		rule,
		m.strip(w),
		"",
		m.hints(w),
	}
	return strings.Join(lines, "\n")
}

// ---- başlık ----

func (m Model) header(w int) string {
	left := strings.Repeat(" ", margin) + theme.Brand.Render("sotto") + theme.Faint.Render("  ·  ") + theme.Dim.Render("alçak sesle")
	right := m.deviceStatus() + " "
	return spread(left, right, w)
}

func (m Model) deviceStatus() string {
	switch m.engine {
	case engineOff:
		return theme.Dim.Render("hoparlör kapalı")
	case engineStarting:
		return theme.Dim.Render("Soloist başlatılıyor…")
	case engineFailed:
		return theme.Error.Render("Soloist kapalı")
	}
	if !m.haveAuth {
		return theme.Dim.Render("bağlanıyor…")
	}
	if !m.auth.LoggedIn {
		return theme.Dim.Render("eşleşme bekleniyor")
	}
	name := m.auth.DeviceName
	if name == "" {
		name = "sotto"
	}
	if m.auth.IsActive {
		return theme.GoldText.Render("♪ ") + theme.Plain.Render(name) + theme.Dim.Render(" · hafif mod")
	}
	return theme.Dim.Render(name + " · etkin değil")
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
	rows := []string{theme.Dim.Italic(true).Render("içindekiler"), ""}
	for i, p := range pages {
		label := fmt.Sprintf(" %-5s%s", p.numeral, p.name)
		if page(i) == m.page {
			rows = append(rows, theme.SelectedRow.Width(menuWidth).Render(label))
			continue
		}
		rows = append(rows, theme.Dim.Render(fmt.Sprintf(" %-5s", p.numeral))+theme.Plain.Render(p.name))
	}
	return lipgloss.NewStyle().Width(menuWidth).Height(h).MaxHeight(h).Render(strings.Join(rows, "\n"))
}

func (m Model) content(w, h int) string {
	var rows []string
	if m.page == pageNow {
		rows = m.nowPlaying(w, h)
	} else {
		p := pages[m.page]
		rows = []string{
			theme.Title.Render(p.name),
			"",
			theme.Dim.Render("Bu sayfa henüz yazılmadı."),
			theme.Faint.Render("gelecek: ") + theme.Dim.Render(p.soon),
		}
	}
	for i, r := range rows {
		rows[i] = ansi.Truncate(r, w, "…")
	}
	return lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).Render(strings.Join(rows, "\n"))
}

// nowPlaying, I · Şimdi çalıyor sayfası.
func (m Model) nowPlaying(w, h int) []string {
	rows := []string{theme.Title.Render("Şimdi çalıyor")}
	if msg := m.engineMessage(); len(msg) > 0 {
		return append(append(rows, ""), msg...)
	}

	it := m.state.Item
	if it == nil || it.Name() == "" {
		return append(rows, "",
			theme.Dim.Render("Çalan bir şey yok."),
			theme.Faint.Render("Telefondan ya da Spotify'dan bir şey başlat, ya da ")+theme.Dim.Render("boşluk")+theme.Faint.Render(" ile kaldığın yerden devam et."),
		)
	}

	if ctx := m.state.Context; ctx != nil && ctx.Name() != "" {
		rows = append(rows, theme.Dim.Render(ctx.Name())+theme.Faint.Render(" · "+entityKind(ctx.EntityType)))
	} else {
		rows = append(rows, "")
	}
	rows = append(rows, "")

	title := theme.Hi.Bold(true).Render(it.Name())
	if it.Explicit() {
		title += theme.Faint.Render("  E")
	}
	rows = append(rows, title)
	if a := it.Creators(); a != "" {
		rows = append(rows, theme.Plain.Render(a))
	}
	if p := it.ParentName(); p != "" {
		rows = append(rows, theme.Dim.Render(p))
	}

	if !m.auth.IsActive {
		rows = append(rows, "", theme.Dim.Render("sotto şu an etkin cihaz değil; ")+theme.Plain.Render("a")+theme.Dim.Render(" ile buraya al."))
	}

	// Sıradakiler. Soloist bazı girişleri adsız gönderiyor; adlar ileride
	// Web API'den tamamlanacak, şimdilik adı olanlar gösteriliyor.
	var named []soloist.QueueEntry
	for _, q := range m.queue.Upcoming {
		if q.Item.Name() != "" {
			named = append(named, q)
		}
	}
	if len(m.queue.Upcoming) > 0 && h-len(rows) > 4 {
		rows = append(rows, "", theme.Dim.Italic(true).Render("sıradakiler"))
		room := h - len(rows)
		for i, q := range named {
			if i >= room-1 {
				break
			}
			num := theme.Faint.Render(fmt.Sprintf("%2d  ", i+1))
			rows = append(rows, num+leader(q.Item.Name(), q.Item.Creators(), w-4, theme.Plain, theme.Dim))
		}
		if rest := len(m.queue.Upcoming) - len(named); rest > 0 {
			rows = append(rows, theme.Faint.Render(fmt.Sprintf("    ve %d şarkı daha", rest)))
		}
	}
	return rows
}

// engineMessage, Soloist hazır değilse ne yapılması gerektiğini anlatır.
func (m Model) engineMessage() []string {
	switch m.engine {
	case engineOff:
		return []string{theme.Dim.Render("Hafif mod kapalı.")}
	case engineStarting:
		return []string{theme.Dim.Render("Soloist başlatılıyor…")}
	case engineFailed:
		err := m.engineErr
		switch {
		case errors.Is(err, soloist.ErrNotInstalled):
			return []string{
				theme.Plain.Render("Soloist bulunamadı."),
				theme.Faint.Render("Beklenen yer: ") + theme.Dim.Render(m.cfg.SoloistPath()),
				theme.Faint.Render("İlk kurulum sihirbazı ileride bunu kendisi indirecek."),
			}
		case errors.Is(err, soloist.ErrNoAPIKey):
			return []string{
				theme.Plain.Render("Soloist anahtarı yok."),
				theme.Faint.Render("Anahtarı şu dosyaya kaydet: ") + theme.Dim.Render(m.cfg.SoloistKeyFile()),
			}
		case errors.Is(err, soloist.ErrExpired):
			return []string{
				theme.Plain.Render("Soloist'in süresi dolmuş."),
				theme.Faint.Render("Her sürüm 90 gün çalışıyor; yenisini indirmek gerekiyor."),
			}
		default:
			msg := "bilinmeyen hata"
			if err != nil {
				msg = err.Error()
			}
			return []string{
				theme.Error.Render("Soloist çalışmıyor."),
				theme.Dim.Render(msg),
				theme.Faint.Render("Kayıt: ") + theme.Dim.Render(m.cfg.SoloistLog()),
			}
		}
	}
	if !m.haveAuth {
		return []string{theme.Dim.Render("Soloist'e bağlanılıyor…")}
	}
	if !m.auth.LoggedIn {
		return []string{
			theme.Plain.Render("Eşleşme bekleniyor."),
			theme.Dim.Render("Spotify uygulamasında ya da telefonda cihaz listesinden ") + theme.Hi.Render("sotto") + theme.Dim.Render("'yu seç."),
			theme.Faint.Render("Bu bir kerelik; sonra sotto girişi hatırlar."),
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

	var line1 string
	if it == nil || it.Name() == "" {
		line1 = icon + "  " + theme.Faint.Render("çalan bir şey yok")
	} else {
		line1 = icon + "  " + theme.Title.Render(it.Name())
		if a := it.Creators(); a != "" {
			line1 += theme.Dim.Render(" — " + a)
		}
	}
	right := ""
	if m.flash != "" {
		right = theme.Error.Render(m.flash)
	} else if ctx := m.state.Context; ctx != nil && ctx.Name() != "" {
		right = theme.Faint.Render(ctx.Name())
	}
	line1 = spread(ansi.Truncate(line1, inner-lipgloss.Width(right)-2, "…"), right, inner)

	// İlerleme çubuğu, süre, ses, karıştır, tekrar.
	pos, dur := m.position(), it.Duration()
	extras := theme.Faint.Render("   ses ") + theme.Plain.Render(fmt.Sprint(m.state.Volume)) + "  " + m.shuffleMark() + " " + m.repeatMark()
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

func (m Model) hints(w int) string {
	type hint struct{ key, verb string }
	hs := []hint{
		{"boşluk", "çal/durdur"}, {"n p", "sonraki/önceki"}, {"← →", "sar"}, {"+ −", "ses"},
		{"s", "karıştır"}, {"r", "tekrar"}, {"↑ ↓", "sayfa"}, {"q", "çık"},
	}
	parts := make([]string, len(hs))
	for i, h := range hs {
		parts[i] = theme.Dim.Render(h.key) + " " + theme.Faint.Render(h.verb)
	}
	line := strings.Repeat(" ", margin) + strings.Join(parts, theme.Faint.Render(" · "))
	return ansi.Truncate(line, w, "…")
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

// leader, "sol ········ sağ" satırı: iki ucu noktalı çizgiyle birleştirir.
func leader(left, right string, w int, ls, rs lipgloss.Style) string {
	if right == "" {
		return ls.Render(ansi.Truncate(left, w, "…"))
	}
	rw := lipgloss.Width(right)
	maxLeft := w - rw - 4
	if maxLeft < 4 {
		return ls.Render(ansi.Truncate(left, w, "…"))
	}
	left = ansi.Truncate(left, maxLeft, "…")
	dots := w - lipgloss.Width(left) - rw - 2
	return ls.Render(left) + " " + theme.Faint.Render(strings.Repeat("·", max(dots, 1))) + " " + rs.Render(right)
}

func clock(d time.Duration) string {
	s := int(d.Seconds())
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

func entityKind(t string) string {
	switch t {
	case "playlist":
		return "çalma listesi"
	case "album":
		return "albüm"
	case "artist":
		return "sanatçı"
	case "show":
		return "podcast"
	case "collection":
		return "beğenilenler"
	}
	return t
}
