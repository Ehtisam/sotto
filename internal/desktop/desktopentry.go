package desktop

import "strings"

// Linux uygulama menüsü: ~/.local/share/applications/sotto.desktop
// (freedesktop "Desktop Entry" belirtimi). sotto bir terminal programı olduğu
// için Terminal=true: menüden açılınca masaüstünün terminali onu başlatır.
// Burada yalnızca dosyanın içeriği; yazan kod shortcut_linux.go'da.

// desktopEntry, target'ı (sotto'nun tam yolu) açan .desktop dosyası.
func desktopEntry(target string) string {
	return "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=sotto\n" +
		"GenericName=Spotify player\n" +
		"Comment=A good-looking, full-featured Spotify player for the terminal\n" +
		"Exec=" + execQuote(target) + "\n" +
		"Icon=sotto\n" +
		"Terminal=true\n" +
		"Categories=AudioVideo;Audio;Player;ConsoleOnly;\n" +
		"Keywords=spotify;music;player;\n"
}

// execQuote: Exec satırı için yol. Belirtime göre özel karakterli bir
// argüman çift tırnağa alınır, içindeki " ` $ \ ters eğik çizgiyle kaçar;
// dosya dizesi düzeyinde de her ters eğik çizgi ikilenir.
func execQuote(s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("/._-+", r))
	}) < 0 {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune("\"`$\\", r) {
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	return `"` + strings.ReplaceAll(b.String(), `\`, `\\`) + `"`
}

// execTarget: execQuote'un tersi (ShortcutInfo için).
func execTarget(line string) string {
	v := strings.TrimPrefix(line, "Exec=")
	if !strings.HasPrefix(v, `"`) {
		return v
	}
	v = strings.TrimSuffix(strings.TrimPrefix(v, `"`), `"`)
	v = strings.ReplaceAll(v, `\\`, `\`)
	var b strings.Builder
	esc := false
	for _, r := range v {
		if r == '\\' && !esc {
			esc = true
			continue
		}
		esc = false
		b.WriteRune(r)
	}
	return b.String()
}
