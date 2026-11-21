package ui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// Kaydırma çubuğu: liste pencereye sığmıyorsa sağ kenarda ince bir çubuk;
// koyu kısım görünen bölümün listedeki yeri ve büyüklüğü.

// scrollbarW, n satırlık liste h satıra sığmıyorsa çubuk için ayrılan
// sütun sayısı (boşluk + çubuk), sığıyorsa 0.
func scrollbarW(n, h int) int {
	if h > 0 && n > h {
		return 2
	}
	return 0
}

// thumb, çubuğun koyu kısmının ilk ve son+1 satırı.
func thumb(n, scroll, h int) (int, int) {
	size := max(1, h*h/n)
	top := 0
	if n > h {
		top = scroll * (h - size) / (n - h)
	}
	top = max(0, min(top, h-size))
	return top, top + size
}

// withScrollbar, rows[start:]'taki liste satırlarının (en çok h tane) sağına
// çubuğu ekler. Satırlar w-2 genişliğinde çizilmiş olmalı (scrollbarW).
func withScrollbar(rows []string, start, n, scroll, h, w int) []string {
	if scrollbarW(n, h) == 0 || start > len(rows) {
		return rows
	}
	from, to := thumb(n, scroll, h)
	track, knob := "│", "┃"
	if theme.Icon.Note == "*" || theme.Icon.Pause == "||" { // ascii simgeler
		track, knob = "|", "#"
	}
	shown := min(h, len(rows)-start)
	for i := 0; i < shown; i++ {
		r := rows[start+i]
		if lipgloss.Width(r) > w-2 {
			r = truncate(r, w-2) // çubuk taşmasın
		}
		pad := w - 2 - lipgloss.Width(r)
		if pad < 0 {
			pad = 0
		}
		bar := theme.Faint.Render(track)
		if i >= from && i < to {
			bar = theme.Dim.Render(knob)
		}
		rows[start+i] = r + strings.Repeat(" ", pad) + " " + bar
	}
	return rows
}
