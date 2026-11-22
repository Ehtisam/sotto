package ui

import (
	"os"
	"strings"
	"testing"
)

// Ayarlar → Hoparlör: açılış davranışı, geçiş, kalite, eşitleme satırları
// gerçek programda tuşlarla değişir ve ayar dosyasına yazılır. Soloist açık
// değil: "bir sonraki açılışta" notu çıkar, yeniden başlatma olmaz.
func TestDrivePlaybackSettings(t *testing.T) {
	d, _ := driveOffline(t)
	d.do("8<wait:600>")
	if s := d.screen(); !strings.Contains(s, "Settings") {
		t.Fatalf("settings did not open:\n%s", s)
	}
	goTo := func(id settingID) {
		for i := 0; i < 40 && d.model().settingSelected() != id; i++ {
			d.do("<down><wait:60>")
		}
		if d.model().settingSelected() != id {
			t.Fatalf("row %v not reachable", id)
		}
	}
	goTo(setResume)
	// Hoparlör bölümü, imleç ona inince görünür (liste kaydırılır).
	s := d.screen()
	if !strings.Contains(s, "Speaker") {
		t.Fatalf("settings lack the Speaker section:\n%s", s)
	}
	d.do("<right><wait:200>")
	if s = d.screen(); !strings.Contains(s, "play it") {
		t.Fatalf("on start not changed:\n%s", s)
	}
	goTo(setCrossfade)
	d.do("<right><wait:120><right><wait:300>")
	s = d.screen()
	if !strings.Contains(s, "5 s") || !strings.Contains(s, "Applies the next time Soloist starts") {
		t.Fatalf("crossfade row:\n%s", s)
	}
	goTo(setQuality)
	d.do("<left><wait:300>")
	if s = d.screen(); !strings.Contains(s, "high · 160 kbps") {
		t.Fatalf("quality row:\n%s", s)
	}
	goTo(setNormalize)
	if s = d.screen(); !strings.Contains(s, "Even out loudness") { // pencere kaydı
		t.Fatalf("normalize row hidden:\n%s", s)
	}
	d.do("<enter><wait:300>")
	b, err := os.ReadFile(d.model().cfg.ConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`on_start = "play"`, "crossfade = 5", `quality = "high"`, "normalize = true"} { // sürücünün ayarında kapalıydı
		if !strings.Contains(string(b), want) {
			t.Errorf("config lacks %q", want)
		}
	}
	d.t.Log("\n" + d.screen())
}

// Ayarlar'da / ile arama: yazdıkça süzülür, enter sonuca geçer, ← → değiştirir,
// esc süzgeci temizler ve seçili ayarda kalır.
func TestDriveSettingsFind(t *testing.T) {
	d, _ := driveOffline(t)
	d.do("8<wait:500>/<wait:100>cross<wait:300>")
	s := d.screen()
	if !strings.Contains(s, "Crossfade") || strings.Contains(s, "Accent colour") || !strings.Contains(s, "/ cross") {
		t.Fatalf("filter not applied:\n%s", s)
	}
	d.do("<enter><wait:100><right><wait:300>")
	if m := d.model(); m.cfg.Soloist.Crossfade != 3 || m.settingSelected() != setCrossfade {
		t.Fatalf("crossfade = %d, selected %v", m.cfg.Soloist.Crossfade, m.settingSelected())
	}
	d.do("<esc><wait:200>")
	if m := d.model(); m.set.find.active() || m.settingSelected() != setCrossfade {
		t.Fatalf("esc: find=%v selected=%v", m.set.find.active(), m.settingSelected())
	}
	if s = d.screen(); !strings.Contains(s, "Theme") && !strings.Contains(s, "Speaker") {
		t.Fatalf("full list not back:\n%s", s)
	}
	// Bölüm adıyla ve açıklamayla da bulunur; hiçbir şey uymazsa söyler.
	d.do("/<wait:100>desktop<wait:300>")
	if s = d.screen(); !strings.Contains(s, "Song notifications") || strings.Contains(s, "Crossfade") {
		t.Fatalf("section search:\n%s", s)
	}
	d.do("<esc><wait:100>/<wait:100>zzqx<wait:300>")
	if s = d.screen(); !strings.Contains(s, "Nothing in this list matches") {
		t.Fatalf("no-match message missing:\n%s", s)
	}
	d.do("<enter><wait:100><down><wait:100><right><wait:200>") // boş listede tuşlar bir şey bozmasın
	d.do("<esc><wait:200>")
	if d.model().set.find.active() {
		t.Fatal("esc should clear the empty filter")
	}
}
