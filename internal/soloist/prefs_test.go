package soloist

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWritePrefs(t *testing.T) {
	data := t.TempDir()
	user := filepath.Join(data, "settings", "Users", "abc-user")
	if err := os.MkdirAll(user, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(user, "prefs")
	old := "audio.crossfade_migrated_to_ps=true\r\naudio.play_bitrate_non_metered_migrated=true\r\naudio.gapless_migrated_to_ps=true\r\n"
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := WritePrefs(data, Prefs{CrossfadeS: 5, Quality: "high", Normalize: false}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	got := string(b)
	for _, want := range []string{
		"audio.gapless_migrated_to_ps=true\n", // başka satırlar kalır
		"audio.crossfade_migrated_to_ps=false\n",
		"audio.crossfade_v2=true\n",
		"audio.crossfade.time_v2=5000\n",
		"audio.play_bitrate_enumeration=3\n",
		"audio.play_bitrate_non_metered_enumeration=3\n",
		"audio.normalize_v2=false\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "crossfade_migrated_to_ps=true") || strings.Contains(got, "\r") || strings.Count(got, "audio.crossfade_migrated") != 1 {
		t.Errorf("old managed lines not replaced:\n%s", got)
	}

	// Kayıpsız: 5.
	if err := WritePrefs(data, Prefs{Quality: "lossless"}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); !strings.Contains(string(b), "audio.play_bitrate_enumeration=5\n") ||
		!strings.Contains(string(b), "audio.play_bitrate_non_metered_enumeration=5\n") ||
		strings.Count(string(b), "audio.play_bitrate_non_metered_migrated=true") != 1 {
		t.Errorf("lossless write wrong:\n%s", b)
	}

	// İkinci yazış: kapalı geçiş, çok yüksek kalite; satırlar çoğalmaz.
	if err := WritePrefs(data, Prefs{CrossfadeS: 0, Quality: "very_high", Normalize: true}); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(path)
	got = string(b)
	if strings.Count(got, "audio.crossfade_v2=") != 1 || !strings.Contains(got, "audio.crossfade_v2=false") ||
		!strings.Contains(got, "audio.play_bitrate_enumeration=4") || !strings.Contains(got, "audio.normalize_v2=true") {
		t.Errorf("second write wrong:\n%s", got)
	}
}

func TestWritePrefsNoAccount(t *testing.T) {
	data := t.TempDir()
	if err := WritePrefs(data, Prefs{CrossfadeS: 3}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(data, "settings")); !os.IsNotExist(err) {
		t.Error("should not create folders before Soloist has an account")
	}
}
