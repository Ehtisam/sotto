// Package theme, sotto'nun renk tokenları ve temel stilleri.
// Varsayılan tema Cardo'nun paletinden gelir.
package theme

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Renk tokenları.
var (
	Ground   color.Color = lipgloss.Color("#0e0c0a") // zemin
	Text     color.Color = lipgloss.Color("#e8e0d0") // yazı
	Muted    color.Color = lipgloss.Color("#a89880") // soluk yazı
	Line     color.Color = lipgloss.Color("#4a423a") // çizgi, kenarlık
	Accent   color.Color = lipgloss.Color("#9d6ee0") // vurgu
	AccentHi color.Color = lipgloss.Color("#b892ec") // açık vurgu
	Selected color.Color = lipgloss.Color("#2d1f3d") // seçili satır zemini
	Gold     color.Color = lipgloss.Color("#d9a441") // çalan şarkı, ▶
	Red      color.Color = lipgloss.Color("#d9705f") // hata
)

// Temel stiller.
var (
	Plain    = lipgloss.NewStyle().Foreground(Text)
	Dim      = lipgloss.NewStyle().Foreground(Muted)
	Faint    = lipgloss.NewStyle().Foreground(Line)
	Title    = lipgloss.NewStyle().Foreground(Text).Italic(true)
	Brand    = lipgloss.NewStyle().Foreground(AccentHi).Italic(true)
	Hi       = lipgloss.NewStyle().Foreground(AccentHi)
	GoldText = lipgloss.NewStyle().Foreground(Gold)
	Error    = lipgloss.NewStyle().Foreground(Red)
	Bold     = lipgloss.NewStyle().Foreground(Text).Bold(true)

	// SelectedRow, menüde ve listelerde seçili satır.
	SelectedRow = lipgloss.NewStyle().Foreground(AccentHi).Background(Selected)
)
