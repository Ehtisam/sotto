package ui

import (
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/config"
	"github.com/Ehtisam/sotto/internal/spotify"
)

// Çevrimdışı sürücü: gerçek Bubble Tea programı, gerçek terminal baytları
// (kaçış dizileri, fare dizileri), ama Spotify yerine sahte bir sunucu ve
// geçici ayar klasörü. Hesaba dokunmaz, istek sınırını harcamaz; her zaman çalışır.

func driveOffline(t *testing.T) (*driver, *fakeWeb) {
	t.Helper()
	f, _ := newFakeWeb(t)
	var items []string
	for i := 0; i < 30; i++ {
		items = append(items, fmt.Sprintf(`{"added_at":"2025-01-%02dT00:00:00Z","item":{"name":"Song %02d","uri":"spotify:track:t%d","duration_ms":%d,"artists":[{"name":"Artist %d"}]}}`, i%28+1, i, i, 100000+i*1000, i%3))
	}
	f.on("GET me/tracks", `{"total":0,"items":[]}`)
	f.on("GET me/playlists", `{"total":2,"items":[{"id":"pl","name":"Road Trip","uri":"spotify:playlist:pl","owner":{"id":"me","display_name":"Me"},"items":{"total":30}},{"id":"zz","name":"Autumn","uri":"spotify:playlist:zz","owner":{"id":"me","display_name":"Me"},"items":{"total":0}}]}`)
	f.on("GET playlists/pl/items", `{"total":30,"items":[`+strings.Join(items, ",")+`]}`)
	f.on("GET me/player/recently-played", `{"items":[]}`)
	f.on("GET me/top/", `{"items":[]}`)
	f.on("GET me/player/devices", `{"devices":[]}`)
	f.on("GET me", `{"id":"me","display_name":"Tester","product":"premium"}`)

	dir := t.TempDir()
	cfg := config.Config{ConfigDir: dir, LocalDir: dir, Spotify: config.Spotify{ClientID: "x"},
		Look: config.Look{Theme: "cardo", Covers: "off", LineNumbers: "absolute", Mouse: true}}
	if err := spotify.WriteTokenForTest(cfg.TokenFile()); err != nil {
		t.Fatal(err)
	}
	r, w := io.Pipe()
	d := &driver{t: t, in: w, done: make(chan struct{}), sync: make(chan int, 16)}
	d.p = tea.NewProgram(New(cfg),
		tea.WithInput(r), tea.WithOutput(io.Discard), tea.WithWindowSize(110, 32),
		tea.WithoutSignalHandler(),
		tea.WithFilter(func(m tea.Model, msg tea.Msg) tea.Msg {
			if mm, ok := m.(Model); ok {
				d.mu.Lock()
				d.last = mm
				d.mu.Unlock()
			}
			if s, ok := msg.(syncMsg); ok {
				d.sync <- s.n
				return nil
			}
			return msg
		}))
	go func() { d.p.Run(); close(d.done) }()
	t.Cleanup(func() {
		d.p.Kill()
		<-d.done
		d.last.Shutdown()
	})
	time.Sleep(700 * time.Millisecond)
	return d, f
}

func (d *driver) raw(b string) {
	_, _ = d.in.Write([]byte(b))
	time.Sleep(80 * time.Millisecond)
}

func TestDriveOfflineLookGroup(t *testing.T) {
	d, f := driveOffline(t)

	// g l: Kitaplık; enter: ilk liste (Road Trip).
	d.do("gl")
	if m := d.model(); m.page != pageLibrary || m.focus != focusContent {
		t.Fatalf("g l → page %v focus %v", m.page, m.focus)
	}
	d.do("j<enter><wait:300>")
	m := d.model()
	if m.lib.open == nil || len(m.lib.open.tracks) != 30 {
		t.Fatalf("list not open: %+v\n%s", m.lib.open, d.screen())
	}

	// 12j: on iki aşağı.
	d.do("12j")
	if m := d.model(); m.lib.open.cursor != 12 {
		t.Fatalf("12j cursor %d\n%s", m.lib.open.cursor, d.screen())
	}

	// / ile süz: yazılanlar kısayol değil.
	d.do("/song 2")
	m = d.model()
	// Önce "song 2" bütünüyle geçenler (Song 20–29), sonra iki sözcüğün de
	// geçtiği öbürleri (Song 02, "Artist 2"…).
	if m.lib.open.find.query != "song 2" || m.lib.open.rows() != 17 || m.lib.open.tracks[m.lib.open.at(0)].Name != "Song 20" {
		t.Fatalf("filter %q rows %d\n%s", m.lib.open.find.query, m.lib.open.rows(), d.screen())
	}
	if !strings.Contains(d.screen(), "/ song 2") {
		t.Fatalf("filter bar missing\n%s", d.screen())
	}
	d.do("<enter>j")
	if m := d.model(); m.lib.open.tracks[m.lib.open.cursor].Name != "Song 21" {
		t.Fatalf("selected %q", m.lib.open.tracks[m.lib.open.cursor].Name)
	}
	d.do("<esc>")
	if m := d.model(); m.lib.open == nil || m.lib.open.find.active() {
		t.Fatal("esc should clear the filter and stay in the list")
	}

	// ctrl+f de başlatır.
	d.raw("\x06")
	d.do("autumn")
	if m := d.model(); m.lib.open.find.query != "autumn" {
		t.Fatalf("ctrl+f query %q", m.lib.open.find.query)
	}
	d.do("<esc>")

	// shift+O: sıralama menüsü; süreye göre, sonra tersi.
	d.do("O")
	if !d.model().sortm.open || !strings.Contains(d.screen(), "Duration") {
		t.Fatalf("sort menu\n%s", d.screen())
	}
	d.do("jjjjjj<enter>") // kendi, ad, sanatçı, albüm, çıkış, eklenme, süre
	m = d.model()
	if m.lib.open.sortBy != sortDuration || m.lib.open.tracks[m.lib.open.at(0)].Name != "Song 00" {
		t.Fatalf("sort %q first %q", m.lib.open.sortBy, m.lib.open.tracks[m.lib.open.at(0)].Name)
	}
	d.do("O<enter>")
	if m := d.model(); !m.lib.open.desc || m.lib.open.tracks[m.lib.open.at(0)].Name != "Song 29" {
		t.Fatal("second pick should flip the order")
	}
	if !strings.Contains(d.screen(), "Duration ↓") {
		t.Fatalf("sort label missing\n%s", d.screen())
	}

	// Fare: listede bir satıra tıkla (seç), yeniden tıkla (çal).
	m = d.model()
	sel := m.selectedContentRow()
	l := m.layout()
	x, y := l.contentX+6, l.bodyTop+sel-2                                       // iki satır yukarı (imleç listenin sonunda)
	clickSeq := fmt.Sprintf("\x1b[<0;%d;%dM\x1b[<0;%d;%dm", x+1, y+1, x+1, y+1) // SGR: 1 tabanlı
	start := m.lib.open.pos()
	t.Logf("sel row %d start pos %d target y %d", sel, start, y)
	d.raw(clickSeq)
	m = d.model()
	if m.lib.open.pos() != start-2 {
		t.Fatalf("click selected pos %d, want %d\n%s", m.lib.open.pos(), start-2, d.screen())
	}
	d.raw(clickSeq)
	time.Sleep(300 * time.Millisecond)
	if len(f.sent("PUT me/player/play")) != 1 {
		t.Fatalf("second click did not play: %v", f.sent("PUT"))
	}
	// Tekerlek listede aşağı.
	d.raw(fmt.Sprintf("\x1b[<65;%d;%dM", x+1, y+1))
	if m := d.model(); m.lib.open.pos() != start-1 {
		t.Fatalf("wheel pos %d", m.lib.open.pos())
	}

	// ? listesi tuş haritasından.
	d.do("?")
	if !strings.Contains(d.screen(), "Moving around") || !strings.Contains(d.screen(), "g l") {
		t.Fatalf("help\n%s", d.screen())
	}
	d.do("<esc>")

	// g o: günlük (çalma isteği ve notlar orada).
	d.do("go")
	if m := d.model(); m.extra.page != extraLog {
		t.Fatalf("g o → %v", m.extra.page)
	}
	d.do("<esc>")
	if d.model().extraOn() {
		t.Fatal("esc should close the log")
	}
}
