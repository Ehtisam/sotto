package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/Ehtisam/sotto/internal/history"
)

// Dinleme geçmişi: çalan şarkı yeterince dinlenip değişince kaydedilir;
// g y sayfası özet, dağılım, en çok dinlenenler ve son dinlemeleri gösterir;
// tab aralığı değiştirir, enter şarkıyı çalar.
func TestDriveHistory(t *testing.T) {
	d, f := driveOffline(t)
	dir := d.model().cfg.LocalDir
	// Eski bir kayıt: yalnız "hepsi" aralığında görünür.
	_ = history.Append(dir, history.Play{At: time.Now().AddDate(-2, 0, 0), URI: "spotify:track:old", Title: "Old Song", Artist: "Old Band", MS: 200000})

	player := func(uri, name string) string {
		return `{"device":{"id":"ph","name":"Phone","volume_percent":50},"is_playing":true,"progress_ms":1000,
		"item":{"name":"` + name + `","uri":"` + uri + `","type":"track","duration_ms":10000,"artists":[{"name":"Artist 1","uri":"spotify:artist:a1"}],"album":{"name":"Album"}}}`
	}
	f.on("GET me/player?additional_types", player("spotify:track:t1", "Song 01"))
	d.do("<wait:7000>") // 10 sn'lik şarkının yarısından fazlası
	f.on("GET me/player?additional_types", player("spotify:track:t2", "Song 02"))
	d.do("<wait:2500>")
	plays, _ := history.Load(dir)
	if len(plays) != 2 || plays[1].URI != "spotify:track:t1" || plays[1].MS < 5000 || plays[1].Device != "Phone" {
		t.Fatalf("not recorded: %+v", plays)
	}

	d.do("gy<wait:400>")
	s := d.screen()
	for _, want := range []string{"Listening history", "last 7 days", "1 plays", "By hour of the day", "Most played songs", "Song 01"} {
		if !strings.Contains(s, want) {
			t.Fatalf("history page lacks %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "Old Song") {
		t.Fatalf("two-year-old play in the week view:\n%s", s)
	}
	// Seçili ilk satır en çok dinlenen şarkı: enter çalar.
	d.do("<enter><wait:500>")
	if len(f.sent("PUT me/player/play")) == 0 {
		t.Fatalf("enter did not play; sent %v", f.sent("PUT"))
	}
	// tab: ay, yıl, hepsi → eski kayıt görünür.
	d.do("<tab><wait:100><tab><wait:100><tab><wait:300>")
	if s = d.screen(); !strings.Contains(s, "2 plays") {
		t.Fatalf("all-time view:\n%s", s)
	}
	if m := d.model(); m.extra.hist.rng != 3 {
		t.Fatalf("range %d", m.extra.hist.rng)
	}
	if strings.Contains(s, "pick a line") {
		t.Fatalf("lyrics hints on the history page:\n%s", s)
	}
	d.t.Log("\n" + s)
	d.do("<esc><wait:200>")
	if d.model().extraOn() {
		t.Fatal("esc should close the history page")
	}
}
