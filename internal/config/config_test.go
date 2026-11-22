package config

import (
	"os"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

// Save'in yazdığı dosya geri okununca aynı ayarları vermeli.
func TestSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	c := defaults()
	c.ConfigDir = dir
	c.Language = "tr"
	c.Look = Look{Theme: "paper", Accent: "teal", Covers: "off"}
	c.Soloist.Autostart, c.Soloist.InitialVolume = false, 55
	c.Extras.Enabled = false
	c.Spotify.ClientID = `a"b`
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(c.ConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "[look]") {
		t.Fatal("look section missing")
	}
	got := defaults()
	if err := toml.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Language != "tr" || got.Look != c.Look || got.Soloist.Autostart || got.Soloist.InitialVolume != 55 ||
		got.Extras.Enabled || got.Spotify.ClientID != `a"b` {
		t.Fatalf("round trip = %+v", got)
	}
	if got.Look.CoversOn() {
		t.Fatal("covers should be off")
	}
}

// Soloist ses ayarları ve açılış davranışı yazılıp geri okunur.
func TestSaveSoloistPlayback(t *testing.T) {
	dir := t.TempDir()
	c := defaults()
	c.ConfigDir = dir
	c.Soloist.OnStart, c.Soloist.Crossfade, c.Soloist.Quality, c.Soloist.Normalize = "play", 8, "high", false
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(c.ConfigFile())
	if strings.Contains(string(b), "resume =") {
		t.Fatal("old resume key written")
	}
	got := defaults()
	if err := toml.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	got.Soloist.normalize()
	if got.Soloist.OnStart != "play" || got.Soloist.Crossfade != 8 || got.Soloist.Quality != "high" || got.Soloist.Normalize {
		t.Fatalf("round trip = %+v", got.Soloist)
	}
}

// Eski dosyalardaki "resume" on_start'a çevrilir; bozuk değerler düzelir.
func TestSoloistNormalize(t *testing.T) {
	for _, tc := range []struct {
		file string
		want string
	}{
		{"[soloist]\nresume = false\n", "off"},
		{"[soloist]\nresume = true\n", "paused"},
		{"[soloist]\n", "paused"},
		{"[soloist]\non_start = \"play\"\n", "play"},
		{"[soloist]\non_start = \"loud\"\ncrossfade = 40\nquality = \"x\"\n", "paused"},
	} {
		c := defaults()
		if err := toml.Unmarshal([]byte(tc.file), &c); err != nil {
			t.Fatal(err)
		}
		c.Soloist.normalize()
		if c.Soloist.OnStart != tc.want || c.Soloist.Resume != nil || c.Soloist.Crossfade > 12 || c.Soloist.Quality != "very_high" {
			t.Errorf("%q → %+v", tc.file, c.Soloist)
		}
	}
}

func TestLanguageFrom(t *testing.T) {
	for in, want := range map[string]string{
		"tr-TR": "tr", "tr_TR.UTF-8": "tr", "TR": "tr", "tr.UTF-8": "tr",
		"en-US": "en", "de_DE.UTF-8": "en", "": "en", "trk": "en",
	} {
		if got := languageFrom(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
	t.Logf("this system: %q → %s", systemLocale(), SystemLanguage())
}
