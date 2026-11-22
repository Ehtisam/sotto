package ui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// Yan panel ([look] side_panel): geniş pencerede içeriğin sağında çalan
// şarkı, kapağı, sözler ve sıradakiler her sayfada görünür kalır. Yerini
// layout hesaplar; Şimdi çalıyor sayfasında (zaten hepsi orada), yardımda,
// kurulumda ve tam ekran görselleştirmede gösterilmez.

const (
	panelW          = 36 // panelin içerik genişliği
	panelMinContent = 72 // panel açılınca sayfaya en az bu kadar sütun kalmalı
)

// panelWanted: panel bu ekranda gösterilmeli mi (yer ayrıca layout'ta).
func (m Model) panelWanted() bool {
	return m.cfg.Look.PanelOn() && m.state.Item != nil && m.page != pageNow &&
		!m.help && !m.wiz.on && !m.whatsnew.open && !(m.vizFull && !m.pick.open)
}

// sidePanel, w genişliğinde, en çok h satırlık panel.
func (m Model) sidePanel(w, h int) string {
	rows := []string{theme.Dim.Italic(true).Render(T("panel.title")), ""}
	name, artists, album := m.describe(m.state.Item)

	// Kapak: kare (sütun = 2 × satır); panelin üçte birinden fazlasını almaz.
	if url := m.nowCoverURL(); url != "" {
		if ch := min(w/2, 9, (h-8)/3); ch >= 4 {
			rows = append(rows, strings.Split(m.coverBox(url, ch*2, ch), "\n")...)
			rows = append(rows, "")
		}
	}
	rows = append(rows, theme.Hi.Bold(true).Render(truncate(name, w)))
	if artists != "" {
		rows = append(rows, theme.Plain.Render(truncate(artists, w)))
	}
	if album != "" {
		rows = append(rows, theme.Dim.Italic(true).Render(truncate(album, w)))
	}
	if d := m.state.Item.Duration(); d > 0 {
		rows = append(rows, theme.Faint.Render(clock(m.position())+" / "+clock(d)))
	}

	// Sözler: söylenen satır ve çevresi (eş zamanlıysa), yalnız sözler varsa.
	if l, known := m.currentLyrics(); known && l != nil && len(l.Lines) > 0 && h-len(rows) > 8 {
		n := min(7, h-len(rows)-6)
		lines := m.lyricsRows(w, n+2) // son iki satır: boşluk ve kaynak
		if len(lines) > 2 {
			rows = append(rows, "", theme.Dim.Italic(true).Render(T("panel.lyrics")))
			rows = append(rows, lines[:len(lines)-2]...)
		}
	}

	// Sıradakiler: kalan yere sığdığı kadar.
	var next []string
	for _, q := range m.queue.Upcoming {
		if q.Item == nil || q.Item.URI == "" {
			continue
		}
		t, a, _ := m.describe(q.Item)
		if t == "" {
			continue
		}
		line := t
		if a != "" {
			line += theme.Faint.Render(" — " + a)
		}
		next = append(next, theme.Plain.Render(truncate(line, w)))
	}
	if room := h - len(rows) - 2; room > 0 && len(next) > 0 {
		rows = append(rows, "", theme.Dim.Italic(true).Render(T("panel.next")))
		rows = append(rows, next[:min(len(next), room)]...)
	}
	if len(rows) > h {
		rows = rows[:h]
	}
	return lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).Render(strings.Join(rows, "\n"))
}
