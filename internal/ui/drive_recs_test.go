package ui

import (
	"strings"
	"testing"

	"github.com/Ehtisam/sotto/internal/config"
)

// Önerilen şarkılar gerçek programda: kendi listenin sonuna inince öneriler
// istenir ve altta görünür; son şarkıda ↓ önerilere iner, a listeye ekler
// (satır listeye geçer), enter çalar, ↑ listeye döner.
func TestDriveRecommendations(t *testing.T) {
	driveCfg = func(c *config.Config) { c.Extras.Enabled = true }
	defer func() { driveCfg = nil }()
	fakeInner(t)
	d, f := driveOffline(t)
	d.model().rt.bridge.SetTokenForTest("tok")
	for _, id := range []string{"r1", "r2", "r3"} {
		f.on("GET tracks/"+id, `{"name":"Rec `+id+`","uri":"spotify:track:`+id+`","duration_ms":180000,"artists":[{"name":"Rec Artist"}]}`)
	}
	f.on("POST playlists/pl/items", `{"snapshot_id":"x"}`)
	f.on("PUT me/player/play", `{}`)

	d.do("gl<wait:300>j<enter><wait:600>")
	if tl := d.model().lib.open; tl == nil || len(tl.tracks) != 30 {
		t.Fatal("Road Trip not open")
	}
	if got := f.sent("GET tracks/"); len(got) != 0 {
		t.Fatalf("recommendations asked before the end of the list: %v", got)
	}
	d.do("G<wait:800>")
	s := d.screen()
	for _, want := range []string{"Recommended", "Rec r1", "Rec r3"} {
		if !strings.Contains(s, want) {
			t.Fatalf("no %q at the end of the list:\n%s", want, s)
		}
	}
	d.do("j<wait:100>")
	if tl := d.model().lib.open; !tl.recs.in || tl.recs.cursor != 0 {
		t.Fatalf("down from the last song should enter the recommendations: %+v", tl.recs)
	}
	if !strings.Contains(d.screen(), "a adds") {
		t.Fatal("no key hint in the recommendations")
	}
	d.do("j<wait:80>a<wait:500>")
	if got := f.sent("POST playlists/pl/items"); len(got) != 1 || !strings.Contains(f.bodyOf(got[0]), "spotify:track:r2") {
		t.Fatalf("add requests = %v", got)
	}
	tl := d.model().lib.open
	if len(tl.tracks) != 31 || tl.tracks[30].URI != "spotify:track:r2" || len(tl.recs.tracks) != 2 {
		t.Fatalf("after add: %d tracks, recs %d", len(tl.tracks), len(tl.recs.tracks))
	}
	d.do("<enter><wait:400>")
	if got := f.sent("PUT me/player/play"); len(got) == 0 {
		t.Fatal("enter on a recommendation did not play")
	}
	d.do("k<wait:80>k<wait:80>")
	if tl := d.model().lib.open; tl.recs.in || tl.pos() != tl.rows()-1 {
		t.Fatalf("up from the first recommendation should go back to the list: in=%v pos=%d", tl.recs.in, tl.pos())
	}
}
