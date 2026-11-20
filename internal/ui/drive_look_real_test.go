package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Ehtisam/sotto/internal/cli"
)

// 2. grup "Görünüm", gerçek hesapla (yalnız okur: hiçbir şey çalmaz, beğenmez,
// sesi değiştirmez; sıralamayı deneyip geri alır).
//
//	SOTTO_DRIVE=1 go test ./internal/ui -run DriveLookReal -v
func TestDriveLookReal(t *testing.T) {
	d := drive(t)

	// Kitaplık: KaBoom.2'yi sayfa içi aramayla bul ve aç.
	d.do("gl<wait:2500>")
	m := d.model()
	if m.page != pageLibrary || len(m.lib.entries) < 2 {
		t.Fatalf("library: page %v entries %d\n%s", m.page, len(m.lib.entries), d.screen())
	}
	d.do("/kaboom<wait:300>")
	vis := d.model().libVisible()
	if len(vis) == 0 || !strings.Contains(strings.ToLower(vis[0].name), "kaboom") {
		t.Fatalf("filter kaboom: %v\n%s", names(vis), d.screen())
	}
	d.do("<enter><enter><wait:2500>")
	m = d.model()
	tl := m.lib.open
	if tl == nil || len(tl.tracks) == 0 {
		t.Fatalf("KaBoom.2 not open\n%s", d.screen())
	}
	t.Logf("opened %q: %d of %d songs", tl.entry.name, len(tl.tracks), tl.total)

	// Listede süz: ilk şarkının adının ilk sözcüğü.
	word := strings.Fields(tl.tracks[0].Name)[0]
	d.do("/" + word + "<wait:1500>")
	m = d.model()
	if m.lib.open.rows() == 0 || !strings.Contains(d.screen(), "/ "+word) {
		t.Fatalf("filter %q: rows %d\n%s", word, m.lib.open.rows(), d.screen())
	}
	if m.lib.open.total > len(m.lib.open.tracks) && len(m.lib.open.tracks) < maxFindLoad {
		t.Errorf("filter should load the whole list: %d of %d", len(m.lib.open.tracks), m.lib.open.total)
	}
	d.do("<esc>")
	if d.model().lib.open == nil || d.model().lib.open.find.active() {
		t.Fatal("esc should only clear the filter")
	}

	// Sıralama: ada göre, sonra kendi sırasına geri (kayıt silinmeli).
	d.do("Oj<enter><wait:1500>")
	m = d.model()
	tl = m.lib.open
	if tl.sortBy != sortName {
		t.Fatalf("sort %q\n%s", tl.sortBy, d.screen())
	}
	for p := 1; p < tl.rows(); p++ {
		a, b := cli.Fold(tl.tracks[tl.at(p-1)].Name), cli.Fold(tl.tracks[tl.at(p)].Name)
		if a > b {
			t.Fatalf("not sorted at %d: %q > %q", p, a, b)
		}
	}
	d.do("Okkkkkk<enter><wait:300>")
	if d.model().lib.open.sortBy != "" {
		t.Fatal("custom order not restored")
	}
	if _, ok := loadSorts(d.model().cfg.LocalDir)[tl.entry.key]; ok {
		t.Fatal("saved sort not removed")
	}

	// Sayı: 3j.
	if len(tl.tracks) > 4 {
		before := d.model().lib.open.pos()
		d.do("3j<wait:200>")
		if got := d.model().lib.open.pos(); got != min(before+3, tl.rows()-1) {
			t.Errorf("3j: %d → %d", before, got)
		}
	}

	// Fare: bir satıra tek tıklama seçer (çalmaz). Ayarlarda fare kapalıysa tıklama yok sayılır.
	m = d.model()
	sel, l := m.selectedContentRow(), m.layout()
	if m.cfg.Look.Mouse && sel > 6 {
		y := l.bodyTop + sel - 2
		d.raw(fmt.Sprintf("\x1b[<0;%d;%dM\x1b[<0;%d;%dm", l.contentX+7, y+1, l.contentX+7, y+1))
		if got := d.model().selectedContentRow(); got != sel-2 {
			t.Errorf("click: selected row %d, want %d", got, sel-2)
		}
	}

	// Ek sayfalar.
	d.do("<esc><esc>gt<wait:3000>")
	m = d.model()
	if m.extra.page != extraTop || len(m.extra.items) == 0 {
		t.Errorf("top page: %+v err %v\n%s", m.extra.page, m.extra.err, d.screen())
	} else {
		t.Logf("top: %d rows", len(m.extra.items))
	}
	d.do("<esc>gr<wait:3000>")
	m = d.model()
	if m.extra.page != extraRecent || len(m.extra.items) == 0 {
		t.Errorf("recent page: err %v\n%s", m.extra.err, d.screen())
	}
	d.do("<esc>go<wait:300>")
	if d.model().extra.page != extraLog {
		t.Error("g o did not open the log")
	}
	d.do("<esc>?<wait:200>")
	if !strings.Contains(d.screen(), "g l") {
		t.Errorf("help\n%s", d.screen())
	}
	d.do("<esc>")

	// c: çalan bir şey varsa listesinde bulur (çalmaz).
	if m := d.model(); m.state.Item != nil {
		d.do("c<wait:3000>")
		m = d.model()
		if tl := m.shownList(); tl != nil && indexOfTrack(tl.tracks, m.state.Item.URI) == tl.cursor {
			t.Logf("c found the playing song in %q", tl.entry.name)
		} else {
			t.Logf("c: page %v flash %q (playing %s, context %s)", m.page, m.flash, m.state.Item.URI, m.state.Context.URIOrEmpty())
		}
	}
	t.Log("\n" + d.screen())
}
