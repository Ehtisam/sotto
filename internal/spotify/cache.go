package spotify

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Ehtisam/sotto/internal/i18n"
)

// Spotify geliştirici uygulamalarına düşük bir istek sınırı koyuyor; aşılınca
// saatlerce (20.11.2026'da 3,5 saat) 429 dönüyor. İki önlem:
//
//  1. Yanıt önbelleği: GET yanıtları türüne göre bir süre diskte tutulur
//     (%LOCALAPPDATA%\sotto\api-cache). Açılışta kitaplık, ana sayfa, profil
//     yeniden istenmez. Yazan bir istekten sonra ilgili kayıtlar silinir.
//     Çalma durumu (me/player) hiç tutulmaz.
//  2. Sınır: Spotify uzun bir bekleme isterse (Retry-After > 10 sn) o saate
//     kadar istek gönderilmez (diske de yazılır, komut satırı ve yeniden açılan
//     sotto da bekler); bu sürede eldeki kayıt (eski de olsa) gösterilir.

// RateLimitError: Spotify sotto'yu bir süreliğine sınırladı.
type RateLimitError struct{ Until time.Time }

func (e *RateLimitError) Error() string {
	return i18n.F("err.ratelimit", humanWait(time.Until(e.Until)))
}

// IsRateLimited, hatanın uzun süreli sınır olup olmadığını söyler.
func IsRateLimited(err error) bool {
	var r *RateLimitError
	return errors.As(err, &r)
}

// humanWait: "3 sa 35 dk", "12 dk", "40 sn".
func humanWait(d time.Duration) string {
	d = d.Round(time.Second)
	h, m, s := int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60
	switch {
	case h > 0:
		return i18n.F("wait.hm", h, m)
	case m > 0:
		return i18n.F("wait.m", max(m, 1))
	}
	return i18n.F("wait.s", max(s, 1))
}

// cacheTTL, bir GET yolunun önbellekte ne kadar geçerli sayılacağı (0: tutulmaz).
func cacheTTL(path string) time.Duration {
	p := strings.SplitN(path, "?", 2)[0]
	switch {
	case p == "me/player/recently-played":
		return 2 * time.Minute
	case strings.HasPrefix(p, "me/player"):
		return 0 // çalma durumu, sıra, cihazlar: canlı
	case p == "me":
		return 24 * time.Hour
	case strings.HasPrefix(p, "me/top/"):
		return 6 * time.Hour
	case strings.HasPrefix(p, "me/following"), strings.HasPrefix(p, "me/albums"), strings.HasPrefix(p, "me/shows"):
		return time.Hour
	case strings.HasPrefix(p, "me/playlists"), strings.HasPrefix(p, "me/tracks"), strings.HasPrefix(p, "playlists/"):
		return 10 * time.Minute
	case p == "search":
		return 10 * time.Minute
	case strings.HasPrefix(p, "tracks/"), strings.HasPrefix(p, "albums/"), strings.HasPrefix(p, "artists/"),
		strings.HasPrefix(p, "shows/"), strings.HasPrefix(p, "episodes/"):
		return 24 * time.Hour
	}
	return 0
}

// livePaths: yazmalardan etkilenmeyen, uzun tutulan kayıtlar (bir yazmadan sonra silinmez).
func keepOnWrite(path string) bool {
	p := strings.SplitN(path, "?", 2)[0]
	return p == "me" || strings.HasPrefix(p, "me/top/") || strings.HasPrefix(p, "tracks/") ||
		strings.HasPrefix(p, "albums/") || strings.HasPrefix(p, "artists/") || strings.HasPrefix(p, "shows/") ||
		strings.HasPrefix(p, "episodes/") || p == "search"
}

type cacheEntry struct {
	Path  string          `json:"path"`
	Saved time.Time       `json:"saved"`
	Data  json.RawMessage `json:"data"`
}

type apiCache struct {
	dir string // "" : yalnız bellek

	mu      sync.Mutex
	mem     map[string]cacheEntry
	blocked map[string]time.Time // grup → bitiş
	loaded  bool
}

func newAPICache(dir string) *apiCache {
	return &apiCache{dir: dir, mem: map[string]cacheEntry{}, blocked: map[string]time.Time{}}
}

func cacheKey(path string) string {
	h := sha1.Sum([]byte(path))
	return hex.EncodeToString(h[:10])
}

// get: kayıt ve geçerli olup olmadığı.
func (c *apiCache) get(path string, now time.Time) (cacheEntry, bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.mem[path]
	if !ok && c.dir != "" {
		if b, err := os.ReadFile(filepath.Join(c.dir, cacheKey(path)+".json")); err == nil && json.Unmarshal(b, &e) == nil && e.Path == path {
			c.mem[path], ok = e, true
		}
	}
	if !ok {
		return e, false, false
	}
	return e, true, now.Sub(e.Saved) < cacheTTL(path)
}

func (c *apiCache) put(path string, data []byte, now time.Time) {
	e := cacheEntry{Path: path, Saved: now, Data: append(json.RawMessage(nil), data...)}
	c.mu.Lock()
	c.mem[path] = e
	c.mu.Unlock()
	if c.dir == "" {
		return
	}
	if b, err := json.Marshal(e); err == nil {
		_ = os.MkdirAll(c.dir, 0o700)
		_ = os.WriteFile(filepath.Join(c.dir, cacheKey(path)+".json"), b, 0o600)
	}
}

// invalidate: bir yazmadan sonra, değişebilecek bütün kayıtları siler.
func (c *apiCache) invalidate() {
	c.mu.Lock()
	for p := range c.mem {
		if !keepOnWrite(p) {
			delete(c.mem, p)
		}
	}
	c.mu.Unlock()
	if c.dir == "" {
		return
	}
	files, _ := filepath.Glob(filepath.Join(c.dir, "*.json"))
	for _, f := range files {
		var e cacheEntry
		if b, err := os.ReadFile(f); err == nil && json.Unmarshal(b, &e) == nil && keepOnWrite(e.Path) {
			continue
		}
		_ = os.Remove(f)
	}
}

// group: sınırın uygulandığı yol grubu. Çalma uçları sınırlanmadan çalışabiliyor.
func group(path string) string {
	if strings.HasPrefix(path, "me/player") && !strings.HasPrefix(path, "me/player/recently-played") {
		return "player"
	}
	return "api"
}

func (c *apiCache) blockFile() string { return filepath.Join(c.dir, "rate-limit.json") }

func (c *apiCache) loadBlocks() {
	if c.loaded || c.dir == "" {
		return
	}
	c.loaded = true
	if b, err := os.ReadFile(c.blockFile()); err == nil {
		_ = json.Unmarshal(b, &c.blocked)
	}
}

// blockedUntil: grup için sınırın bitişi (sıfır: sınır yok).
func (c *apiCache) blockedUntil(path string, now time.Time) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.loadBlocks()
	if t := c.blocked[group(path)]; now.Before(t) {
		return t
	}
	return time.Time{}
}

func (c *apiCache) block(path string, until time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.loadBlocks()
	c.blocked[group(path)] = until
	if c.dir != "" {
		if b, err := json.Marshal(c.blocked); err == nil {
			_ = os.MkdirAll(c.dir, 0o700)
			_ = os.WriteFile(c.blockFile(), b, 0o600)
		}
	}
}

// ---- isteği yapanın seçenekleri ----

type freshKey struct{}

// Fresh, önbelleği atlayıp Spotify'a sorar (R ile yenileme).
func Fresh(ctx context.Context) context.Context { return context.WithValue(ctx, freshKey{}, true) }

func isFresh(ctx context.Context) bool { v, _ := ctx.Value(freshKey{}).(bool); return v }

// RateLimitedUntil, sınır varsa bitişi (durum satırı ve sotto doctor için).
func (c *Client) RateLimitedUntil() time.Time {
	return c.cache.blockedUntil("me", time.Now())
}
