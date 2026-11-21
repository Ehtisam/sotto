package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// Karşılama: hiçbir şey çalmıyorken Şimdi çalıyor'da büyük yazı ve temel
// tuşlar; bir şey çalmaya başlayınca şarkı görünür.
func TestDriveWelcome(t *testing.T) {
	d, f := driveOffline(t)
	d.do("1<wait:300>")
	s := d.screen()
	for _, want := range []string{"██████", "Spotify player for the terminal", "pick the speaker", "every key", "Soloist"} {
		if !strings.Contains(s, want) {
			t.Fatalf("welcome lacks %q:\n%s", want, s)
		}
	}
	for _, line := range strings.Split(s, "\n") {
		if lipgloss.Width(line) > 110 {
			t.Fatalf("line too wide: %q", line)
		}
	}
	d.t.Log("\n" + s)
	f.on("GET me/player?additional_types", `{"device":{"id":"ph","name":"Phone","volume_percent":50},"is_playing":true,"progress_ms":1000,
		"item":{"name":"Song 01","uri":"spotify:track:t1","type":"track","duration_ms":200000,"artists":[{"name":"Artist 1"}],"album":{"name":"Album"}}}`)
	d.do("<wait:2500>")
	if s = d.screen(); strings.Contains(s, "every key") || !strings.Contains(s, "Song 01") {
		t.Fatalf("welcome should give way to the song:\n%s", s)
	}
}
