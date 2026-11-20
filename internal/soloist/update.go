package soloist

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Güncelleme: yeni sürüm çalışan Soloist'in yanına (soloist.exe.new) indirilir,
// sürümü daha yeniyse yer değiştirilir. Windows çalışan bir programın
// silinmesine ya da üstüne yazılmasına izin vermez ama adının değişmesine
// izin verir: eski dosya soloist.exe.old olur, çalışan Soloist müziği kesmeden
// devam eder, bir sonraki başlatışta yeni sürüm açılır.

// StagedPath, indirilen yeni sürümün geçici yeri.
func StagedPath(exe string) string { return exe + ".new" }

func oldPath(exe string) string { return exe + ".old" }

// Swap, indirilen sürümü yerine koyar.
func Swap(exe string) error {
	staged, old := StagedPath(exe), oldPath(exe)
	_ = os.Remove(old) // önceki takastan kalan (artık çalışmıyorsa)
	if _, err := os.Stat(exe); err == nil {
		if err := os.Rename(exe, old); err != nil {
			return err
		}
	}
	if err := os.Rename(staged, exe); err != nil {
		_ = os.Rename(old, exe) // geri al
		return err
	}
	_ = os.Remove(old) // çalışıyorsa silinemez; CleanOld sonra siler
	return nil
}

// CleanOld, önceki takastan kalan eski sürümü ve yarım indirmeleri siler.
func CleanOld(exe string) {
	_ = os.Remove(oldPath(exe))
	_ = os.Remove(StagedPath(exe))
}

func checkFile(exe string) string { return filepath.Join(filepath.Dir(exe), "update-check") }

// CheckedSince, en son ne zaman yeni sürüm arandığı (hiç aranmadıysa sıfır).
func CheckedSince(exe string) time.Time {
	b, err := os.ReadFile(checkFile(exe))
	if err != nil {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339, strings.TrimSpace(string(b)))
	return t
}

// MarkChecked, yeni sürüm arandığını kaydeder (günde bir kez aranır).
func MarkChecked(exe string, at time.Time) {
	_ = os.WriteFile(checkFile(exe), []byte(at.Format(time.RFC3339)), 0o600)
}
