package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/Ehtisam/sotto/internal/spotify"
)

// o: basıldığı an "geçiliyor" görünür (şeritte ve başlıkta), geçiş bitince
// "artık … üzerinde çalıyor"a döner.
func TestSwitchFeedback(t *testing.T) {
	_, m := newFakeWeb(t)
	m.cfg.Soloist.DeviceName = "sotto"
	m.now = time.Now()
	m.engine = engineOff

	m, cmd := m.toggleSoloist()
	if cmd == nil || m.engine != engineStarting || m.devs.switching != "sotto" {
		t.Fatalf("o: engine %v switching %q", m.engine, m.devs.switching)
	}
	screen := ansi.Strip(m.render())
	if strings.Count(screen, "switching to sotto…") < 1 || !strings.Contains(screen, "Switching to sotto…") && !strings.Contains(screen, "switching to sotto") {
		t.Fatalf("no feedback right after o\n%s", screen)
	}
	// İkinci basış: hâlâ açılıyor, yine söyler (sessiz kalmaz).
	m.flash = ""
	m, _ = m.toggleSoloist()
	if !strings.Contains(m.flash, "Switching to sotto") {
		t.Fatalf("second o: %q", m.flash)
	}
	// Spinner döner.
	a := m.switchingText()
	m.now = m.now.Add(500 * time.Millisecond)
	if b := m.switchingText(); a == b {
		t.Fatalf("spinner did not move: %q", a)
	}
	// Geçiş bitti.
	out, _ := m.Update(transferredMsg{name: "sotto"})
	m = out.(Model)
	if m.devs.switching != "" || !strings.Contains(m.flash, "Now playing on sotto") {
		t.Fatalf("after transfer: switching %q flash %q", m.devs.switching, m.flash)
	}

	// t: kumandaya dönerken cihazın adıyla.
	m.devs.items = []spotify.Device{{ID: "ph", Name: "Phone"}}
	m.devs.switchTo = "ph"
	m.engine = engineReady
	m.haveAuth, m.auth.LoggedIn, m.auth.IsActive = true, true, true
	if m.soloistActive() {
		m, _ = m.switchSpeaker()
		if m.devs.switching != "Phone" {
			t.Fatalf("t: switching %q", m.devs.switching)
		}
	}

	// Hata: yazı kalkar, hata görünür.
	m.devs.switching = "x"
	out, _ = m.Update(engineErrMsg{err: errNotListed})
	if out.(Model).devs.switching != "" {
		t.Fatal("engine error should end the switch")
	}
	// Zaman aşımı.
	m.devs.switching, m.devs.switchAt = "x", m.now.Add(-switchTimeout-time.Second)
	out, _ = m.Update(tickMsg(m.now))
	if out.(Model).devs.switching != "" {
		t.Fatal("switch text should time out")
	}
}
