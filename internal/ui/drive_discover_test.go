package ui

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/config"
	"github.com/Ehtisam/sotto/internal/spclient"
	"github.com/Ehtisam/sotto/internal/spotify"
)

// 3. grup "Keşif": gerçek Bubble Tea programı, sahte Web API ve sahte iç uçlar
// (spclient) ile. Ekstralar açık; bridge jetonu sabit.

type fakeSP struct {
	mu   sync.Mutex
	resp map[string]string // yol öneki → JSON
	reqs []string
}

func (f *fakeSP) on(prefix, json string) { f.mu.Lock(); f.resp[prefix] = json; f.mu.Unlock() }

func (f *fakeSP) sent(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.reqs {
		if strings.HasPrefix(r, prefix) {
			n++
		}
	}
	return n
}

const artistViewJSON = `{"header":{"text":{"title":"Athena","accessory":"1M monthly listeners"},"custom":{"verified":true}},
"body":[
 {"id":"artist-entity-view-top-tracks-combined","text":{"title":"Popular"}},
 {"id":"artist-entity-view-top-tracks-combined_row0","text":{"title":"Dam","subtitle":"33,324,983"},"metadata":{"sectionId":"artist-entity-view-top-tracks-combined","uri":"spotify:track:p1","album_uri":"spotify:album:al1"}},
 {"id":"artist-entity-view-top-tracks-combined_row1","text":{"title":"Yalan","subtitle":"54,052,565"},"metadata":{"sectionId":"artist-entity-view-top-tracks-combined","uri":"spotify:track:p2","album_uri":"spotify:album:al2"}},
 {"id":"artist-entity-view-releases","text":{"title":"Popular releases"}},
 {"id":"artist-entity-view-releases_row0","text":{"title":"Altüst","subtitle":"2014"},"metadata":{"uri":"spotify:album:al1","sectionId":"artist-entity-view-releases"}},
 {"id":"artist-entity-view-releases_link","text":{"title":"See discography"},"metadata":{"uri":"spotify:artist:a1:releases","sectionId":"artist-entity-view-releases"}},
 {"id":"artist-entity-view-you-might-also-like","text":{"title":"Featuring Athena"},"children":[
  {"id":"artist-entity-view-you-might-also-like_card0","text":{"title":"This Is Athena","description":"The essentials"},"metadata":{"uri":"spotify:playlist:37i9this","sectionId":"artist-entity-view-you-might-also-like"}}]},
 {"id":"artist-entity-view-related","text":{"title":"Fans also like"},"children":[
  {"id":"artist-entity-view-related_card0","text":{"title":"Kargo"},"metadata":{"uri":"spotify:artist:a2","sectionId":"artist-entity-view-related"}}]},
 {"id":"artist-entity-view-appears-on-section","text":{"title":"Appears on"},"children":[
  {"id":"artist-entity-view-appears-on-section_card0","text":{"title":"Comp","subtitle":"2012"},"metadata":{"uri":"spotify:album:al9","sectionId":"artist-entity-view-appears-on-section"}}]}
]}`

func driveDiscover(t *testing.T) (*driver, *fakeWeb, *fakeSP) {
	t.Helper()
	f, _ := newFakeWeb(t)
	sp := &fakeSP{resp: map[string]string{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		sp.mu.Lock()
		sp.reqs = append(sp.reqs, p)
		best := ""
		for k := range sp.resp {
			if strings.HasPrefix(p, k) && len(k) > len(best) {
				best = k
			}
		}
		out := sp.resp[best]
		sp.mu.Unlock()
		if out == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		io.WriteString(w, out)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(spclient.SetBaseForTest(srv.URL))

	day := func(d int) string { return time.Now().AddDate(0, 0, -d).Format("2006-01-02") }
	f.on("GET me/tracks", `{"total":0,"items":[]}`)
	f.on("GET me/playlists", `{"total":1,"items":[{"id":"pl","name":"Road Trip","uri":"spotify:playlist:pl","owner":{"id":"me","display_name":"Me"},"items":{"total":0}}]}`)
	f.on("GET me/player/recently-played", `{"items":[]}`)
	f.on("GET me/top/", `{"items":[]}`)
	f.on("GET me/player/devices", `{"devices":[]}`)
	f.on("GET me/library/contains", `[false]`)
	f.on("GET me", `{"id":"me","display_name":"Tester","product":"premium"}`)
	f.on("GET me/albums", `{"total":1,"items":[{"added_at":"2026-01-01T00:00:00Z","album":{"id":"al5","name":"Saved Album","uri":"spotify:album:al5","release_date":"2019-05-01","total_tracks":9,"album_type":"album","artists":[{"name":"Duman","uri":"spotify:artist:d1"}]}}]}`)
	f.on("GET me/following", `{"artists":{"items":[{"id":"a1","name":"Athena","uri":"spotify:artist:a1"}],"cursors":{"after":""}}}`)
	f.on("GET me/shows", `{"total":1,"items":[{"show":{"id":"sh1","name":"Talk Show","uri":"spotify:show:sh1","total_episodes":40}}]}`)
	f.on("GET shows/sh1/episodes", `{"total":2,"items":[{"id":"e1","name":"Fresh Episode","uri":"spotify:episode:e1","duration_ms":1800000,"release_date":"`+day(2)+`"},{"id":"e0","name":"Old Episode","uri":"spotify:episode:e0","release_date":"`+day(90)+`"}]}`)
	f.on("GET search", `{"artists":{"total":1,"items":[{"id":"a1","name":"Athena","uri":"spotify:artist:a1"}]},"tracks":{"total":1,"items":[{"id":"t9","name":"Athena Song","uri":"spotify:track:t9","artists":[{"name":"X"}],"album":{"uri":"spotify:album:x"}}]}}`)
	f.on("GET artists/a1/albums?include_groups=album,single,compilation", `{"total":2,"items":[
		{"id":"al1","name":"Altüst","uri":"spotify:album:al1","release_date":"2014-03-01","total_tracks":12,"album_type":"album","album_group":"album","artists":[{"name":"Athena","uri":"spotify:artist:a1"}]},
		{"id":"al3","name":"Tek","uri":"spotify:album:al3","release_date":"2020-01-01","total_tracks":1,"album_type":"single","album_group":"single","artists":[{"name":"Athena","uri":"spotify:artist:a1"}]}]}`)
	f.on("GET artists/a1/albums?include_groups=album,single&", `{"total":2,"items":[
		{"id":"new1","name":"Brand New","uri":"spotify:album:new1","release_date":"`+day(3)+`","total_tracks":1,"album_type":"single","album_group":"single","artists":[{"name":"Athena","uri":"spotify:artist:a1"}]},
		{"id":"old1","name":"Old One","uri":"spotify:album:old1","release_date":"2014-03-01","total_tracks":12,"album_type":"album","album_group":"album","artists":[{"name":"Athena","uri":"spotify:artist:a1"}]}]}`)
	f.on("GET albums/al1/tracks", `{"total":2,"items":[{"id":"p1","name":"Dam","uri":"spotify:track:p1","duration_ms":200000},{"id":"x2","name":"Other","uri":"spotify:track:x2","duration_ms":100000}]}`)
	f.on("GET albums/al1", `{"id":"al1","name":"Altüst","uri":"spotify:album:al1","release_date":"2014-03-01","total_tracks":12,"album_type":"album","label":"PASAJ","copyrights":[{"text":"2014 PASAJ","type":"P"}],"artists":[{"name":"Athena","uri":"spotify:artist:a1"}],"tracks":{"total":12,"items":[]}}`)
	f.on("GET artists/w1", `{"id":"w1","name":"Gökhan Özoğuz","uri":"spotify:artist:w1"}`)

	sp.on("/artistview/v1/artist/a1", artistViewJSON)
	sp.on("/artistview/v1/artist/a2", `{"header":{"text":{"title":"Kargo","accessory":"500K monthly listeners"}},"body":[]}`)
	sp.on("/metadata/4/artist/", `{"biography":[{"text":"Athena is a <a href=\"spotify:artist:x\">band</a> from Istanbul.\n\nThey play ska."}]}`)
	sp.on("/track-credits-view/", `{"roleCredits":[{"roleTitle":"Writers","artists":[{"uri":"spotify:artist:w1","name":"Gökhan Özoğuz","subroles":["composer","lyricist"]},{"name":"Someone Else","subroles":["lyricist"]}]}],"sourceNames":["PASAJ"]}`)
	sp.on("/user-profile-view/v3/profile/me/followers", `{"profiles":[{"uri":"spotify:user:u1","name":"irem","followers_count":26,"is_followed":true,"is_following":true}]}`)
	sp.on("/user-profile-view/v3/profile/me/following", `{"profiles":[{"uri":"spotify:artist:a1","name":"Athena","followers_count":900000,"is_following":true}]}`)
	sp.on("/user-profile-view/v3/profile/u1", `{"name":"irem","followers_count":26,"following_count":30,"public_playlists":[{"uri":"spotify:playlist:up1","name":"irem's mix","owner_name":"irem"}]}`)
	sp.on("/user-profile-view/v3/profile/me", `{"followers_count":6,"following_count":12,"public_playlists":[]}`)
	sp.on("/presence-view/v1/buddylist", `{"friends":[{"timestamp":`+fmt.Sprint(time.Now().Add(-time.Minute).UnixMilli())+`,"user":{"uri":"spotify:user:u1","name":"irem"},"track":{"uri":"spotify:track:f1","name":"Friend Song","artist":{"uri":"spotify:artist:a2","name":"Kargo"},"album":{"uri":"spotify:album:fa","name":"FA"},"context":{"uri":"spotify:album:fa","name":"FA"}}}]}`)

	dir := t.TempDir()
	cfg := config.Config{ConfigDir: dir, LocalDir: dir, Spotify: config.Spotify{ClientID: "x"},
		Look:   config.Look{Theme: "cardo", Covers: "off", LineNumbers: "absolute"},
		Extras: config.Extras{Enabled: true}}
	if err := spotify.WriteTokenForTest(cfg.TokenFile()); err != nil {
		t.Fatal(err)
	}
	model := New(cfg)
	model.rt.bridge.SetTokenForTest("tok")
	r, w := io.Pipe()
	d := &driver{t: t, in: w, done: make(chan struct{}), sync: make(chan int, 16)}
	d.p = tea.NewProgram(model,
		tea.WithInput(r), tea.WithOutput(io.Discard), tea.WithWindowSize(110, 34),
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
	return d, f, sp
}

func TestDriveDiscoverArtist(t *testing.T) {
	d, f, sp := driveDiscover(t)

	// Ara: "athena" → en iyi sonuç sanatçı; enter sanatçı sayfasını açar.
	d.do("/athena<wait:900><enter>")
	m := d.model()
	if len(m.search.items) < 2 || m.search.items[0].section != "search.top" || m.search.items[1].kind != "artist" {
		t.Fatalf("top result missing\n%s", d.screen())
	}
	d.do("<enter><wait:400>")
	m = d.model()
	if m.extra.page != extraArtist || m.extra.artist == nil || m.extra.artist.view == nil {
		t.Fatalf("artist page not open\n%s", d.screen())
	}
	s := d.screen()
	for _, want := range []string{"Athena ✓", "1M monthly listeners", "Popular", "Discography", "Fans also like", "Dam", "33,324,983", "Popular releases", "This Is Athena"} {
		if !strings.Contains(s, want) {
			t.Fatalf("artist page lacks %q\n%s", want, s)
		}
	}

	// Popüler şarkı: popüler şarkılar sırayla, seçilenden başlar.
	d.do("jj<enter><wait:300>")
	plays := f.sent("PUT me/player/play")
	if len(plays) != 1 || !strings.Contains(f.bodyOf(plays[0]), `"spotify:track:p1","spotify:track:p2"`) || !strings.Contains(f.bodyOf(plays[0]), `"offset":{"uri":"spotify:track:p2"}`) {
		t.Fatalf("play popular: %v %s", plays, f.bodyOf(plays[0]))
	}

	// i: şarkının ayrıntıları ve künyesi; yazara enter sanatçı sayfası.
	d.do("ki<wait:400>")
	m = d.model()
	if m.extra.page != extraInfo || m.extra.prev == nil {
		t.Fatalf("info not open over the artist page\n%s", d.screen())
	}
	s = d.screen()
	for _, want := range []string{"Details", "Credits", "Writers", "Gökhan Özoğuz", "composer, lyricist", "Someone Else", "PASAJ", "℗ 2014 PASAJ"} {
		if !strings.Contains(s, want) {
			t.Fatalf("info lacks %q\n%s", want, s)
		}
	}
	// Künyede sayfası olan kişi seçilebilir; olmayan atlanır.
	for i := 0; i < 8 && !strings.Contains(d.model().extra.items[d.model().extra.cursor].name, "Gökhan"); i++ {
		d.do("j")
	}
	d.do("<enter><wait:400>")
	m = d.model()
	if m.extra.page != extraArtist || m.extra.artist.id != "w1" || m.extra.artist.name != "Gökhan Özoğuz" {
		t.Fatalf("credit artist page: %+v\n%s", m.extra.artist, d.screen())
	}
	d.do("<esc><esc>")
	if m := d.model(); m.extra.page != extraArtist || m.extra.artist.id != "a1" {
		t.Fatalf("esc esc should come back to Athena: %v", m.extra.page)
	}

	// tab: Diskografi (resmî API), gruplar; albüm açılır ve şarkıları gelir.
	d.do("<tab><wait:400>")
	s = d.screen()
	for _, want := range []string{"Albums", "Altüst", "2014 · 12 songs", "Singles and EPs", "Tek"} {
		if !strings.Contains(s, want) {
			t.Fatalf("discography lacks %q\n%s", want, s)
		}
	}
	d.do("<enter><wait:400>")
	if m := d.model(); m.extra.open == nil || len(m.extra.open.tracks) != 2 {
		t.Fatalf("album from the artist page did not load\n%s", d.screen())
	}
	d.do("<esc>")
	if m := d.model(); m.extra.open != nil || m.extra.artist.section != artDisco {
		t.Fatal("esc should close the album and stay on the discography")
	}

	// Benzer sanatçılar: başka sanatçıya geç, esc geri getirir.
	d.do("<tab><wait:200><enter><wait:400>")
	m = d.model()
	if m.extra.artist == nil || m.extra.artist.name != "Kargo" || m.extra.prev == nil {
		t.Fatalf("related artist\n%s", d.screen())
	}
	d.do("<esc>")
	if m := d.model(); m.extra.artist.name != "Athena" || m.extra.artist.section != artRelated {
		t.Fatalf("back to Athena: %q %v", m.extra.artist.name, m.extra.artist.section)
	}

	// Hakkında: biyografi düz yazı; j/k kaydırır.
	d.do("<tab><wait:200>")
	s = d.screen()
	if !strings.Contains(s, "Athena is a band from Istanbul.") || !strings.Contains(s, "They play ska.") || strings.Contains(s, "<a") {
		t.Fatalf("about\n%s", s)
	}

	// h, "çal" satırında sanatçıyı takip eder.
	d.do("<tab>h<wait:300>")
	if got := f.sent("PUT me/library?uris=spotify%3Aartist%3Aa1"); len(got) != 1 {
		t.Fatalf("follow artist: %v", f.sent("PUT"))
	}
	d.do("<esc>")
	if d.model().extraOn() {
		t.Fatal("esc should close the artist page")
	}
	if sp.sent("/metadata/4/artist/") == 0 {
		t.Fatal("biography was never asked")
	}
}

func TestDriveDiscoverLibraryNewsPeople(t *testing.T) {
	d, f, _ := driveDiscover(t)

	// Kitaplık: albümler, sanatçılar, podcast'ler süzgeçleri.
	d.do("gl<wait:600>")
	s := d.screen()
	if !strings.Contains(s, "Albums") || !strings.Contains(s, "Podcasts") || strings.Contains(s, "Podcas…") {
		t.Fatalf("library chips\n%s", s)
	}
	for i := 0; i < 10 && d.model().lib.filter != libAlbums; i++ {
		d.do("<tab>")
	}
	if s := d.screen(); !strings.Contains(s, "Saved Album") || !strings.Contains(s, "Duman · 2019") || strings.Contains(s, "Road Trip") {
		t.Fatalf("albums filter\n%s", s)
	}
	d.do("<tab>")
	if s := d.screen(); !strings.Contains(s, "Athena") {
		t.Fatalf("artists filter\n%s", s)
	}
	d.do("<enter><wait:400>")
	if m := d.model(); m.extra.page != extraArtist || m.extra.artist.id != "a1" {
		t.Fatalf("artist from library\n%s", d.screen())
	}
	d.do("<esc><tab>")
	if s := d.screen(); !strings.Contains(s, "Talk Show") {
		t.Fatalf("podcasts filter\n%s", s)
	}
	// Hepsi: listelerden sonra kayıtlılar.
	d.do("<tab>")
	m := d.model()
	if m.lib.filter != libAll {
		t.Fatalf("tab should wrap to All, at %v", m.lib.filter)
	}
	if vis := m.libVisible(); len(vis) < 5 {
		t.Fatalf("All should hold lists and saved items: %d", len(vis))
	}

	// g w: Yenilikler; eski çıkış yok, yeni bölüm ikinci bölümde.
	d.do("gw<wait:800>")
	s = d.screen()
	if !strings.Contains(s, "What's new") || !strings.Contains(s, "Brand New") || strings.Contains(s, "Old One") || !strings.Contains(s, "This week") {
		t.Fatalf("news\n%s", s)
	}
	d.do("<tab>")
	if s := d.screen(); !strings.Contains(s, "Fresh Episode") || strings.Contains(s, "Old Episode") {
		t.Fatalf("news episodes\n%s", s)
	}
	d.do("<enter><wait:300>")
	if len(f.sent("PUT me/player/play")) != 1 {
		t.Fatal("enter on a new episode should play it")
	}
	d.do("<esc>")

	// g f: kişiler; takipçiye enter profilini açar, h takip eder.
	d.do("gf<wait:500>")
	s = d.screen()
	if !strings.Contains(s, "Followers 1") || !strings.Contains(s, "irem") || !strings.Contains(s, "you follow") {
		t.Fatalf("people\n%s", s)
	}
	d.do("<enter><wait:400>")
	m = d.model()
	if m.extra.page != extraUser || m.extra.user.view == nil {
		t.Fatalf("user page\n%s", d.screen())
	}
	if s := d.screen(); !strings.Contains(s, "irem's mix") || !strings.Contains(s, "26 followers") {
		t.Fatalf("user page content\n%s", s)
	}
	d.do("h<wait:300>")
	if got := f.sent("PUT me/library?uris=spotify%3Auser%3Au1"); len(got) != 1 {
		t.Fatalf("follow user: %v", f.sent("PUT"))
	}
	d.do("<esc><tab><wait:400>")
	if s := d.screen(); !strings.Contains(s, "Athena") || !strings.Contains(s, "Artists") && !strings.Contains(s, "artist") {
		t.Fatalf("following\n%s", s)
	}
	d.do("<tab><wait:400>")
	s = d.screen()
	if !strings.Contains(s, "Friend activity") || !strings.Contains(s, "Friend Song") || !strings.Contains(s, "Kargo · FA") {
		t.Fatalf("friends\n%s", s)
	}
	d.do("<esc>")
	if d.model().extraOn() {
		t.Fatal("esc should close people")
	}
}
