package ui

import (
	"testing"
	"time"

	"github.com/Ehtisam/sotto/internal/config"
	"github.com/Ehtisam/sotto/internal/discord"
	"github.com/Ehtisam/sotto/internal/soloist"
)

// Discord durumu: Ayarlar'dan açılınca çalan şarkı gönderilir; durunca
// temizlenir. Kimlik verilmemişse sotto'nun kendi uygulaması kullanılır.
func TestDiscordStatus(t *testing.T) {
	acts, restore := discord.FakeForTest()
	defer restore()
	m := selectSetting(settingsModel(t), setDiscord)
	m.cfg.Desktop.Discord, m.cfg.Desktop.DiscordAppID = false, ""
	if id := m.cfg.Desktop.DiscordID(); id != config.DefaultDiscordAppID {
		t.Fatalf("default id = %q", id)
	}
	m.cfg.Desktop.DiscordAppID = " 123 "
	if id := m.cfg.Desktop.DiscordID(); id != "123" {
		t.Fatalf("own id = %q", id)
	}
	m.cfg.Desktop.DiscordAppID = ""
	m.state.Item = nil
	playingState(&m)
	m, _, _ = m.settingsKey("enter")
	if m.rt.discord == nil {
		t.Fatal("presence not started")
	}
	defer m.Shutdown()
	wait := func() map[string]any {
		select {
		case a := <-acts:
			return a
		case <-time.After(3 * time.Second):
			t.Fatal("nothing sent to Discord")
			return nil
		}
	}
	a := wait()
	if a == nil || a["details"] != "Song 01" || a["state"] != "Artist 1" {
		t.Fatalf("activity %v", a)
	}
	if b, _ := a["buttons"].([]any); len(b) != 1 {
		t.Fatalf("no Spotify button: %v", a)
	}
	m.state.Status = "paused"
	m.updateDiscord()
	if a := wait(); a != nil {
		t.Fatalf("pause should clear, got %v", a)
	}
	m, _, _ = m.settingsKey("enter") // kapat
	if m.rt.discord != nil {
		t.Fatal("presence still running")
	}
}

// playingState: "Song 01 — Artist 1" bu bilgisayarda çalıyor.
func playingState(m *Model) {
	it := &soloist.Entity{URI: "spotify:track:t1", EntityType: "track"}
	it.Decorations.Identity.Name = "Song 01"
	it.Decorations.Playback.DurationMS = 200000
	var artist soloist.Entity
	artist.Decorations.Identity.Name = "Artist 1"
	it.Decorations.Creators = append(it.Decorations.Creators, struct {
		Entity soloist.Entity `json:"entity"`
	}{artist})
	m.state.Item = it
	m.state.Status = soloist.StatusPlaying
	m.now = time.Now()
}
