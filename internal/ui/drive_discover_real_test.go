package ui

import (
	"strings"
	"testing"
)

// 3. grup "Keşif", gerçek hesapla. Yalnız okur: hiçbir şey çalmaz, takip
// etmez, beğenmez (h, enter ile çalma yok).
//
//	SOTTO_DRIVE=1 go test ./internal/ui -run DriveDiscoverReal -v
func TestDriveDiscoverReal(t *testing.T) {
	d := drive(t)

	// Ara → en iyi sonuç → sanatçı sayfası.
	d.do("/athena<wait:2500><enter>")
	m := d.model()
	if len(m.search.items) < 2 || m.search.items[0].section != "search.top" {
		t.Fatalf("no top result\n%s", d.screen())
	}
	t.Logf("top result: %s (%s)", m.search.items[1].name, m.search.items[1].kind)
	if m.search.items[1].kind != "artist" {
		t.Fatalf("top result should be the artist\n%s", d.screen())
	}
	d.do("<enter><wait:4000>")
	m = d.model()
	a := m.extra.artist
	if m.extra.page != extraArtist || a == nil || a.view == nil {
		t.Fatalf("artist page: %+v\n%s", a, d.screen())
	}
	t.Logf("artist %q verified=%v listeners=%q popular=%d releases=%d featuring=%d related=%d appears=%d bio=%d chars",
		a.name, a.view.Verified, a.view.Listeners, len(a.view.Popular), len(a.view.Releases), len(a.view.Featuring),
		len(a.view.Related), len(a.view.AppearsOn), len(a.bio))
	if len(a.view.Popular) == 0 || len(a.view.Related) == 0 {
		t.Errorf("popular or related empty\n%s", d.screen())
	}
	t.Log("\n" + d.screen())

	// Künye: ilk popüler şarkı.
	d.do("ji<wait:3000>")
	m = d.model()
	in := m.extra.info
	if m.extra.page != extraInfo || in == nil || in.loading {
		t.Fatalf("info\n%s", d.screen())
	}
	if in.err != nil {
		t.Errorf("info error: %v", in.err)
	}
	t.Logf("info: album=%v credits=%v", in.album != nil, in.credits != nil)
	if in.album != nil {
		t.Logf("label %q date %q copyrights %d", in.album.Label, in.album.ReleaseDate, len(in.album.Copyrights))
	}
	if in.credits == nil || len(in.credits.Roles) == 0 {
		t.Errorf("no credits\n%s", d.screen())
	}
	t.Log("\n" + d.screen())
	d.do("<esc>")

	// Diskografi (resmî API): bütün çıkışlar.
	d.do("<tab><wait:4000>")
	a = d.model().extra.artist
	groups := map[string]int{}
	for _, al := range a.albums {
		groups[al.Group(a.id)]++
	}
	t.Logf("discography %d %v err=%v", len(a.albums), groups, a.albumsErr)
	if len(a.albums) == 0 {
		t.Errorf("empty discography\n%s", d.screen())
	}
	d.do("<tab><tab><wait:300>")
	t.Log("\n" + d.screen())
	d.do("<esc>")

	// Kitaplık: albüm, sanatçı, podcast sayıları.
	d.do("gl<wait:6000>")
	m = d.model()
	t.Logf("library saved: loaded=%v albums=%d artists=%d podcasts=%d err=%v", m.lib.savedLoaded,
		len(m.savedOf(kindAlbum)), len(m.savedOf(kindArtist)), len(m.savedOf(kindShow)), m.lib.savedErr)
	if !m.lib.savedLoaded {
		t.Errorf("saved items not loaded\n%s", d.screen())
	}
	t.Log("\n" + d.screen())

	// Yenilikler.
	d.do("gw<wait:15000>")
	n := d.model().extra.news
	if n == nil || n.loading {
		t.Fatalf("news still loading\n%s", d.screen())
	}
	t.Logf("news: artists=%d shows=%d releases=%d episodes=%d err=%v", n.artists, n.shows, len(n.releases), len(n.episodes), n.err)
	t.Log("\n" + d.screen())
	d.do("<esc>")

	// Kişiler: takipçiler, takip edilenler, arkadaşlar; bir takipçinin profili.
	d.do("gf<wait:3000>")
	ps := d.model().extra.people
	if ps == nil {
		t.Fatalf("people\n%s", d.screen())
	}
	t.Logf("followers %d err=%v", len(ps.followers), ps.err[pplFollowers])
	d.do("<tab><wait:3000>")
	t.Logf("following %d err=%v", len(d.model().extra.people.following), d.model().extra.people.err[pplFollowing])
	d.do("<tab><wait:3000>")
	t.Logf("friends %d err=%v", len(d.model().extra.people.friends), d.model().extra.people.err[pplFriends])
	t.Log("\n" + d.screen())
	d.do("<tab><wait:300>")
	if len(d.model().extra.people.followers) > 0 {
		d.do("<enter><wait:3000>")
		u := d.model().extra.user
		if u == nil || u.view == nil {
			t.Fatalf("user page\n%s", d.screen())
		}
		t.Logf("user %q image=%v followers=%d following=%d lists=%d", u.name, u.view.Image != "", u.view.Followers, u.view.FollowingN, len(u.view.Playlists))
		if u.name == "" || strings.HasPrefix(u.name, "spotify:") {
			t.Errorf("user name missing")
		}
		t.Log("\n" + d.screen())
	}
}
