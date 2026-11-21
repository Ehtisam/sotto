package soloist

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Prefs, Soloist'in kendi ayar dosyasından okuduğu ses ayarları. Soloist'te
// bunlar için komut satırı seçeneği yok; Spotify masaüstü uygulamasının
// kullanıcı başına "prefs" dosyasını okuyor (trill'in bulduğu yol). Soloist
// dosyayı yalnızca açılırken okur: değişiklik yeniden başlatınca geçerli olur.
type Prefs struct {
	CrossfadeS int    // 0: kapalı; 1–12 sn
	Quality    string // "normal", "high" ya da "very_high"
	Normalize  bool   // ses düzeyini eşitle
}

// CrossfadeSteps, Ayarlar'da sırayla dolaşılan geçiş süreleri (sn).
var CrossfadeSteps = []int{0, 3, 5, 8, 12}

// Qualities, Ayarlar'da sırayla dolaşılan ses kaliteleri.
var Qualities = []string{"normal", "high", "very_high"}

// qualityEnum: Spotify'ın kalite numaraları (4 = çok yüksek, 320 kbps; trill doğruladı).
func qualityEnum(q string) int {
	switch q {
	case "normal":
		return 2
	case "high":
		return 3
	}
	return 4
}

// managed: sotto'nun yazdığı anahtarlar; dosyadaki eski değerleri silinir.
var managed = []string{"audio.crossfade", "audio.play_bitrate", "audio.normalize"}

// WritePrefs, veri klasöründeki her hesabın prefs dosyasına ayarları yazar.
// Dosyadaki başka satırlara dokunmaz. Hiç hesap klasörü yoksa (Soloist'e
// henüz giriş yapılmamış) bir şey yazmaz; ayarlar bir sonraki açılışta yazılır.
func WritePrefs(dataDir string, p Prefs) error {
	dirs, _ := filepath.Glob(filepath.Join(dataDir, "settings", "Users", "*-user"))
	for _, dir := range dirs {
		if err := writePrefsFile(filepath.Join(dir, "prefs"), p); err != nil {
			return err
		}
	}
	return nil
}

func writePrefsFile(path string, p Prefs) error {
	old, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	var lines []string
	for _, l := range strings.Split(strings.ReplaceAll(string(old), "\r\n", "\n"), "\n") {
		if l == "" || hasManagedPrefix(l) {
			continue
		}
		lines = append(lines, l)
	}
	cf := max(0, min(12, p.CrossfadeS))
	q := qualityEnum(p.Quality)
	// Soloist bu değerleri yalnız "migrated" işaretleri false iken okur.
	lines = append(lines,
		"audio.crossfade_migrated_to_ps=false",
		fmt.Sprintf("audio.crossfade_v2=%t", cf > 0),
		fmt.Sprintf("audio.crossfade.time_v2=%d", max(1, cf)*1000),
		"audio.play_bitrate_non_metered_migrated=false",
		fmt.Sprintf("audio.play_bitrate_non_metered_enumeration=%d", q),
		fmt.Sprintf("audio.play_bitrate_enumeration=%d", q),
		fmt.Sprintf("audio.normalize_v2=%t", p.Normalize),
	)
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
}

func hasManagedPrefix(l string) bool {
	for _, m := range managed {
		if strings.HasPrefix(l, m) {
			return true
		}
	}
	return false
}
