package ui

import (
	"strings"
	"testing"

	"github.com/Ehtisam/sotto/internal/soloist"
)

// Soloist kapalıyken ses ayarı kaydedilir, yeniden başlatma olmaz.
func TestCrossfadeSavedWithoutSoloist(t *testing.T) {
	m := selectSetting(settingsModel(t), setCrossfade)
	m.cfg.Soloist.Crossfade = 0
	m, cmd, _ := m.settingsKey("right")
	m, cmd2, _ := m.settingsKey("right")
	if m.cfg.Soloist.Crossfade != 5 {
		t.Fatalf("crossfade = %d, want 5", m.cfg.Soloist.Crossfade)
	}
	if cmd != nil || cmd2 != nil {
		t.Fatal("no restart expected while Soloist is off")
	}
	if !strings.Contains(savedFile(t, m), "crossfade = 5") {
		t.Fatal("crossfade not written to config.toml")
	}
	if !strings.Contains(m.flash, T("set.applied.next")) {
		t.Fatalf("flash = %q", m.flash)
	}
	m, _, _ = m.settingsKey("left")
	m, _, _ = m.settingsKey("left")
	m, _, _ = m.settingsKey("left") // 5 → 3 → kapalı → 12
	if m.cfg.Soloist.Crossfade != 12 {
		t.Fatalf("left wraps to %d, want 12", m.cfg.Soloist.Crossfade)
	}
	if v := m.settingValue(setCrossfade); v != "12 s" {
		t.Fatalf("value = %q", v)
	}
}

func TestQualityAndNormalizeCycle(t *testing.T) {
	m := selectSetting(settingsModel(t), setQuality)
	m.cfg.Soloist.Quality, m.cfg.Soloist.Normalize = "very_high", true
	m, _, _ = m.settingsKey("right")
	if m.cfg.Soloist.Quality != "lossless" || !strings.Contains(savedFile(t, m), `quality = "lossless"`) {
		t.Fatalf("quality = %q", m.cfg.Soloist.Quality)
	}
	m, _, _ = m.settingsKey("right")
	if m.cfg.Soloist.Quality != "normal" || !strings.Contains(savedFile(t, m), `quality = "normal"`) {
		t.Fatalf("quality = %q", m.cfg.Soloist.Quality)
	}
	m = selectSetting(m, setNormalize)
	m, _, _ = m.settingsKey("enter")
	if m.cfg.Soloist.Normalize || !strings.Contains(savedFile(t, m), "normalize = false") {
		t.Fatal("normalize not turned off")
	}
}

// Soloist sotto'nunken art arda basışlar tek bir yeniden başlatmaya iner;
// müzik Soloist'te çalıyorsa yeniden açılınca çalarak geri alınır.
func TestPlaybackSettingRestartsSoloistOnce(t *testing.T) {
	m := selectSetting(settingsModel(t), setCrossfade)
	m.cfg.Soloist.Crossfade = 0
	m.rt.proc, m.rt.client, m.engine = &soloist.Process{}, &soloist.Client{}, engineReady
	m.auth = soloist.AuthState{LoggedIn: true, IsActive: true}
	m.state.Status = soloist.StatusPlaying
	m, cmd1, _ := m.settingsKey("right")
	m, cmd2, _ := m.settingsKey("right")
	if cmd1 == nil || cmd2 == nil {
		t.Fatal("expected delayed apply commands")
	}
	mm, cmd := m.updateSettingsMsg(applyPrefsMsg{seq: m.set.applySeq - 1})
	if cmd != nil || mm.(Model).engine != engineReady {
		t.Fatal("stale apply must do nothing")
	}
	mm, cmd = m.updateSettingsMsg(applyPrefsMsg{seq: m.set.applySeq})
	got := mm.(Model)
	if cmd == nil || got.engine != engineStarting || got.rt.proc != nil {
		t.Fatalf("restart not started: engine=%v proc=%v", got.engine, got.rt.proc)
	}
	if got.devs.pending != got.cfg.Soloist.DeviceName || got.devs.pendingPaused {
		t.Fatalf("pending = %q paused=%v, want playing transfer back", got.devs.pending, got.devs.pendingPaused)
	}

	// Duraklatılmışken: geri alınır ama çalmaz. (rt paylaşılıyor: Soloist'i yeniden kur.)
	m.rt.proc, m.rt.client = &soloist.Process{}, &soloist.Client{}
	m.state.Status = soloist.StatusPaused
	mm, _ = m.updateSettingsMsg(applyPrefsMsg{seq: m.set.applySeq})
	if !mm.(Model).devs.pendingPaused {
		t.Fatal("paused music should come back paused")
	}

	// Müzik başka cihazdayken: yeniden açılır ama müzik alınmaz.
	m.rt.proc, m.rt.client = &soloist.Process{}, &soloist.Client{}
	m.auth.IsActive = false
	mm, _ = m.updateSettingsMsg(applyPrefsMsg{seq: m.set.applySeq})
	if mm.(Model).devs.pending != "" {
		t.Fatal("music elsewhere must not be moved")
	}
}

// Başka bir sotto'nun açtığı Soloist yeniden başlatılmaz.
func TestPlaybackSettingAttachedSoloist(t *testing.T) {
	m := selectSetting(settingsModel(t), setNormalize)
	m.rt.proc, m.rt.client, m.engine = &soloist.Process{Attached: true}, &soloist.Client{}, engineReady
	m, cmd, _ := m.settingsKey("enter")
	if cmd != nil || !strings.Contains(m.flash, T("set.applied.next")) {
		t.Fatalf("attached: cmd=%v flash=%q", cmd != nil, m.flash)
	}
}

func TestOnStartCycleAndOff(t *testing.T) {
	m := selectSetting(settingsModel(t), setResume)
	m.cfg.Soloist.OnStart = "paused"
	m, _, _ = m.settingsKey("right")
	if m.cfg.Soloist.OnStart != "play" || !strings.Contains(savedFile(t, m), `on_start = "play"`) {
		t.Fatalf("on_start = %q", m.cfg.Soloist.OnStart)
	}
	m, _, _ = m.settingsKey("right")
	if m.cfg.Soloist.OnStart != "off" {
		t.Fatalf("on_start = %q", m.cfg.Soloist.OnStart)
	}
	m.spState = spReady
	if m.maybeResume() != nil {
		t.Fatal("off must not resume")
	}
}
