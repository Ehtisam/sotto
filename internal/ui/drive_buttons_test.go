package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// Şerit düğmeleri: Ayarlar'dan açılır; ekranda görülen yerlerine tıklamak
// önceki/sonraki/duraklat/karıştır/tekrar isteklerini yollar. Kutulu düzende
// de (şerit bir sütun sağda) doğru düğmeye denk gelir.
func TestDriveStripButtons(t *testing.T) {
	d, f := driveOffline(t)
	f.on("GET me/player?additional_types", `{"device":{"id":"ph","name":"Phone","volume_percent":50},"is_playing":true,"progress_ms":1000,
		"shuffle_state":false,"repeat_state":"off",
		"item":{"name":"Song 01","uri":"spotify:track:t1","type":"track","duration_ms":200000,"artists":[{"name":"Artist 1"}],"album":{"name":"Album"}}}`)
	d.do("<wait:2000>")
	if s := d.screen(); strings.Contains(s, "▸▸") {
		t.Fatal("buttons shown before they are turned on")
	}
	// find, ekranda (son satırlarda) bir yazının yerini bulur.
	find := func(text string) (int, int) {
		lines := strings.Split(d.screen(), "\n")
		for y := len(lines) - 1; y >= 0; y-- {
			plain := ansi.Strip(lines[y])
			if i := strings.Index(plain, text); i >= 0 {
				return ansi.StringWidth(plain[:i]), y
			}
		}
		t.Fatalf("%q not on screen:\n%s", text, d.screen())
		return 0, 0
	}
	click := func(x, y int) {
		d.raw(fmt.Sprintf("\x1b[<0;%d;%dM\x1b[<0;%d;%dm", x+1, y+1, x+1, y+1))
		d.do("<wait:300>")
	}
	turnOn := func(id settingID) {
		d.do("g,<wait:300>")
		for i := 0; i < 40 && d.model().settingSelected() != id; i++ {
			d.do("<down><wait:40>")
		}
		d.do("<right><wait:300>")
	}
	turnOn(setButtons)
	if !d.model().cfg.Look.Buttons {
		t.Fatal("buttons setting not on")
	}
	check := func(what, req string) {
		t.Helper()
		if len(f.sent(req)) == 0 {
			t.Fatalf("%s: no %q; sent %v", what, req, f.sent(""))
		}
	}
	click(find("▸▸"))
	check("next", "POST me/player/next")
	click(find("◂◂"))
	check("previous", "POST me/player/previous")
	click(find("❚❚"))
	check("pause", "PUT me/player/pause")
	click(find("⇄"))
	check("shuffle", "PUT me/player/shuffle?state=true")
	click(find("↻"))
	check("repeat", "PUT me/player/repeat")
	d.t.Log("\n" + d.screen())

	// Kutulu düzende yeniden: önceki isteği sayıp bir daha tıkla.
	turnOn(setLayout)
	before := len(f.sent("POST me/player/next"))
	click(find("▸▸"))
	if len(f.sent("POST me/player/next")) != before+1 {
		t.Fatalf("boxed: next not sent once more\n%s", d.screen())
	}
	// Şarkının adına tıklamak yine Şimdi çalıyor'a götürür.
	click(find("Song 01 —"))
	if d.model().page != pageNow {
		t.Fatalf("title click: page %v", d.model().page)
	}
}
