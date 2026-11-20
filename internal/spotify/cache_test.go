package spotify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeAPI: istekleri sayan sahte Web API. limited: bu yollar 429 + uzun Retry-After döner.
type fakeAPI struct {
	mu      sync.Mutex
	hits    map[string]int
	limited map[string]bool
}

func newFake(t *testing.T) (*fakeAPI, *Client) {
	t.Helper()
	f := &fakeAPI{hits: map[string]int{}, limited: map[string]bool{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		f.mu.Lock()
		f.hits[r.Method+" "+p]++
		lim := f.limited[p]
		f.mu.Unlock()
		if lim {
			w.Header().Set("Retry-After", "3600")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"u1","display_name":"Ada","items":[],"total":7}`))
	}))
	t.Cleanup(srv.Close)
	old := apiBase
	apiBase = srv.URL + "/"
	t.Cleanup(func() { apiBase = old })

	dir := t.TempDir()
	tokPath := filepath.Join(dir, "spotify-token.json")
	if err := saveToken(tokPath, Token{AccessToken: "x", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	return f, NewClient("cid", tokPath)
}

func (f *fakeAPI) count(k string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits[k]
}

func TestCacheServesRepeatsAndRefreshBypasses(t *testing.T) {
	f, c := newFake(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := c.Me(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.count("GET me"); n != 1 {
		t.Fatalf("me fetched %d times", n)
	}
	if _, err := c.Me(Fresh(ctx)); err != nil || f.count("GET me") != 2 {
		t.Fatalf("fresh did not refetch: %v", err)
	}
	// Çalma durumu hiç önbelleğe alınmaz.
	c.Player(ctx)
	c.Player(ctx)
	if n := f.count("GET me/player"); n != 2 {
		t.Fatalf("player fetched %d times", n)
	}
	// Başka bir istemci (yeniden açılan sotto) diskteki kaydı kullanır.
	c2 := NewClient("cid", c.tokenPath)
	if u, err := c2.Me(ctx); err != nil || u.ID != "u1" || f.count("GET me") != 2 {
		t.Fatalf("disk cache: %+v %v hits %d", u, err, f.count("GET me"))
	}
}

func TestWriteInvalidatesLists(t *testing.T) {
	f, c := newFake(t)
	ctx := context.Background()
	c.MyPlaylists(ctx, 0, 50)
	c.Me(ctx)
	if err := c.Save(ctx, []string{"spotify:track:x"}); err != nil {
		t.Fatal(err)
	}
	c.MyPlaylists(ctx, 0, 50)
	c.Me(ctx)
	if n := f.count("GET me/playlists"); n != 2 {
		t.Fatalf("playlists fetched %d times after a write", n)
	}
	if n := f.count("GET me"); n != 1 {
		t.Fatalf("me refetched after a write: %d", n)
	}
}

func TestLongRateLimitFailsFastAndServesStale(t *testing.T) {
	f, c := newFake(t)
	ctx := context.Background()
	c.MyPlaylists(ctx, 0, 50) // eldeki kayıt
	f.mu.Lock()
	f.limited["me/top/tracks"] = true
	f.mu.Unlock()

	start := time.Now()
	_, err := c.TopTracks(ctx, ShortTerm, 10)
	if !IsRateLimited(err) || time.Since(start) > 2*time.Second {
		t.Fatalf("err %v after %v", err, time.Since(start))
	}
	if !strings.Contains(err.Error(), "59 min") && !strings.Contains(err.Error(), "1 h") {
		t.Errorf("message %q", err.Error())
	}
	// Sınır sürerken istek gönderilmez.
	before := f.count("GET me/top/artists")
	if _, err := c.TopArtists(ctx, ShortTerm, 10); !IsRateLimited(err) || f.count("GET me/top/artists") != before {
		t.Fatalf("request sent while limited: %v", err)
	}
	// Eldeki kayıt, süresi geçse de verilir.
	c.cache.mu.Lock()
	for p, e := range c.cache.mem {
		e.Saved = e.Saved.Add(-time.Hour)
		c.cache.mem[p] = e
	}
	c.cache.mu.Unlock()
	if _, err := c.MyPlaylists(ctx, 0, 50); err != nil {
		t.Fatalf("stale not served: %v", err)
	}
	// Çalma uçları sınıra takılmaz.
	if _, err := c.Player(ctx); err != nil {
		t.Fatalf("player blocked: %v", err)
	}
	// Sınır diske yazıldı: yeni istemci de bekler.
	c2 := NewClient("cid", c.tokenPath)
	if _, err := c2.RecentlyPlayed(ctx, 10); !IsRateLimited(err) {
		t.Fatalf("new client not limited: %v", err)
	}
	if c2.RateLimitedUntil().IsZero() {
		t.Fatal("RateLimitedUntil zero")
	}
}

func TestHumanWait(t *testing.T) {
	for d, want := range map[time.Duration]string{
		3*time.Hour + 35*time.Minute: "3 h 35 min",
		12 * time.Minute:             "12 min",
		40 * time.Second:             "40 s",
	} {
		if got := humanWait(d); got != want {
			t.Errorf("%v: %q", d, got)
		}
	}
}
