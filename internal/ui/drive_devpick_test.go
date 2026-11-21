package ui

import (
	"strings"
	"testing"
)

// Hızlı cihaz seçici: d başka bir sayfadayken açılır, çalan cihaz seçili
// başlar, j ile sonraki cihaz, enter müziği oraya taşır; esc ve d kapatır.
func TestDriveDevicePicker(t *testing.T) {
	d, f := driveOffline(t)
	f.on("GET me/player/devices", `{"devices":[{"id":"ph","name":"Phone","type":"Smartphone","is_active":true},{"id":"tv","name":"Living room TV","type":"TV","is_active":false}]}`)
	d.do("gl<wait:300>d<wait:500>")
	s := d.screen()
	for _, want := range []string{"Play on", "Phone", "Living room TV", "sotto", "enter moves the music there"} {
		if !strings.Contains(s, want) {
			t.Fatalf("picker lacks %q:\n%s", want, s)
		}
	}
	if m := d.model(); !m.pick.open || m.page != pageLibrary {
		t.Fatalf("picker open=%v page=%v", m.pick.open, m.page)
	}
	// Liste seçici açıldıktan sonra geliyor: imleç yine çalan cihazda olmalı.
	if m := d.model(); m.deviceRows()[m.pick.cursor].name != "Phone" {
		t.Fatalf("cursor on %q, want the playing device", m.deviceRows()[m.pick.cursor].name)
	}
	d.do("j<wait:80><enter><wait:600>")
	if m := d.model(); m.pick.open {
		t.Fatal("enter should close the picker")
	}
	got := f.sent("PUT me/player")
	found := false
	for _, r := range got {
		if strings.Contains(f.bodyOf(r), `"tv"`) {
			found = true
		}
	}
	if !found {
		t.Fatalf("no transfer to tv: %v", got)
	}
	if s = d.screen(); strings.Contains(s, "Play on") {
		t.Fatalf("picker still drawn:\n%s", s)
	}
	// d ile açılıp esc ve d ile kapanır; açıkken başka tuşlar arkadaki sayfaya gitmez.
	d.do("d<wait:200><esc><wait:100>")
	if d.model().pick.open {
		t.Fatal("esc should close")
	}
	d.do("d<wait:200>n<wait:100>d<wait:100>")
	if m := d.model(); m.pick.open || m.form.on {
		t.Fatalf("d should close; n must not open the new-playlist form (form=%v)", m.form.on)
	}
}
