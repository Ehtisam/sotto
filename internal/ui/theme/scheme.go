package theme

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Terminal renk şemalarından tema: iTerm2 (.itermcolors), Alacritty (.toml,
// .yml) ve Windows Terminal (bir şemanın JSON'u). Şemanın zemini, yazısı ve
// 16 rengi sotto'nun paletine çevrilir; sonuç %APPDATA%\sotto\themes\<ad>.toml
// olarak saklanır ve Ayarlar'daki temalar arasında görünür.

// scheme, şemanın sotto'nun kullandığı renkleri.
type scheme struct {
	bg, fg string
	ansi   [16]string // 0–7 normal, 8–15 parlak
}

var ansiNames = []string{"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white"}

// ParseScheme, bir şema dosyasının içeriğini palete çevirir. ext dosya
// uzantısıdır (".itermcolors", ".toml", ".yml", ".yaml", ".json").
func ParseScheme(ext string, data []byte) (Palette, error) {
	var s scheme
	var err error
	switch strings.ToLower(ext) {
	case ".itermcolors", ".plist", ".xml":
		s, err = parseITerm(data)
	case ".toml":
		s, err = parseAlacrittyTOML(data)
	case ".yml", ".yaml":
		s, err = parseAlacrittyYAML(data)
	case ".json":
		s, err = parseWindowsTerminal(data)
	default:
		return Palette{}, fmt.Errorf("unknown scheme type %q (use .itermcolors, Alacritty .toml/.yml or a Windows Terminal .json)", ext)
	}
	if err != nil {
		return Palette{}, err
	}
	return s.palette()
}

func (s scheme) palette() (Palette, error) {
	if !isHex(s.bg) || !isHex(s.fg) {
		return Palette{}, errors.New("the scheme has no background or foreground colour")
	}
	pick := func(i int, def string) string {
		if isHex(s.ansi[i]) {
			return s.ansi[i]
		}
		return def
	}
	accent := pick(5, pick(4, "#9d6ee0")) // mor, yoksa mavi
	hi := pick(13, mix(accent, s.fg, 0.3))
	light := luminance(s.bg) > 0.5
	sel := mix(accent, s.bg, 0.75)
	if light {
		sel = mix(accent, s.bg, 0.82)
	}
	return Palette{
		Ground: s.bg, Text: s.fg,
		Muted: mix(s.fg, s.bg, 0.38), Line: mix(s.fg, s.bg, 0.7),
		Accent: accent, AccentHi: hi, Selected: sel,
		Gold: pick(3, "#d9a441"), Red: pick(1, "#d9705f"),
	}, nil
}

// ---- iTerm2 ----

func parseITerm(data []byte) (scheme, error) {
	var s scheme
	d := xml.NewDecoder(bytes.NewReader(data))
	var stack []string // açık dict'lerin anahtarları
	var key, cur string
	comps := map[string]map[string]float64{}
	text := ""
	for {
		tok, err := d.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			text = ""
			if t.Name.Local == "dict" {
				stack = append(stack, key)
				if len(stack) == 2 {
					cur = key
					comps[cur] = map[string]float64{}
				}
			}
		case xml.CharData:
			text += string(t)
		case xml.EndElement:
			switch t.Name.Local {
			case "key":
				key = strings.TrimSpace(text)
			case "real", "integer":
				if len(stack) == 2 {
					v, _ := strconv.ParseFloat(strings.TrimSpace(text), 64)
					comps[cur][key] = v
				}
			case "dict":
				stack = stack[:len(stack)-1]
			}
		}
	}
	hex := func(name string) string {
		c, ok := comps[name]
		if !ok {
			return ""
		}
		f := func(v float64) int { return max(0, min(255, int(v*255+0.5))) }
		return fmt.Sprintf("#%02x%02x%02x", f(c["Red Component"]), f(c["Green Component"]), f(c["Blue Component"]))
	}
	s.bg, s.fg = hex("Background Color"), hex("Foreground Color")
	for i := 0; i < 16; i++ {
		s.ansi[i] = hex(fmt.Sprintf("Ansi %d Color", i))
	}
	if s.bg == "" {
		return s, errors.New("not an iTerm2 colour scheme")
	}
	return s, nil
}

// ---- Alacritty ----

func normHex(v string) string {
	v = strings.Trim(strings.TrimSpace(v), `"'`)
	if strings.HasPrefix(strings.ToLower(v), "0x") {
		v = "#" + v[2:]
	}
	return strings.ToLower(v)
}

func parseAlacrittyTOML(data []byte) (scheme, error) {
	var f struct {
		Colors struct {
			Primary struct{ Background, Foreground string } `toml:"primary"`
			Normal  map[string]string                       `toml:"normal"`
			Bright  map[string]string                       `toml:"bright"`
		} `toml:"colors"`
	}
	if err := toml.Unmarshal(data, &f); err != nil {
		return scheme{}, err
	}
	var s scheme
	s.bg, s.fg = normHex(f.Colors.Primary.Background), normHex(f.Colors.Primary.Foreground)
	for i, n := range ansiNames {
		s.ansi[i] = normHex(f.Colors.Normal[n])
		s.ansi[8+i] = normHex(f.Colors.Bright[n])
	}
	return s, nil
}

var (
	yamlSection = regexp.MustCompile(`^(\s*)([A-Za-z_]+):\s*$`)
	yamlValue   = regexp.MustCompile(`^(\s*)([A-Za-z_]+):\s*['"]?((?:#|0x)[0-9A-Fa-f]{6})['"]?`)
)

func parseAlacrittyYAML(data []byte) (scheme, error) {
	var s scheme
	section := ""
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		if m := yamlSection.FindStringSubmatch(line); m != nil {
			section = m[2]
			continue
		}
		m := yamlValue.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		k, v := m[2], normHex(m[3])
		switch section {
		case "primary":
			if k == "background" {
				s.bg = v
			} else if k == "foreground" {
				s.fg = v
			}
		case "normal", "bright":
			for i, n := range ansiNames {
				if n == k {
					if section == "bright" {
						i += 8
					}
					s.ansi[i] = v
				}
			}
		}
	}
	return s, nil
}

// ---- Windows Terminal ----

func parseWindowsTerminal(data []byte) (scheme, error) {
	var f map[string]any
	if err := json.Unmarshal(data, &f); err != nil {
		return scheme{}, err
	}
	// settings.json'dan kopyalanan bir şema ya da { "schemes": [ … ] }: ilki.
	if list, ok := f["schemes"].([]any); ok && len(list) > 0 {
		if first, ok := list[0].(map[string]any); ok {
			f = first
		}
	}
	str := func(k string) string {
		v, _ := f[k].(string)
		return normHex(v)
	}
	s := scheme{bg: str("background"), fg: str("foreground")}
	names := []string{"black", "red", "green", "yellow", "blue", "purple", "cyan", "white"}
	for i, n := range names {
		s.ansi[i] = str(n)
		s.ansi[8+i] = str("bright" + strings.ToUpper(n[:1]) + n[1:])
	}
	return s, nil
}

// ---- kullanıcının temaları ----

// UserDir, kullanıcının temalarının klasörü.
func UserDir(configDir string) string { return filepath.Join(configDir, "themes") }

var paletteKeys = []string{"ground", "text", "muted", "line", "accent", "accent_hi", "selected", "gold", "red"}

func (p Palette) values() []string {
	return []string{p.Ground, p.Text, p.Muted, p.Line, p.Accent, p.AccentHi, p.Selected, p.Gold, p.Red}
}

func paletteFrom(m map[string]string, base Palette) Palette {
	v := base.values()
	for i, k := range paletteKeys {
		if c, ok := m[k]; ok && ValidColor(c) {
			v[i] = c
		}
	}
	return Palette{v[0], v[1], v[2], v[3], v[4], v[5], v[6], v[7], v[8]}
}

// ThemeName, dosya adından tema adı: küçük harf, boşluksuz.
func ThemeName(s string) string {
	s = strings.ToLower(strings.TrimSuffix(filepath.Base(s), filepath.Ext(s)))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ', r == '-', r == '_', r == '.':
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// SaveUser, bir temayı themes klasörüne yazar.
func SaveUser(configDir, name, source string, p Palette) (string, error) {
	dir := UserDir(configDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# sotto theme %q", name)
	if source != "" {
		fmt.Fprintf(&b, ", made from %s", filepath.Base(source))
	}
	b.WriteString("\n# Colours as \"#rrggbb\" or a terminal colour number. Change any of them;\n# sotto reads this file when it starts.\n\n")
	for i, v := range p.values() {
		fmt.Fprintf(&b, "%s = %q\n", paletteKeys[i], v)
	}
	path := filepath.Join(dir, name+".toml")
	return path, os.WriteFile(path, []byte(b.String()), 0o600)
}

// LoadUser, themes klasöründeki temaları okur ve tanıtır. Hazır bir temayla
// aynı adı taşıyanlar atlanır. Sorunlu dosyalar hatalarda döner.
func LoadUser(configDir string) []error {
	files, _ := filepath.Glob(filepath.Join(UserDir(configDir), "*.toml"))
	sort.Strings(files)
	var out []Preset
	var errs []error
	builtin := map[string]bool{}
	for _, p := range Presets {
		builtin[p.Name] = true
	}
	for _, f := range files {
		name := ThemeName(f)
		if name == "" || builtin[name] {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		var m map[string]string
		if err := toml.Unmarshal(b, &m); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", filepath.Base(f), err))
			continue
		}
		out = append(out, Preset{Name: name, Palette: paletteFrom(m, Presets[0].Palette), User: true})
	}
	SetUser(out)
	return errs
}

// Import, bir şema dosyasını tema olarak kaydeder ve adını döner.
func Import(configDir, file, name string) (string, string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return "", "", err
	}
	p, err := ParseScheme(filepath.Ext(file), data)
	if err != nil {
		return "", "", err
	}
	if name == "" {
		name = file
	}
	name = ThemeName(name)
	if name == "" {
		return "", "", errors.New("give the theme a name with --name")
	}
	for _, b := range Presets {
		if b.Name == name {
			name += "-custom"
		}
	}
	path, err := SaveUser(configDir, name, file, p)
	if err != nil {
		return "", "", err
	}
	_ = LoadUser(configDir)
	return name, path, nil
}
