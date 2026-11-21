package ui

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Ehtisam/sotto/internal/config"
	"github.com/Ehtisam/sotto/internal/spclient"
)

// Şarkı gizleme gerçek programda: açık listede a → h gizler (collection v2
// "ban" yazılır, satır ⊘ ile soluk), yine a → menüde "Unhide song", h geri alır.
// Spotify reddederse satır eski hâline döner.
func TestDriveHideSong(t *testing.T) {
	var mu sync.Mutex
	var writes []string
	fail := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path != "/collection/v2/write" || !bytes.Contains(b, []byte("ban")) {
			http.Error(w, "no", 404)
			return
		}
		if fail {
			http.Error(w, "nope", 500)
			return
		}
		removed := bytes.HasSuffix(b, []byte{0x18, 0x01}) // öğenin 3. alanı: is_removed = 1
		writes = append(writes, map[bool]string{true: "unhide", false: "hide"}[removed]+" "+string(b))
	}))
	defer srv.Close()
	defer spclient.SetBaseForTest(srv.URL)()
	driveCfg = func(c *config.Config) { c.Extras.Enabled = true }
	defer func() { driveCfg = nil }()

	d, _ := driveOffline(t)
	d.model().rt.bridge.SetTokenForTest("tok")
	d.do("gl<wait:300>j<enter><wait:600>jj<wait:100>")
	tl := d.model().lib.open
	if tl == nil || tl.tracks[tl.cursor].Name != "Song 02" {
		t.Fatal("not on Song 02")
	}
	d.do("a<wait:200>")
	if s := d.screen(); !strings.Contains(s, "Hide song") {
		t.Fatalf("no hide item in the menu:\n%s", s)
	}
	d.do("h<wait:500>")
	mu.Lock()
	n := len(writes)
	ok := n == 1 && strings.HasPrefix(writes[0], "hide") && strings.Contains(writes[0], "spotify:track:t2")
	mu.Unlock()
	if !ok {
		t.Fatalf("writes = %q", writes)
	}
	if m := d.model(); !m.isHidden("spotify:track:t2") || !strings.Contains(m.flash, "Hid Song 02") {
		t.Fatalf("hidden=%v flash=%q", m.isHidden("spotify:track:t2"), m.flash)
	}
	if s := d.screen(); !strings.Contains(s, "⊘ Song 02") {
		t.Fatalf("row not marked:\n%s", s)
	}
	d.do("a<wait:200>")
	if s := d.screen(); !strings.Contains(s, "Unhide song") {
		t.Fatalf("no unhide item:\n%s", s)
	}
	d.do("h<wait:500>")
	mu.Lock()
	ok = len(writes) == 2 && strings.HasPrefix(writes[1], "unhide")
	mu.Unlock()
	if !ok || d.model().isHidden("spotify:track:t2") {
		t.Fatalf("unhide: writes %q", writes)
	}
	// Spotify reddederse geri alınır.
	mu.Lock()
	fail = true
	mu.Unlock()
	d.do("a<wait:200>h<wait:600>")
	if m := d.model(); m.isHidden("spotify:track:t2") || !strings.Contains(m.flash, "500") {
		t.Fatalf("after a failed write: hidden=%v flash=%q", m.isHidden("spotify:track:t2"), m.flash)
	}
}
