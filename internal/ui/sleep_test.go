package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/Ehtisam/sotto/internal/soloist"
)

func sleepModel(t *testing.T) Model {
	t.Helper()
	m := queueModel(t)
	m.state.Status = soloist.StatusPlaying
	m.state.Item.Decorations.Playback.DurationMS = 200_000
	m.state.Position = soloist.Position{PositionMS: 10_000}
	return m
}

func TestSleepCycle(t *testing.T) {
	m := sleepModel(t)
	var seen []string
	for range len(sleepSteps) + 1 {
		mm, _ := m.Update(keyPress("z"))
		m = mm.(Model)
		seen = append(seen, m.flash)
	}
	if !strings.Contains(seen[0], "15 minutes") || !strings.Contains(seen[5], "song ends") || !strings.Contains(seen[6], "off") {
		t.Fatalf("notes = %q", seen)
	}
	if m.sleep.on() {
		t.Fatal("should be off after a full cycle")
	}
}

func TestSleepTimerStops(t *testing.T) {
	m := sleepModel(t).cycleSleep() // 15 dk
	if !strings.Contains(m.sleepMark(), "15:00") {
		t.Fatalf("mark = %q", m.sleepMark())
	}
	if _, cmd := m.sleepTick(); cmd != nil {
		t.Fatal("stopped too early")
	}
	m.now = m.now.Add(15 * time.Minute)
	m, cmd := m.sleepTick()
	if cmd == nil || m.sleep.on() {
		t.Fatal("did not stop when time was up")
	}
}

func TestSleepAtSongEnd(t *testing.T) {
	m := sleepModel(t)
	for range len(sleepSteps) {
		m = m.cycleSleep()
	}
	if m.sleep.track != "spotify:track:now" {
		t.Fatalf("track mode: %+v", m.sleep)
	}
	if _, cmd := m.sleepTick(); cmd != nil {
		t.Fatal("stopped mid-song")
	}
	// Şarkının son saniyesi.
	m.state.Position = soloist.Position{PositionMS: 199_000}
	if mm, cmd := m.sleepTick(); cmd == nil || mm.sleep.on() {
		t.Fatal("did not stop at the end of the song")
	}
	// Şarkı atlandıysa da durur.
	m.state.Position = soloist.Position{PositionMS: 0}
	m.state.Item = entity("other")
	if _, cmd := m.sleepTick(); cmd == nil {
		t.Fatal("did not stop after the song changed")
	}
}
