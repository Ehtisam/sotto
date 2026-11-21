package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Kaydırma çubuğu: 30 şarkılık liste pencereye sığmayınca sağda çubuk
// çıkar, aşağı inince koyu kısım aşağı kayar; satırlar taşmaz.
func TestDriveScrollbar(t *testing.T) {
	d, _ := driveOffline(t)
	d.do("gl<wait:200>j<enter><wait:400>")
	top := d.screen()
	knobAt := func(s string) int {
		for i, line := range strings.Split(s, "\n") {
			if strings.HasSuffix(strings.TrimRight(ansi.Strip(line), " "), "┃") {
				return i
			}
		}
		return -1
	}
	k0 := knobAt(top)
	if k0 < 0 || !strings.Contains(top, "│") {
		t.Fatalf("no scrollbar on a long list:\n%s", top)
	}
	for _, line := range strings.Split(top, "\n") {
		if lipgloss.Width(line) > 110 {
			t.Fatalf("line wider than the window (%d): %q", lipgloss.Width(line), line)
		}
	}
	d.do("G<wait:300>")
	if k1 := knobAt(d.screen()); k1 <= k0 {
		t.Fatalf("knob did not move down: %d → %d\n%s", k0, k1, d.screen())
	}
	// Kısa liste (Kitaplık'ın kendisi, 3 satır): çubuk yok.
	d.do("<esc><wait:300>")
	if s := d.screen(); strings.Contains(s, "┃") {
		t.Fatalf("scrollbar on a short list:\n%s", s)
	}
	d.t.Log("\n" + top)
}
