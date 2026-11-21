package ui

import (
	"os"
	"strings"
	"testing"
)

// Tam ekran görselleştirme: shift+V açar (menü çekilir), → stili değiştirip
// kaydeder, boşluk yine çal/durdur yollar, esc kapatır (menü geri gelir).
// Çevrimdışı: müzik telefonda, bu yüzden "bu bilgisayarda çalan" notu çıkar.
func TestDriveVizFull(t *testing.T) {
	d, f := driveOffline(t)
	f.on("GET me/player?additional_types", `{"device":{"id":"ph","name":"Phone","volume_percent":50},"is_playing":true,"progress_ms":1000,
		"item":{"name":"Song 01","uri":"spotify:track:t1","type":"track","duration_ms":200000,"artists":[{"name":"Artist 1"}],"album":{"name":"Album"}}}`)
	d.do("gl<wait:1800>V<wait:300>")
	s := d.screen()
	for _, want := range []string{"Visualizer", "bars", "listens to music played on this computer", "← → style"} {
		if !strings.Contains(s, want) {
			t.Fatalf("visualizer lacks %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "VIII Settings") {
		t.Fatalf("menu should give way:\n%s", s)
	}
	d.do("<right><wait:200><right><wait:300>")
	if m := d.model(); m.vizStyle() != "scope" {
		t.Fatalf("style %q, want scope", m.vizStyle())
	}
	if b, _ := os.ReadFile(d.model().cfg.ConfigFile()); !strings.Contains(string(b), `viz_style = "scope"`) {
		t.Fatal("style not saved")
	}
	if s = d.screen(); !strings.Contains(s, "oscilloscope") {
		t.Fatalf("style name not shown:\n%s", s)
	}
	d.do(" <wait:400>")
	if len(f.sent("PUT me/player/pause")) == 0 {
		t.Fatalf("space should still pause; sent %v", f.sent("PUT"))
	}
	d.do("<esc><wait:300>")
	if m := d.model(); m.vizFull || m.page != pageLibrary {
		t.Fatalf("esc: vizFull=%v page=%v", m.vizFull, m.page)
	}
	if s = d.screen(); !strings.Contains(s, "VIII Settings") {
		t.Fatalf("menu should be back:\n%s", s)
	}
	// ? listesinde yeni tuş görünür.
	d.do("?<wait:200>/<wait:50>visual<wait:300>")
	if s = d.screen(); !strings.Contains(s, "full-screen visualizer") || !strings.Contains(s, "V") {
		t.Fatalf("help lacks the visualizer key:\n%s", s)
	}
}
