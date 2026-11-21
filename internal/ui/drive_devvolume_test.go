package ui

import (
	"strings"
	"testing"
)

// Cihaz başına ses: Cihazlar sayfasında satırlarda ses yazar; + / - seçili
// cihazın sesini değiştirir (çalmayan cihaz da), art arda basışlar tek istek
// olur; sesi uzaktan değişmeyen cihazda istek gitmez, nedeni söylenir.
func TestDriveDeviceVolume(t *testing.T) {
	d, f := driveOffline(t)
	f.on("GET me/player/devices", `{"devices":[
		{"id":"ph","name":"Phone","type":"Smartphone","is_active":true,"volume_percent":50,"supports_volume":true},
		{"id":"tv","name":"Living room TV","type":"TV","is_active":false,"volume_percent":30,"supports_volume":true},
		{"id":"sp","name":"Kitchen","type":"Speaker","is_active":false,"volume_percent":70,"supports_volume":false}]}`)
	d.do("6<wait:800>")
	if m := d.model(); m.page != pageDevices || m.focus != focusContent {
		t.Fatalf("6 → page %v focus %v", m.page, m.focus)
	}
	s := d.screen()
	for _, want := range []string{" 50%", " 30%", "+ / - change the selected device"} {
		if !strings.Contains(s, want) {
			t.Fatalf("devices page lacks %q:\n%s", want, s)
		}
	}
	// İmleci TV'ye götür (ilk satır kapalı Soloist).
	rows := d.model().deviceRows()
	for i, r := range rows {
		if r.name == "Living room TV" {
			for range i - d.model().devs.cursor {
				d.do("j<wait:60>")
			}
		}
	}
	if r := d.model().deviceRows()[d.model().devs.cursor]; r.name != "Living room TV" {
		t.Fatalf("cursor on %q", r.name)
	}
	d.do("+<wait:60>+<wait:60>-<wait:60>+<wait:700>")
	got := f.sent("PUT me/player/volume")
	if len(got) != 1 || !strings.Contains(got[0], "volume_percent=40") || !strings.Contains(got[0], "device_id=tv") {
		t.Fatalf("volume requests = %v", got)
	}
	if s := d.screen(); !strings.Contains(s, " 40%") {
		t.Fatalf("row not updated:\n%s", s)
	}
	// Sesi uzaktan değişmeyen cihaz.
	d.do("j<wait:60>+<wait:600>")
	if got := f.sent("PUT me/player/volume"); len(got) != 1 {
		t.Fatalf("request sent to a fixed-volume device: %v", got)
	}
	if m := d.model(); !strings.Contains(m.flash, "Kitchen") {
		t.Fatalf("flash = %q", m.flash)
	}
}
