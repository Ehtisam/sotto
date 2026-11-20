package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
)

// Sessize alma: ses 0'a iner, önceki ses bir dosyada tutulur; açan sotto da
// komut satırı da aynı dosyayı kullanır, böylece biri kapatıp öbürü açabilir.

// StateDir, sotto'nun kendi klasörü (%LOCALAPPDATA%\sotto); Main doldurur.
var StateDir string

func muteFile(dir string) string { return filepath.Join(dir, "muted.json") }

// MutedVolume, sessize almadan önceki ses (0: kayıt yok).
func MutedVolume(dir string) int {
	var v struct {
		Volume int `json:"volume"`
	}
	if dir == "" {
		return 0
	}
	b, err := os.ReadFile(muteFile(dir))
	if err != nil || json.Unmarshal(b, &v) != nil {
		return 0
	}
	return v.Volume
}

// SaveMuted, sessize almadan önceki sesi yazar; 0 kaydı siler.
func SaveMuted(dir string, vol int) {
	if dir == "" {
		return
	}
	if vol <= 0 {
		_ = os.Remove(muteFile(dir))
		return
	}
	_ = os.MkdirAll(dir, 0o700)
	b, _ := json.Marshal(map[string]int{"volume": vol})
	_ = os.WriteFile(muteFile(dir), b, 0o600)
}

// MuteTarget, sessize alma tuşunun yeni sesi: ses açıksa 0 (ve eski ses
// saklanır), kapalıysa saklanan ses (yoksa 50). want: "" değiştir, "on", "off".
func MuteTarget(current, saved int, want string) (vol, save int, muted bool) {
	mute := current > 0
	switch want {
	case "on":
		mute = true
	case "off":
		mute = false
	}
	if mute {
		if current == 0 {
			return 0, saved, true // zaten sessiz
		}
		return 0, current, true
	}
	if current > 0 {
		return current, 0, false // zaten açık
	}
	if saved <= 0 {
		saved = 50
	}
	return saved, 0, false
}

// MuteText, sessize alma sonucunun metni.
func MuteText(muted bool, vol int) string {
	if muted {
		return "Muted."
	}
	return "Unmuted (volume " + strconv.Itoa(vol) + ")."
}
