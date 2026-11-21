package ui

import (
	"strings"
	"testing"
)

// ? kısayol tablosu: açıklama · tuş · nerede sütunları; / ile süzülür,
// yazılan harfler kısayol olarak çalışmaz; esc önce süzgeci, sonra listeyi kapatır.
func TestDriveHelpTable(t *testing.T) {
	d, _ := driveOffline(t)
	d.do("?<wait:300>")
	s := d.screen()
	for _, want := range []string{"Keys", "key", "where", "everywhere", "in lists", "Now playing"} {
		if !strings.Contains(s, want) {
			t.Fatalf("help lacks %q:\n%s", want, s)
		}
	}
	d.do("/<wait:80>speaker<wait:300>")
	if s = d.screen(); !strings.Contains(s, "pick the speaker from any page") {
		t.Fatalf("speaker filter:\n%s", s)
	}
	d.do("<esc><wait:100>/<wait:80>lyrics<wait:300>")
	s = d.screen()
	if !strings.Contains(s, "/ lyrics") || !strings.Contains(s, "pick a lyrics line") || strings.Contains(s, "pick the speaker") {
		t.Fatalf("filter not applied:\n%s", s)
	}
	if m := d.model(); !m.help || m.lyr.showQueue || m.page != pageNow {
		// "l" yazılırken sözler kısayolu çalışmamalı (sayfa değişmemeli).
		t.Fatalf("typing ran shortcuts: help=%v page=%v", m.help, m.page)
	}
	d.do("<enter><wait:100><esc><wait:150>")
	if m := d.model(); !m.help || m.helpFind.active() {
		t.Fatalf("first esc should only clear the filter (help=%v find=%v)", m.help, m.helpFind.active())
	}
	d.do("<esc><wait:150>")
	if d.model().help {
		t.Fatal("second esc should close help")
	}
	// Dar pencerede yer sütunu düşer ama tablo yine okunur.
	d.do("?<wait:200>")
	d.t.Log("\n" + d.screen())
}
