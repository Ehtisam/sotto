package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Ehtisam/sotto/internal/cli"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// Mini görünüm: pencere çok küçültülünce menü, sayfalar ve kısayol satırı
// kalkar; yalnızca çalan şarkı kalır: kapak, ad, sanatçı, albüm, söylenen söz
// satırı (sığarsa), ilerleme çubuğu ve durum. Tuşların hepsi çalışmaya devam eder.

const (
	miniBelowW = 60 // bu genişliğin altında
	miniBelowH = 16 // ya da bu yüksekliğin altında
)

// miniMode: mini görünüm mü. Kurulum, form, eylem menüsü ve kısayol listesi
// açıkken normal görünüm kalır (onlar yer ister).
func (m Model) miniMode() bool {
	if m.width <= 0 || m.height <= 0 || m.wiz.on || m.form.on || m.actions.open || m.help {
		return false
	}
	return m.width < miniBelowW || m.height < miniBelowH
}

func (m Model) miniView() string {
	w, h := m.width, m.height
	pad := 1
	if w < 30 {
		pad = 0
	}
	inner := max(w-2*pad, 1)

	name, artists, album := m.describe(m.state.Item)

	// Kapak: bütün yükseklik, kare; yanına en az 18 sütun bilgi kalsın.
	ch := h
	cw := ch * 2
	if inner-cw-2 < 18 {
		cw = max(inner-2-18, 0)
		ch = min(ch, cw/2)
		cw = ch * 2
	}
	url := m.notifyCoverURL()
	if !m.cfg.Look.CoversOn() || ch < 3 || url == "" || name == "" {
		cw, ch = 0, 0
	}
	iw := inner
	if cw > 0 {
		iw = inner - cw - 2
	}

	// Alt satırlar: ilerleme ve durum.
	pos, dur := m.position(), m.state.Item.Duration()
	left, right := theme.Dim.Render(clock(pos))+" ", " "+theme.Dim.Render(clock(dur))
	progress := left + bar(pos, dur, max(iw-lipgloss.Width(left)-lipgloss.Width(right), 3)) + right
	status := m.miniStatus(iw)

	var top []string
	if name == "" {
		top = append(top, theme.Dim.Render(truncate(T("now.nothing"), iw)))
		if l := m.rt.ev.last; l.Track != "" && l.Title != "" && m.spState == spReady && h >= 4 {
			top = append(top, theme.Faint.Render(T("mini.resume"))+theme.Plain.Render(truncate(l.Title, iw-8)))
		}
		progress = ""
	} else {
		title := theme.Hi.Bold(true).Render(truncate(name, iw))
		top = append(top, title)
		if artists != "" && h >= 3 {
			top = append(top, theme.Plain.Render(truncate(artists, iw)))
		}
		if h >= 5 {
			sub := album
			if ctx := m.state.Context; ctx != nil && ctx.Name() != "" && h < 7 {
				sub = ctx.Name()
			}
			if sub != "" {
				top = append(top, theme.Dim.Render(truncate(sub, iw)))
			}
		}
		if h >= 7 {
			if ctx := m.state.Context; ctx != nil && ctx.Name() != "" && ctx.Name() != album {
				top = append(top, theme.Faint.Render(truncate(ctx.Name(), iw)))
			}
		}
	}

	if h == 1 && name != "" { // tek satır: durum, ad, süre
		line := m.miniIcon() + "  " + theme.Hi.Bold(true).Render(name)
		if artists != "" {
			line += theme.Dim.Render(" — " + artists)
		}
		tail := "  " + theme.Dim.Render(clock(pos)+"/"+clock(dur))
		line = ansi.Truncate(line, max(inner-lipgloss.Width(tail), 1), "…") + tail
		return lipgloss.NewStyle().MarginLeft(pad).MaxWidth(w).Render(ansi.Truncate(line, inner, ""))
	}
	if h == 2 && name != "" && artists != "" { // ad ve sanatçı tek satırda
		top = []string{ansi.Truncate(top[0]+theme.Dim.Render(" — "+artists), iw, "…")}
	}
	bottom := []string{status}
	if progress != "" && h >= 3 {
		bottom = append([]string{progress}, bottom...)
	}
	// Söylenen söz satırı (sığarsa, sözler biliniyorsa).
	if line := m.miniLyric(); line != "" && h-len(top)-len(bottom) >= 2 {
		bottom = append([]string{theme.Playing.Render(truncate(theme.Icon.Note+" "+line, iw))}, bottom...)
	}

	rows := top
	if gap := h - len(top) - len(bottom); gap > 0 {
		rows = append(rows, make([]string, gap)...)
	} else if gap < 0 { // çok alçak: üstten kes, durum hep kalsın
		rows = rows[:max(len(rows)+gap, 0)]
	}
	rows = append(rows, bottom...)
	if len(rows) > h {
		rows = rows[len(rows)-h:]
	}
	for i, r := range rows {
		rows[i] = ansi.Truncate(r, iw, "…")
	}
	info := lipgloss.NewStyle().Width(iw).Height(h).MaxHeight(h).Render(strings.Join(rows, "\n"))

	block := info
	if cw > 0 {
		cover := m.coverBox(url, cw, ch)
		if ch < h { // kapağı dikeyde ortala
			cover = lipgloss.NewStyle().PaddingTop((h - ch) / 2).Render(cover)
		}
		block = lipgloss.JoinHorizontal(lipgloss.Top, cover, "  ", info)
	}
	block = lipgloss.NewStyle().MarginLeft(pad).Render(block)
	return lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).MaxWidth(w).Render(block)
}

// miniStatus: çal/durdur simgesi, ses, karıştır/tekrar, cihaz; not varsa not.
// miniIcon: çalarken ❚❚ (basınca duraklar), dururken ▶, boşken ·.
func (m Model) miniIcon() string {
	switch {
	case m.state.Item == nil:
		return theme.Faint.Render(theme.Icon.Idle)
	case m.playing():
		return theme.Playing.Render(theme.Icon.Pause)
	}
	return theme.Dim.Render(theme.Icon.Play)
}

func (m Model) miniStatus(w int) string {
	icon := m.miniIcon()
	if m.flash != "" {
		style := theme.Error
		if m.flashInfo {
			style = theme.GoldText
		}
		return icon + "  " + style.Render(truncate(m.flash, max(w-4, 1)))
	}
	vol := theme.Plain.Render(fmt.Sprint(m.state.Volume))
	if m.remote.active && cli.Disallowed(m.remote.player, "volume", "") {
		vol = theme.Faint.Render(fmt.Sprint(m.state.Volume))
	}
	s := icon + "  " + theme.Faint.Render(strings.TrimSpace(T("strip.vol"))+" ") + vol + "  " + m.shuffleMark() + " " + m.repeatMark()
	if sl := m.sleepMark(); sl != "" {
		s += "  " + sl
	}
	dev := m.remote.device
	if m.soloistActive() {
		dev = m.auth.DeviceName
	}
	if dev != "" && lipgloss.Width(s)+3+lipgloss.Width(dev) <= w {
		s = spread(s, theme.Faint.Render(dev), w)
	}
	return s
}

// miniLyric: eş zamanlı sözlerde şu an söylenen satır.
func (m Model) miniLyric() string {
	l, known := m.currentLyrics()
	if !known || l == nil || !l.Synced {
		return ""
	}
	if i := l.At(m.position()); i >= 0 && i < len(l.Lines) {
		return strings.TrimSpace(l.Lines[i].Text)
	}
	return ""
}
