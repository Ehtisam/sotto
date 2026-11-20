// Package theme, sotto'nun renk tokenları, temel stilleri, simgeleri ve
// çizgileri. Varsayılan tema Cardo'nun paletinden gelir; Configure başka bir
// temaya, vurguya, öğe renklerine, simge takımına ve çizgi türüne geçer.
package theme

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

// Palette, bir temanın renkleri: "#rrggbb", terminalin kendi renklerinden
// biri için "0"–"15", ya da "" (terminalin varsayılanı; yalnız Ground ve Text).
type Palette struct {
	Ground, Text, Muted, Line  string
	Accent, AccentHi, Selected string
	Gold, Red                  string
}

// Preset, adı olan bir tema.
type Preset struct {
	Name    string
	Palette Palette
	User    bool // kullanıcının içe aktardığı (themes klasöründen)
}

// Presets, hazır temalar. Ad ayar dosyasına yazılan addır; görünen ad çeviriden gelir.
var Presets = []Preset{
	{"cardo", Palette{"#0e0c0a", "#e8e0d0", "#a89880", "#4a423a", "#9d6ee0", "#b892ec", "#2d1f3d", "#d9a441", "#d9705f"}, false},
	{"night", Palette{"#0b0e14", "#dde3ee", "#8b96a8", "#39414f", "#5b8def", "#8fb2f5", "#1a2740", "#e0b153", "#e06c6c"}, false},
	{"moss", Palette{"#0c0f0c", "#e2e6da", "#98a38c", "#3e463a", "#6fae5f", "#98cc88", "#1d2c1a", "#d9a441", "#d9705f"}, false},
	{"mono", Palette{"#0d0d0d", "#e6e6e6", "#9a9a9a", "#444444", "#bdbdbd", "#ffffff", "#2a2a2a", "#e6e6e6", "#d9705f"}, false},
	{"paper", Palette{"#f4efe6", "#2b2620", "#7a6e60", "#c9bfb0", "#7a4fc0", "#6a3fb0", "#e6dcf3", "#a8740f", "#b84a3a"}, false},
	// terminal: terminalin kendi 16 rengi ve zemini (terminalin şemasına uyar).
	{"terminal", Palette{"", "", "8", "8", "5", "13", "0", "3", "1"}, false},
}

var userPresets []Preset

// All, hazır temalar ve kullanıcının temaları.
func All() []Preset { return append(append([]Preset(nil), Presets...), userPresets...) }

// SetUser, kullanıcının temalarını tanıtır (LoadUser okur).
func SetUser(ps []Preset) { userPresets = ps }

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

// ColorKeys, [colors] bölümünde kullanılabilecek anahtarlar: önce paletin
// renkleri, sonra tek tek öğeler.
var ColorKeys = []string{
	"ground", "text", "muted", "line", "accent", "accent_hi", "selected", "gold", "red",
	"brand", "title", "playing", "progress", "selected_text", "hint_key", "hint_text", "border",
}

// Renk tokenları.
var (
	Ground   color.Color // zemin (nil: terminalin)
	Text     color.Color // yazı
	Muted    color.Color // soluk yazı
	Line     color.Color // çizgi, kenarlık
	Accent   color.Color // vurgu
	AccentHi color.Color // açık vurgu
	Selected color.Color // seçili satır zemini
	Gold     color.Color // çalan şarkı, ▶
	Red      color.Color // hata
)

// Temel stiller. Configure hepsini yeniden kurar.
var (
	Plain, Dim, Faint, Title, Brand, Hi, GoldText, Error, Bold lipgloss.Style

	// SelectedRow, menüde ve listelerde seçili satır.
	SelectedRow lipgloss.Style

	// Öğeler: çalan şarkı işareti, ilerleme çubuğu, alt satırdaki tuşlar, çizgiler.
	Playing, ProgressDone, ProgressLeft, HintKey, HintText, Border lipgloss.Style
)

// Options, görünümün bütün ayarları.
type Options struct {
	Theme, Accent string
	Colors        map[string]string // öğe renkleri (ColorKeys)
	Icons         string            // unicode, ascii, nerd
	Border        string            // line, heavy, double, dashed, ascii, none
	Progress      string            // line, block, dots, thin
}

var cur = Options{Theme: "cardo"}

func init() { Configure(cur) }

// Current, son uygulanan ayarlar.
func Current() Options { return cur }

// PaletteOf, adı verilen temanın renkleri; bilinmeyen ad varsayılana düşer.
func PaletteOf(name string) Palette {
	for _, p := range All() {
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

func isHex(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	_, err := strconv.ParseUint(s[1:], 16, 32)
	return err == nil
}

// ValidColor: "#rrggbb" ya da terminal rengi "0"–"255".
func ValidColor(s string) bool {
	if isHex(s) {
		return true
	}
	n, err := strconv.Atoi(s)
	return err == nil && n >= 0 && n <= 255
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
	if !isHex(p.Ground) { // terminal renkleri: karışım hesaplanamaz
		p.AccentHi = mix(hex, "#ffffff", 0.25)
		return p
	}
	if luminance(p.Ground) > 0.5 { // açık tema
		p.AccentHi = mix(hex, "#000000", 0.15)
		p.Selected = mix(hex, p.Ground, 0.82)
	} else {
		p.AccentHi = mix(hex, "#ffffff", 0.25)
		p.Selected = mix(hex, p.Ground, 0.75)
	}
	return p
}

// Apply, temayı ve vurgu rengini uygular (öbür ayarlar olduğu gibi kalır).
func Apply(themeName, accent string) {
	o := cur
	o.Theme, o.Accent = themeName, accent
	Configure(o)
}

func col(s string) color.Color {
	if s == "" {
		return nil
	}
	return lipgloss.Color(s)
}

// Configure, bütün görünüm ayarlarını uygular.
func Configure(o Options) {
	cur = o
	p := Resolve(o.Theme, o.Accent)
	over := map[string]string{}
	for k, v := range o.Colors {
		if ValidColor(v) {
			over[k] = v
		}
	}
	pick := func(key, def string) string {
		if v, ok := over[key]; ok {
			return v
		}
		return def
	}
	p.Ground, p.Text, p.Muted, p.Line = pick("ground", p.Ground), pick("text", p.Text), pick("muted", p.Muted), pick("line", p.Line)
	p.Accent, p.AccentHi, p.Selected = pick("accent", p.Accent), pick("accent_hi", p.AccentHi), pick("selected", p.Selected)
	p.Gold, p.Red = pick("gold", p.Gold), pick("red", p.Red)

	Ground, Text, Muted, Line = col(p.Ground), col(p.Text), col(p.Muted), col(p.Line)
	Accent, AccentHi, Selected = col(p.Accent), col(p.AccentHi), col(p.Selected)
	Gold, Red = col(p.Gold), col(p.Red)

	s := lipgloss.NewStyle
	fg := func(c color.Color) lipgloss.Style {
		if c == nil {
			return s()
		}
		return s().Foreground(c)
	}
	elem := func(key string, def color.Color) lipgloss.Style {
		if v, ok := over[key]; ok {
			return fg(col(v))
		}
		return fg(def)
	}
	Plain = fg(Text)
	Dim = fg(Muted)
	Faint = fg(Line)
	Title = elem("title", Text).Italic(true)
	Brand = elem("brand", AccentHi).Italic(true)
	Hi = fg(AccentHi)
	GoldText = fg(Gold)
	Error = fg(Red)
	Bold = fg(Text).Bold(true)
	SelectedRow = elem("selected_text", AccentHi)
	if Selected != nil {
		SelectedRow = SelectedRow.Background(Selected)
	} else {
		SelectedRow = SelectedRow.Reverse(true)
	}
	Playing = elem("playing", Gold)
	ProgressDone = elem("progress", AccentHi)
	ProgressLeft = fg(Line)
	HintKey = elem("hint_key", Muted)
	HintText = elem("hint_text", Line)
	Border = elem("border", Line)

	Icon = iconSets[o.Icons]
	if Icon.Play == "" {
		Icon = iconSets["unicode"]
	}
	ruleChar = "─"
	if r, ok := rules[o.Border]; ok {
		ruleChar = r
	}
	bar = bars["line"]
	if b, ok := bars[o.Progress]; ok {
		bar = b
	}
}

// Swatch, renk kareleri (önizleme): verilen her renkten bir ■.
func Swatch(hexes ...string) string {
	out := ""
	for _, h := range hexes {
		if h == "" {
			out += "■"
			continue
		}
		out += lipgloss.NewStyle().Foreground(lipgloss.Color(h)).Render("■")
	}
	return out
}

// ---- simgeler ----

// IconSet, arayüzdeki işaretler.
type IconSet struct {
	Play, Pause, Buffering, Idle string // alt şeritteki düğme
	Note                         string // çalan şarkı / bağlam
	Shuffle, Repeat, RepeatOne   string
	Heart, Moon, Folder          string
	Muted                        string
}

// Icon, seçili simge takımı.
var Icon IconSet

var iconSets = map[string]IconSet{
	"unicode": {Play: "▶", Pause: "❚❚", Buffering: "…", Idle: "·", Note: "♪", Shuffle: "⇄", Repeat: "↻", RepeatOne: "↻1",
		Heart: "♥", Moon: "☾", Folder: "▸", Muted: "✕"},
	"ascii": {Play: ">", Pause: "||", Buffering: "..", Idle: "-", Note: "*", Shuffle: "S", Repeat: "R", RepeatOne: "R1",
		Heart: "<3", Moon: "z", Folder: ">", Muted: "x"},
	// Nerd Font (Material Design) simgeleri.
	"nerd": {Play: "\U000F040A", Pause: "\U000F03E4", Buffering: "\U000F0772", Idle: "\U000F04DB", Note: "\U000F0388",
		Shuffle: "\U000F049D", Repeat: "\U000F0456", RepeatOne: "\U000F0458", Heart: "\U000F02D1", Moon: "\U000F0594",
		Folder: "\U000F024B", Muted: "\U000F075F"},
}

// IconSets, seçilebilecek simge takımları.
var IconSets = []string{"unicode", "ascii", "nerd"}

// ---- çizgiler ----

var rules = map[string]string{"line": "─", "heavy": "━", "double": "═", "dashed": "╌", "ascii": "-", "none": " "}

// BorderKinds, seçilebilecek çizgi türleri.
var BorderKinds = []string{"line", "heavy", "double", "dashed", "ascii", "none"}

var ruleChar = "─"

// Rule, w genişliğinde yatay çizgi.
func Rule(w int) string { return Border.Render(strings.Repeat(ruleChar, max(w, 0))) }

// ---- ilerleme çubuğu ----

type barStyle struct{ done, head, left string }

var bars = map[string]barStyle{
	"line":  {"━", "", "─"},
	"block": {"█", "", "░"},
	"dots":  {"●", "", "○"},
	"thin":  {"─", "•", "─"},
}

// ProgressKinds, seçilebilecek ilerleme çubukları.
var ProgressKinds = []string{"line", "block", "dots", "thin"}

var bar = bars["line"]

// Bar, w genişliğinde, filled kadarı dolu ilerleme çubuğu.
func Bar(filled, w int) string {
	filled = max(0, min(w, filled))
	if bar.head != "" && w > 0 {
		head := min(filled, w-1)
		return ProgressDone.Render(strings.Repeat(bar.done, head)+bar.head) + ProgressLeft.Render(strings.Repeat(bar.left, w-head-1))
	}
	return ProgressDone.Render(strings.Repeat(bar.done, filled)) + ProgressLeft.Render(strings.Repeat(bar.left, w-filled))
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

// Solid, zemin rengi; tema terminalin zeminini kullanıyorsa siyah (resim
// çizimi gibi bilinen bir renk gereken yerler için).
func Solid() color.Color {
	if Ground == nil {
		return lipgloss.Color("#000000")
	}
	return Ground
}

// ValidLook: adı verilen tema var mı (hazır ya da kullanıcının).
func ValidLook(name string) bool {
	for _, p := range All() {
		if p.Name == name {
			return true
		}
	}
	return false
}
