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
