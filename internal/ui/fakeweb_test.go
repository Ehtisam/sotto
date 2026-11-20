package ui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/config"
	"github.com/Ehtisam/sotto/internal/spotify"
)

// fakeWeb: istekleri kaydeden sahte Web API. Yanıtlar yol önekine göre
// (en uzun eşleşen) verilir; yoksa 204.
type fakeWeb struct {
	mu   sync.Mutex
	reqs []string // "PUT me/player/volume?volume_percent=0"
	body map[string]string
	resp map[string]string
}

func newFakeWeb(t *testing.T) (*fakeWeb, Model) {
	t.Helper()
	f := &fakeWeb{resp: map[string]string{}, body: map[string]string{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if r.URL.RawQuery != "" {
			p += "?" + r.URL.RawQuery
		}
		b, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		key := r.Method + " " + p
		f.reqs = append(f.reqs, key)
		f.body[key] = string(b)
		best := ""
		for k := range f.resp {
			if strings.HasPrefix(key, k) && len(k) > len(best) {
				best = k
			}
		}
		out := f.resp[best]
		f.mu.Unlock()
		if out == "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, out)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(spotify.SetAPIBaseForTest(srv.URL + "/"))

	dir := t.TempDir()
	cfg := config.Config{ConfigDir: dir, LocalDir: dir, Spotify: config.Spotify{ClientID: "x"}}
	if err := spotify.WriteTokenForTest(cfg.TokenFile()); err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m.wiz.on = false
	m.spState = spReady
	m.lib.loading, m.home.loading = false, false
	m.width, m.height = 120, 40
	m.me.ID = "me"
	return f, m
}

func (f *fakeWeb) on(prefix, json string) { f.mu.Lock(); f.resp[prefix] = json; f.mu.Unlock() }

// sent, öneki tutan istekler.
func (f *fakeWeb) sent(prefix string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, r := range f.reqs {
		if strings.HasPrefix(r, prefix) {
			out = append(out, r)
		}
	}
	return out
}

func (f *fakeWeb) bodyOf(req string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.body[req]
}

// settle, komutları çalıştırıp çıkan mesajları modele geri verir (birkaç tur).
// Zamanlayıcılar ve dinleyiciler runAll'da atlanır.
func settle(m Model, cmd tea.Cmd) Model {
	for round := 0; round < 8 && cmd != nil; round++ {
		var next []tea.Cmd
		for _, msg := range runDeep(cmd) {
			if msg == nil {
				continue
			}
			if _, ok := msg.(tickMsg); ok {
				continue
			}
			out, c := m.Update(msg)
			m = out.(Model)
			next = append(next, c)
		}
		cmd = tea.Batch(next...)
	}
	return m
}

// runDeep, runAll gibi, ama tea.Sequence'in sıralı komutlarını da açar.
func runDeep(cmd tea.Cmd) []tea.Msg {
	var out []tea.Msg
	for _, msg := range runAll(cmd) {
		v := reflect.ValueOf(msg)
		if v.IsValid() && v.Kind() == reflect.Slice && v.Type().Elem() == reflect.TypeOf(tea.Cmd(nil)) {
			for i := 0; i < v.Len(); i++ {
				c, _ := v.Index(i).Interface().(tea.Cmd)
				out = append(out, runDeep(c)...)
			}
			continue
		}
		out = append(out, msg)
	}
	return out
}

// key, bir tuşa basar ve sonuçları yerleştirir.
func (m Model) pressKey(k string) Model {
	out, cmd := m.Update(keyPress(k))
	return settle(out.(Model), cmd)
}

func teaPaste(s string) tea.PasteMsg { return tea.PasteMsg{Content: s} }
