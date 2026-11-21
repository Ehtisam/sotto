// Package history, sotto'nun yerel dinleme geçmişi: sotto açıkken çalan
// (en az 30 saniye ya da şarkının yarısı dinlenen) her şarkı
// %LOCALAPPDATA%\sotto\history.jsonl dosyasına bir satır olarak eklenir.
// Spotify'ın "son çalınanlar"ı 50 şarkıyla sınırlı; bu kayıt sınırsız ve
// istatistikler (en çok dinlenenler, saat/gün dağılımı, özetler) buradan çıkar.
package history

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Play, bir dinleme.
type Play struct {
	At        time.Time `json:"at"` // başladığı an
	URI       string    `json:"uri"`
	Title     string    `json:"title"`
	Artist    string    `json:"artist,omitempty"`
	ArtistURI string    `json:"artist_uri,omitempty"`
	Album     string    `json:"album,omitempty"`
	MS        int64     `json:"ms"` // dinlenen süre
	Context   string    `json:"context,omitempty"`
	Device    string    `json:"device,omitempty"`
}

// MinPlay: bundan kısa dinlemeler sayılmaz (Spotify'ın ölçüsü gibi).
const MinPlay = 30 * time.Second

// Counts: şarkının uzunluğu bilinen bir dinleme sayılır mı.
func Counts(played, duration time.Duration) bool {
	if played >= MinPlay {
		return true
	}
	return duration > 0 && played >= duration/2 && played >= 5*time.Second
}

// File, geçmiş dosyası.
func File(localDir string) string { return filepath.Join(localDir, "history.jsonl") }

// Append, bir dinlemeyi dosyanın sonuna ekler.
func Append(localDir string, p Play) error {
	if err := os.MkdirAll(localDir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(File(localDir), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return err
}

// Load, bütün dinlemeler (eskiden yeniye). Bozuk satırlar atlanır.
func Load(localDir string) ([]Play, error) {
	f, err := os.Open(File(localDir))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Play
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		var p Play
		if json.Unmarshal(sc.Bytes(), &p) == nil && p.URI != "" {
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out, sc.Err()
}

// Range, istatistik aralığı.
type Range string

const (
	Week  Range = "week"
	Month Range = "month"
	Year  Range = "year"
	All   Range = "all"
)

// Ranges, sırasıyla dolaşılan aralıklar.
var Ranges = []Range{Week, Month, Year, All}

// Since, aralığın başlangıcı (All için sıfır zaman).
func (r Range) Since(now time.Time) time.Time {
	switch r {
	case Week:
		return now.AddDate(0, 0, -7)
	case Month:
		return now.AddDate(0, -1, 0)
	case Year:
		return now.AddDate(-1, 0, 0)
	}
	return time.Time{}
}

// Count, bir şarkı ya da sanatçı ve dinlenme sayısı.
type Count struct {
	Key, Name, Sub string // Key: URI (sanatçıda URI ya da ad)
	Plays          int
	MS             int64
}

// Stats, bir aralığın özeti.
type Stats struct {
	Plays   int
	MS      int64
	Songs   int // farklı şarkı
	Artists int // farklı sanatçı
	Days    int // dinlenen gün sayısı
	ByHour  [24]int
	ByDay   [7]int // Pazartesi 0 … Pazar 6
	Top     []Count
	TopArt  []Count
	Recent  []Play // en yeni önce
}

// Compute, plays içinden [since, now] aralığının özeti (yerel saatle).
func Compute(plays []Play, r Range, now time.Time, topN int) Stats {
	since := r.Since(now)
	var s Stats
	songs := map[string]*Count{}
	arts := map[string]*Count{}
	days := map[string]bool{}
	for _, p := range plays {
		if p.At.Before(since) || p.At.After(now) {
			continue
		}
		t := p.At.Local()
		s.Plays++
		s.MS += p.MS
		s.ByHour[t.Hour()]++
		s.ByDay[(int(t.Weekday())+6)%7]++
		days[t.Format("2006-01-02")] = true
		c := songs[p.URI]
		if c == nil {
			c = &Count{Key: p.URI, Name: p.Title, Sub: p.Artist}
			songs[p.URI] = c
		}
		c.Plays++
		c.MS += p.MS
		for _, a := range splitArtists(p.Artist) {
			key := strings.ToLower(a)
			ac := arts[key]
			if ac == nil {
				ac = &Count{Key: a, Name: a}
				if p.ArtistURI != "" && strings.HasPrefix(p.Artist, a) {
					ac.Key = p.ArtistURI
				}
				arts[key] = ac
			}
			ac.Plays++
			ac.MS += p.MS
		}
		s.Recent = append(s.Recent, p)
	}
	s.Songs, s.Artists, s.Days = len(songs), len(arts), len(days)
	s.Top = topOf(songs, topN)
	s.TopArt = topOf(arts, topN)
	for i, j := 0, len(s.Recent)-1; i < j; i, j = i+1, j-1 {
		s.Recent[i], s.Recent[j] = s.Recent[j], s.Recent[i]
	}
	return s
}

// splitArtists: "A, B" → [A B] (sotto sanatçıları virgülle yazar).
func splitArtists(s string) []string {
	var out []string
	for _, a := range strings.Split(s, ", ") {
		if a = strings.TrimSpace(a); a != "" {
			out = append(out, a)
		}
	}
	return out
}

func topOf(m map[string]*Count, n int) []Count {
	out := make([]Count, 0, len(m))
	for _, c := range m {
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Plays != out[j].Plays {
			return out[i].Plays > out[j].Plays
		}
		if out[i].MS != out[j].MS {
			return out[i].MS > out[j].MS
		}
		return out[i].Name < out[j].Name
	})
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}
