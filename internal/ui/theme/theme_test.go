package theme

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const itermSample = `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
<key>Ansi 1 Color</key><dict><key>Blue Component</key><real>0.2</real><key>Green Component</key><real>0.2</real><key>Red Component</key><real>0.8</real></dict>
<key>Ansi 3 Color</key><dict><key>Blue Component</key><real>0</real><key>Green Component</key><real>0.8</real><key>Red Component</key><real>0.9</real></dict>
<key>Ansi 5 Color</key><dict><key>Blue Component</key><real>0.8</real><key>Green Component</key><real>0.3</real><key>Red Component</key><real>0.6</real></dict>
<key>Background Color</key><dict><key>Blue Component</key><real>0.1</real><key>Green Component</key><real>0.1</real><key>Red Component</key><real>0.1</real></dict>
<key>Foreground Color</key><dict><key>Blue Component</key><real>0.9</real><key>Green Component</key><real>0.9</real><key>Red Component</key><real>0.9</real></dict>
</dict></plist>`

const alacrittyTOML = `
[colors.primary]
background = "#1d1f21"
foreground = "0xc5c8c6"
[colors.normal]
red = "#cc6666"
yellow = "#f0c674"
magenta = "#b294bb"
[colors.bright]
magenta = "#c397d8"
`

const alacrittyYAML = `colors:
  primary:
    background: '#282a36'
    foreground: '#f8f8f2'
  normal:
    red:     '#ff5555'
    yellow:  '#f1fa8c'
    magenta: '#bd93f9'
`

const wtJSON = `{"name": "Campbell", "background": "#0C0C0C", "foreground": "#CCCCCC",
 "red": "#C50F1F", "yellow": "#C19C00", "purple": "#881798", "brightPurple": "#B4009E"}`

func TestParseSchemes(t *testing.T) {
	cases := []struct {
		ext, data         string
		ground, text, acc string
	}{
		{".itermcolors", itermSample, "#1a1a1a", "#e6e6e6", "#994dcc"},
		{".toml", alacrittyTOML, "#1d1f21", "#c5c8c6", "#b294bb"},
		{".yml", alacrittyYAML, "#282a36", "#f8f8f2", "#bd93f9"},
		{".json", wtJSON, "#0c0c0c", "#cccccc", "#881798"},
	}
	for _, c := range cases {
		p, err := ParseScheme(c.ext, []byte(c.data))
		if err != nil {
			t.Fatalf("%s: %v", c.ext, err)
		}
		if p.Ground != c.ground || p.Text != c.text || p.Accent != c.acc {
			t.Errorf("%s: %+v", c.ext, p)
		}
		for _, v := range p.values() {
			if !isHex(v) {
				t.Errorf("%s: bad colour %q in %+v", c.ext, v, p)
			}
		}
	}
	if _, err := ParseScheme(".txt", nil); err == nil {
		t.Error("unknown type accepted")
	}
	if _, err := ParseScheme(".json", []byte(`{"name":"x"}`)); err == nil {
		t.Error("scheme without colours accepted")
	}
}

func TestImportAndLoadUserThemes(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(t.TempDir(), "Dracula Pro.yml")
	os.WriteFile(src, []byte(alacrittyYAML), 0o600)
	name, path, err := Import(dir, src, "")
	if err != nil || name != "dracula-pro" {
		t.Fatalf("import: %q %v", name, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { SetUser(nil) })
	if errs := LoadUser(dir); len(errs) != 0 {
		t.Fatal(errs)
	}
	if PaletteOf("dracula-pro").Ground != "#282a36" {
		t.Fatal("user theme not found")
	}
	// Hazır bir temanın adıyla içe aktarılan "-custom" alır.
	name, _, _ = Import(dir, src, "cardo")
	if name != "cardo-custom" {
		t.Fatalf("name clash: %q", name)
	}
	// Elle değiştirilen dosya okunur.
	os.WriteFile(path, []byte("ground = \"#000000\"\naccent = \"nope\"\n"), 0o600)
	LoadUser(dir)
	p := PaletteOf("dracula-pro")
	if p.Ground != "#000000" || p.Accent != Presets[0].Palette.Accent {
		t.Fatalf("edited theme %+v", p)
	}
}

func TestConfigureOverridesAndIcons(t *testing.T) {
	t.Cleanup(func() { Configure(Options{Theme: "cardo"}) })
	Configure(Options{Theme: "night", Colors: map[string]string{"progress": "#ff0000", "accent": "#00ff00", "text": "bad"},
		Icons: "ascii", Border: "double", Progress: "block"})
	if Icon.Pause != "||" || !strings.Contains(Rule(3), "═══") {
		t.Fatalf("icons %q rule %q", Icon.Pause, Rule(3))
	}
	if r, g, _, _ := Accent.RGBA(); r != 0 || g>>8 != 255 {
		t.Fatal("accent override not applied")
	}
	if r, _, _, _ := Text.RGBA(); r>>8 != 0xdd {
		t.Fatal("a bad colour should keep the theme's")
	}
	if b := Bar(2, 5); !strings.Contains(b, "██") || !strings.Contains(b, "░░░") {
		t.Fatalf("bar %q", b)
	}
	Configure(Options{Theme: "terminal"})
	if Ground != nil || Text != nil {
		t.Fatal("terminal theme should leave ground and text to the terminal")
	}
	Configure(Options{Theme: "terminal", Accent: "teal"}) // karışım hesaplanamaz ama çökmez
	Configure(Options{Theme: "cardo", Progress: "thin"})
	if b := Bar(0, 4); !strings.Contains(b, "•") {
		t.Fatalf("thin bar %q", b)
	}
}
