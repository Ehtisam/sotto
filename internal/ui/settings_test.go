package ui

import (
	"os"
	"strings"
	"testing"

	"github.com/Ehtisam/sotto/internal/i18n"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

func settingsModel(t *testing.T) Model {
	t.Helper()
	m := profileModel(t)
	m.cfg.Look.Theme, m.cfg.Look.Covers = "cardo", "pixel"
	m.cfg.Soloist.InitialVolume, m.cfg.Soloist.Autostart = 40, true
	m.page, m.focus = pageSettings, focusContent
	t.Cleanup(func() { theme.Apply("cardo", ""); i18n.Set("en") })
	return m
}

// selectSetting, imleci verilen ayara getirir.
func selectSetting(m Model, id settingID) Model {
	for i, s := range settingIDs() {
		if s == id {
			m.set.cursor = i
		}
	}
	return m
}

func savedFile(t *testing.T, m Model) string {
	t.Helper()
	b, err := os.ReadFile(m.cfg.ConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSettingsThemeAppliesAndSaves(t *testing.T) {
	m := selectSetting(settingsModel(t), setTheme)
	m, _, _ = m.settingsKey("right")
	if m.cfg.Look.Theme != "night" {
		t.Fatalf("theme = %q", m.cfg.Look.Theme)
	}
	if r, g, b, _ := theme.Ground.RGBA(); r>>8 != 0x0b || g>>8 != 0x0e || b>>8 != 0x14 { // zemin değişti mi
		t.Fatalf("ground not applied: %v", theme.Ground)
	}
	if !strings.Contains(savedFile(t, m), `theme = "night"`) {
		t.Fatal("theme not written to config.toml")
	}
	m, _, _ = m.settingsKey("left")
	m, _, _ = m.settingsKey("left") // cardo'dan geriye: son tema
	if m.cfg.Look.Theme != "paper" {
		t.Fatalf("left wraps to %q, want paper", m.cfg.Look.Theme)
	}
}

func TestSettingsAccentCyclesThroughThemeOwn(t *testing.T) {
	m := selectSetting(settingsModel(t), setAccent)
	m, _, _ = m.settingsKey("right")
	if m.cfg.Look.Accent != "purple" {
		t.Fatalf("accent = %q", m.cfg.Look.Accent)
	}
	m, _, _ = m.settingsKey("left")
	if m.cfg.Look.Accent != "" || m.settingValue(setAccent) != "theme's own" {
		t.Fatalf("accent = %q (%s)", m.cfg.Look.Accent, m.settingValue(setAccent))
	}
	// Açık temada türetilen seçili zemin açık kalmalı.
	p := theme.Resolve("paper", "blue")
	if p.Accent != "#5b8def" || p.Selected[1] < 'c' { // açık zemin: #c…–#f…
		t.Fatalf("paper+blue = %+v", p)
	}
}

func TestSettingsLanguageCoversVolume(t *testing.T) {
	m := selectSetting(settingsModel(t), setLanguage)
	m, _, _ = m.settingsKey("enter")
	if i18n.Lang() != "tr" || m.cfg.Language != "tr" || m.lib.entries[0].name != "Beğenilenler" {
		t.Fatalf("language = %s / %s, liked = %q", i18n.Lang(), m.cfg.Language, m.lib.entries[0].name)
	}
	m = selectSetting(m, setCovers)
	m, _, _ = m.settingsKey("enter")
	if m.cfg.Look.CoversOn() || m.openListCoverURL() != "" {
		t.Fatal("covers should be off")
	}
	m = selectSetting(m, setVolume)
	m, _, _ = m.settingsKey("right")
	if m.cfg.Soloist.InitialVolume != 45 {
		t.Fatalf("volume = %d", m.cfg.Soloist.InitialVolume)
	}
	if !strings.Contains(savedFile(t, m), "initial_volume = 45") {
		t.Fatal("volume not saved")
	}
}

func TestSettingsExtrasOffHidesProfileView(t *testing.T) {
	m := selectSetting(settingsModel(t), setExtras)
	m.cfg.Extras.Enabled = true
	m, _, _ = m.settingsKey("enter")
	if m.cfg.Extras.Enabled {
		t.Fatal("extras should be off")
	}
	for _, f := range m.libFilters() {
		if f == libOnProfile {
			t.Fatal("On profile filter should disappear with extras off")
		}
	}
}
