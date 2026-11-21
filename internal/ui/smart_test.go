package ui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Ehtisam/sotto/internal/soloist"
	"github.com/Ehtisam/sotto/internal/spclient"
)

// fakeInner, iç uçlar (radyo, bağlam, istemci ayarları) için sahte sunucu.
func fakeInner(t *testing.T) *[]string {
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, r.Method+" "+r.URL.Path)
		switch {
		case strings.HasPrefix(r.URL.Path, "/inspiredby-mix/"):
			w.Write([]byte(`{"mediaItems":[{"uri":"spotify:playlist:radio"}]}`))
		case strings.HasPrefix(r.URL.Path, "/context-resolve/"):
			w.Write([]byte(`{"metadata":{},"pages":[{"tracks":[{"uri":"spotify:track:r1"},{"uri":"spotify:track:r2"},{"uri":"spotify:track:r3"}]}]}`))
		case r.URL.Path == "/clientsettings/api/v1/":
			w.WriteHeader(200)
		default:
			http.Error(w, "no", 404)
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(spclient.SetBaseForTest(srv.URL))
	return &hits
}

// s: kapalı → karıştır → akıllı (öneriler gelir) → kapalı. Akıllıyken her üç
// şarkıda bir öneri sıraya eklenir; bekleyen öneri varken yenisi eklenmez.
func TestSmartShuffleCycleAndQueue(t *testing.T) {
	f, m := playingModel(t)
	fakeInner(t)
	m.cfg.Extras.Enabled = true
	m.rt.bridge.SetTokenForTest("tok")
	m.state.Context = &soloist.Entity{URI: "spotify:playlist:pl"}

	m = m.pressKey("s")
	if got := f.sent("PUT me/player/shuffle"); len(got) != 1 || !strings.Contains(got[0], "state=true") || m.rt.smart.on {
		t.Fatalf("first s: %v smart=%v", got, m.rt.smart.on)
	}
	m.state.Options.Shuffle = true
	m = m.pressKey("s")
	if !m.rt.smart.on || len(m.rt.smart.recs) != 3 {
		t.Fatalf("second s: smart=%v recs=%v", m.rt.smart.on, m.rt.smart.recs)
	}
	if !strings.Contains(m.shuffleMark(), "✦") {
		t.Fatal("no smart mark in the strip")
	}
	play := func(uri string) {
		m.state.Item = &soloist.Entity{URI: uri, EntityType: "track"}
		if cmd := m.smartTick(); cmd != nil {
			m = settle(m, cmd)
		}
	}
	play("spotify:track:x1")
	play("spotify:track:x2")
	if len(f.sent("POST me/player/queue")) != 0 {
		t.Fatal("added too early")
	}
	play("spotify:track:x3")
	got := f.sent("POST me/player/queue")
	if len(got) != 1 || !strings.Contains(got[0], "r1") {
		t.Fatalf("after 3 songs: %v", got)
	}
	// Öneri bekliyorken yenisi eklenmez; öneri çalınca sayaç yeniden başlar.
	play("spotify:track:x4")
	play("spotify:track:x5")
	play("spotify:track:x6")
	if len(f.sent("POST me/player/queue")) != 1 {
		t.Fatal("second suggestion while one is still waiting")
	}
	play("spotify:track:r1")
	play("spotify:track:x7")
	play("spotify:track:x8")
	play("spotify:track:x9")
	if got := f.sent("POST me/player/queue"); len(got) != 2 || !strings.Contains(got[1], "r2") {
		t.Fatalf("next suggestion: %v", got)
	}
	// Üçüncü s: kapanır.
	m.state.Options.Shuffle = true
	m = m.pressKey("s")
	if m.rt.smart.on || len(f.sent("PUT me/player/shuffle")) != 2 {
		t.Fatalf("third s: smart=%v %v", m.rt.smart.on, f.sent("PUT me/player/shuffle"))
	}
}

// Ekstralar kapalıyken s yalnız karıştırmayı açıp kapatır.
func TestShuffleWithoutExtras(t *testing.T) {
	f, m := playingModel(t)
	m.cfg.Extras.Enabled = false
	m.state.Options.Shuffle = true
	m = m.pressKey("s")
	if got := f.sent("PUT me/player/shuffle"); len(got) != 1 || !strings.Contains(got[0], "state=false") || m.rt.smart.on {
		t.Fatalf("%v smart=%v", got, m.rt.smart.on)
	}
}

// Özel oturum: Ayarlar'dan açılır, başlıkta görünür, bitiş zamanı saklanır.
func TestPrivateSessionSetting(t *testing.T) {
	m := selectSetting(settingsModel(t), setPrivate)
	hits := fakeInner(t)
	m.cfg.Extras.Enabled = true
	m.rt.bridge.SetTokenForTest("tok")
	m.now = time.Now()
	m, cmd, _ := m.settingsKey("enter")
	if cmd == nil {
		t.Fatal("no request")
	}
	mm, _ := m.Update(cmd())
	m = mm.(Model)
	m.now = time.Now()
	if !m.privateOn() || !strings.Contains(m.settingValue(setPrivate), ":") {
		t.Fatalf("not on: %q", m.settingValue(setPrivate))
	}
	if len(*hits) != 1 || (*hits)[0] != "PUT /clientsettings/api/v1/" {
		t.Fatalf("hits %v", *hits)
	}
	if !strings.Contains(m.header(120), "private") {
		t.Fatal("no private mark in the header")
	}
	if b, _ := os.ReadFile(privateFile(m.cfg.LocalDir)); !strings.Contains(string(b), "until") {
		t.Fatal("not saved")
	}
	m, cmd, _ = m.settingsKey("enter")
	mm, _ = m.Update(cmd())
	m = mm.(Model)
	if m.privateOn() || m.settingValue(setPrivate) != T("set.off") {
		t.Fatal("should be off")
	}
}
