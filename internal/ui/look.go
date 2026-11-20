package ui

import (
	"strings"

	"github.com/Ehtisam/sotto/internal/config"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// Ayrıntılı görünüm: config [look] ve [colors] ayarlarının arayüze uygulanması
// (tema, vurgu, öğe renkleri, simgeler, çizgiler, ilerleme çubuğu, şeridin
// yeri ve biçimi, pencere başlığı, satır numaraları).

// lookOptions, ayarlardan temanın seçenekleri.
func lookOptions(c config.Config) theme.Options {
	return theme.Options{
		Theme: c.Look.Theme, Accent: c.Look.Accent, Colors: c.Colors,
		Icons: c.Look.Icons, Border: c.Look.Border, Progress: c.Look.Progress,
	}
}

// applyLook, ayarları temaya uygular (açılışta ve Ayarlar'da değişince).
func applyLook(c config.Config) { theme.Configure(lookOptions(c)) }

// stripTop: çalan şarkı şeridi üstte.
func (m Model) stripTop() bool { return m.cfg.Look.Strip == "top" }

// stripLine, şeridin ilk satırı: strip_format'a göre; şarkı adı başlık
// rengiyle, öbür alanlar ve yazılar soluk.
func (m Model) stripLine() string {
	format := m.cfg.Look.StripFormat
	if strings.TrimSpace(format) == "" {
		format = config.DefaultStripFormat
	}
	f := m.trackFields()
	// Alanları sırayla yer tutuculara çevir; boş alanların ayracını
	// fillTemplate temizler. Sonra yer tutucuları renklendirerek geri koy.
	const mark = "\x00"
	plain := map[string]string{}
	for k, v := range f {
		if v != "" {
			plain[k] = mark + k + mark
		} else {
			plain[k] = ""
		}
	}
	text := fillTemplate(format, plain)
	var b strings.Builder
	parts := strings.Split(text, mark)
	for i, p := range parts {
		if i%2 == 1 { // alan adı
			if p == "title" {
				b.WriteString(themeTitle(f[p]))
			} else {
				b.WriteString(themeDim(f[p]))
			}
			continue
		}
		if p != "" {
			b.WriteString(themeDim(p))
		}
	}
	return b.String()
}

func themeTitle(s string) string { return theme.Title.Render(s) }
func themeDim(s string) string   { return theme.Dim.Render(s) }

// windowTitleText, pencere başlığı: window_title'a göre ("" ise sotto).
func (m Model) windowTitleText() string {
	format := m.cfg.Look.WindowTitle
	f := m.trackFields()
	if f["title"] == "" || strings.TrimSpace(format) == "" {
		return "sotto"
	}
	return fillTemplate(format, f)
}

// lineNumber, listedeki bir satırın numarası: absolute (1, 2, 3…), relative
// (seçili satıra uzaklık; seçili satırda kendi numarası) ya da off.
func (m Model) lineNumber(abs, pos, cursorPos int, active bool) string {
	switch m.cfg.Look.LineNumbers {
	case "off":
		return ""
	case "relative":
		if active && cursorPos >= 0 && pos != cursorPos {
			d := pos - cursorPos
			if d < 0 {
				d = -d
			}
			return itoa(d)
		}
	}
	return itoa(abs)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
