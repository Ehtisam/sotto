package ui

import (
	"strings"
	"testing"
	"time"
)

// Gerçek hesap, yalnız okuma: Spotify'da sabitlenenler Kitaplık'ta üstte ✦ ile.
// SOTTO_DRIVE=1 go test ./internal/ui -run DrivePinsReal -v
func TestDrivePinsReal(t *testing.T) {
	d := drive(t)
	d.do("gl")
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) && len(d.model().lib.pins) == 0 {
		d.do("<wait:500>")
	}
	m := d.model()
	if !m.cfg.Extras.Enabled {
		t.Skip("extras are off")
	}
	if len(m.lib.pins) == 0 {
		t.Fatalf("no pins read\n%s", d.screen())
	}
	vis := m.libVisible()
	if len(vis) == 0 || !vis[0].pinned {
		t.Fatalf("first row not pinned: %+v", vis[:min(3, len(vis))])
	}
	if s := d.screen(); !strings.Contains(s, "✦") {
		t.Fatalf("pin mark not drawn:\n%s", s)
	}
	t.Log("\n" + d.screen())
}
