// Package theme, sotto'nun renk tokenları ve temel stilleri.
// Varsayılan tema Cardo'nun paletinden gelir; Apply başka bir temaya geçer.
package theme

import (
	"fmt"
	"image/color"

	"charm.land/lipgloss/v2"
)

// Palette, bir temanın renkleri (#rrggbb).
type Palette struct {
	Ground, Text, Muted, Line  string
	Accent, AccentHi, Selected string
	Gold, Red                  string
}

// Hazır temalar. Ad ayar dosyasına yazılan addır; görünen ad çeviriden gelir.
var Presets = []struct {
	Name    string
	Palette Palette
}{
	{"cardo", Palette{"#0e0c0a", "#e8e0d0", "#a89880", "#4a423a", "#9d6ee0", "#b892ec", "#2d1f3d", "#d9a441", "#d9705f"}},
	{"night", Palette{"#0b0e14", "#dde3ee", "#8b96a8", "#39414f", "#5b8def", "#8fb2f5", "#1a2740", "#e0b153", "#e06c6c"}},
	{"moss", Palette{"#0c0f0c", "#e2e6da", "#98a38c", "#3e463a", "#6fae5f", "#98cc88", "#1d2c1a", "#d9a441", "#d9705f"}},
	{"mono", Palette{"#0d0d0d", "#e6e6e6", "#9a9a9a", "#444444", "#bdbdbd", "#ffffff", "#2a2a2a", "#e6e6e6", "#d9705f"}},
	{"paper", Palette{"#f4efe6", "#2b2620", "#7a6e60", "#c9bfb0", "#7a4fc0", "#6a3fb0", "#e6dcf3", "#a8740f", "#b84a3a"}},
}

// Vurgu renkleri: temanın vurgusunun yerine geçer. Açık tonu ve seçili satır
// zemini bundan türetilir.
var Accents = []struct{ Name, Hex string }{
	{"purple", "#9d6ee0"},
	{"blue", "#5b8def"},
	{"teal", "#3fa6a0"},
	{"green", "#6fae5f"},
	{"amber", "#d08a3c"},
	{"pink", "#d66ba0"},
	{"red", "#c95a4a"},
}

// Renk tokenları.
var (
	Ground   color.Color // zemin
	Text     color.Color // yazı
	Muted    color.Color // soluk yazı
	Line     color.Color // çizgi, kenarlık
	Accent   color.Color // vurgu
	AccentHi color.Color // açık vurgu
	Selected color.Color // seçili satır zemini
	Gold     color.Color // çalan şarkı, ▶
	Red      color.Color // hata
)

// Temel stiller. Apply hepsini yeniden kurar.
var (
	Plain, Dim, Faint, Title, Brand, Hi, GoldText, Error, Bold lipgloss.Style

	// SelectedRow, menüde ve listelerde seçili satır.
	SelectedRow lipgloss.Style
)

func init() { Apply("cardo", "") }

// PaletteOf, adı verilen temanın renkleri; bilinmeyen ad varsayılana düşer.
func PaletteOf(name string) Palette {
	for _, p := range Presets {
		if p.Name == name {
			return p.Palette
		}
	}
	return Presets[0].Palette
}

// AccentHex, adı verilen vurgu rengi; "" ya da bilinmeyen ad için "".
func AccentHex(name string) string {
	for _, a := range Accents {
		if a.Name == name {
			return a.Hex
		}
	}
	return ""
}

// Resolve, temaya vurgu rengini uygular: açık ton ve seçili satır zemini
// vurgu ile zeminden türetilir.
func Resolve(themeName, accent string) Palette {
	p := PaletteOf(themeName)
	hex := AccentHex(accent)
	if hex == "" {
		return p
	}
	p.Accent = hex
	if luminance(p.Ground) > 0.5 { // açık tema
		p.AccentHi = mix(hex, "#000000", 0.15)
		p.Selected = mix(hex, p.Ground, 0.82)
	} else {
		p.AccentHi = mix(hex, "#ffffff", 0.25)
		p.Selected = mix(hex, p.Ground, 0.75)
	}
	return p
}

// Apply, temayı ve vurgu rengini bütün arayüze uygular.
func Apply(themeName, accent string) {
	p := Resolve(themeName, accent)
	c := lipgloss.Color
	Ground, Text, Muted, Line = c(p.Ground), c(p.Text), c(p.Muted), c(p.Line)
	Accent, AccentHi, Selected = c(p.Accent), c(p.AccentHi), c(p.Selected)
	Gold, Red = c(p.Gold), c(p.Red)

	s := lipgloss.NewStyle
	Plain = s().Foreground(Text)
	Dim = s().Foreground(Muted)
	Faint = s().Foreground(Line)
	Title = s().Foreground(Text).Italic(true)
	Brand = s().Foreground(AccentHi).Italic(true)
	Hi = s().Foreground(AccentHi)
	GoldText = s().Foreground(Gold)
	Error = s().Foreground(Red)
	Bold = s().Foreground(Text).Bold(true)
	SelectedRow = s().Foreground(AccentHi).Background(Selected)
}

// Swatch, renk kareleri (önizleme): verilen her renkten bir ■.
func Swatch(hexes ...string) string {
	out := ""
	for _, h := range hexes {
		out += lipgloss.NewStyle().Foreground(lipgloss.Color(h)).Render("■")
	}
	return out
}

// ---- renk hesapları ----

func rgb(hex string) (r, g, b float64) {
	var ri, gi, bi int
	if _, err := fmt.Sscanf(hex, "#%02x%02x%02x", &ri, &gi, &bi); err != nil {
		return 0, 0, 0
	}
	return float64(ri), float64(gi), float64(bi)
}

// mix, a'dan b'ye t oranında karışım (t=0: a, t=1: b).
func mix(a, b string, t float64) string {
	ar, ag, ab := rgb(a)
	br, bg, bb := rgb(b)
	f := func(x, y float64) int { return int(x + (y-x)*t + 0.5) }
	return fmt.Sprintf("#%02x%02x%02x", f(ar, br), f(ag, bg), f(ab, bb))
}

func luminance(hex string) float64 {
	r, g, b := rgb(hex)
	return (0.2126*r + 0.7152*g + 0.0722*b) / 255
}
