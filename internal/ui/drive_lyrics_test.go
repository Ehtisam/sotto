package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Ehtisam/sotto/internal/lyrics"
)

// Sözlerde satıra atlama: Şimdi çalıyor'da enter sözlere girer (söylenen
// satır seçili), ↓ ile satır seçilir, enter o satırın başına sarar, esc çıkar.
// Çevrimdışı: müzik sahte bir telefonda çalıyor (kumanda modu).
func TestDriveLyricsJump(t *testing.T) {
	d, f := driveOffline(t)
	var lines []lyrics.Line
	for i := 0; i < 12; i++ {
		lines = append(lines, lyrics.Line{Start: time.Duration(i) * 10 * time.Second, Text: fmt.Sprintf("line number %d", i)})
	}
	// Sözler şarkıdan önce gelsin ki ağa gidilmesin.
	d.p.Send(lyricsMsg{uri: "spotify:track:t1", lyrics: &lyrics.Lyrics{Synced: true, Lines: lines, Source: "Test"}})
	f.on("GET me/player?additional_types", `{"device":{"id":"ph","name":"Phone","volume_percent":50},"is_playing":true,"progress_ms":31000,
		"item":{"name":"Song 01","uri":"spotify:track:t1","type":"track","duration_ms":200000,"artists":[{"name":"Artist 1"}],"album":{"name":"Album"}}}`)
	d.do("1<wait:2500>")
	if s := d.screen(); !strings.Contains(s, "line number 3") || !strings.Contains(s, "jump in the lyrics") {
		t.Fatalf("lyrics or enter hint not shown:\n%s", s)
	}
	d.do("<enter><wait:200>")
	m := d.model()
	if m.focus != focusContent || !m.lyr.pickOn || m.lyr.pick != 3 {
		t.Fatalf("enter: focus=%v pickOn=%v pick=%d", m.focus, m.lyr.pickOn, m.lyr.pick)
	}
	if s := d.screen(); !strings.Contains(s, "pick a line") {
		t.Fatalf("hints lack lyrics keys:\n%s", s)
	}
	d.do("<down><wait:80><down><wait:80>j<wait:200>")
	if m = d.model(); m.lyr.pick != 6 {
		t.Fatalf("pick = %d, want 6", m.lyr.pick)
	}
	d.do("<enter><wait:600>")
	if got := f.sent("PUT me/player/seek"); len(got) != 1 || !strings.HasPrefix(got[0], "PUT me/player/seek?position_ms=60000") {
		t.Fatalf("want one seek to 60 s, got %v", got)
	}
	if m = d.model(); m.lyr.pickOn || m.focus != focusContent {
		t.Fatalf("after jump: pickOn=%v focus=%v", m.lyr.pickOn, m.focus)
	}
	// Seçim yokken esc menüye döner; k ile yeniden seçilir, esc seçimi bırakır.
	d.do("k<wait:150>")
	if !d.model().lyr.pickOn {
		t.Fatal("k should start picking again")
	}
	d.do("<esc><wait:150>")
	if m = d.model(); m.lyr.pickOn || m.focus != focusContent {
		t.Fatal("esc should drop the pick first")
	}
	d.do("<esc><wait:150>")
	if d.model().focus != focusMenu {
		t.Fatal("second esc should go back to the menu")
	}
	d.t.Log("\n" + d.screen())
}
