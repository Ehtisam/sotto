package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/Ehtisam/sotto/internal/spclient"
)

// Ekstralar kapalıyken sanatçı sayfası resmî bilgiyle açılır: yalnızca
// Diskografi ve Hakkında; iç yola hiç gidilmez.
func TestArtistPageWithoutExtras(t *testing.T) {
	f, m := newFakeWeb(t)
	f.on("GET artists/a1/albums", `{"total":1,"items":[{"id":"al1","name":"Altüst","uri":"spotify:album:al1","release_date":"2014-03-01","total_tracks":12,"album_group":"album"}]}`)
	f.on("GET artists/a1", `{"id":"a1","name":"Athena","uri":"spotify:artist:a1","genres":["ska"],"followers":{"total":1234567}}`)
	m.cfg.Extras.Enabled = false
	m, cmd := m.openArtist("spotify:artist:a1", "Athena")
	m = settle(m, cmd)
	a := m.extra.artist
	if a == nil || a.info == nil || a.view != nil {
		t.Fatalf("artist state %+v", a)
	}
	if secs := a.sections(); len(secs) != 2 || secs[0] != artDisco || a.section != artDisco {
		t.Fatalf("sections %v at %v", secs, a.section)
	}
	view := ansi.Strip(strings.Join(m.extraView(80), "\n"))
	if !strings.Contains(view, "Altüst") || !strings.Contains(view, "1,234,567 followers") || !strings.Contains(view, "more with the extras") {
		t.Fatalf("view\n%s", view)
	}
	m, _ = m.switchArtistSection(1)
	view = ansi.Strip(strings.Join(m.extraView(80), "\n"))
	if !strings.Contains(view, "Genres") || !strings.Contains(view, "ska") || !strings.Contains(view, "Turn them on in Settings") {
		t.Fatalf("about\n%s", view)
	}
	if len(f.sent("GET artists/a1/albums")) != 1 {
		t.Fatal("discography asked more than once")
	}
	// g f ve künye ekstralar olmadan söyler.
	mm, _ := m.openPeople("", "", pplFollowers)
	if !strings.Contains(mm.(Model).flash, "extras") {
		t.Fatalf("people without extras: %q", mm.(Model).flash)
	}
}

// Fotoğraf: Hakkında'da sayılar fotoğrafın yanına geçer, liste altta kısalır.
func TestArtistAboutPhoto(t *testing.T) {
	_, m := newFakeWeb(t)
	m.cfg.Look.Covers = "pixel"
	m.cfg.Extras.Enabled = true
	m.page, m.focus = pageHome, focusContent
	a := &artistState{uri: "spotify:artist:a1", id: "a1", name: "Athena", section: artAbout,
		view: &spclient.ArtistView{Name: "Athena", Image: "https://i/x", Listeners: "1M monthly listeners", Verified: true},
		bio:  "Line one.\n\nLine two."}
	m.extra = extraState{page: extraArtist, host: pageHome, artist: a}
	m.rebuildExtra()
	url, cols, rows := m.extraPhoto()
	if url != "https://i/x" || rows < 4 || cols != rows*2 {
		t.Fatalf("photo %q %dx%d", url, cols, rows)
	}
	if m.extraHeight() != m.listHeight()-rows-1 {
		t.Fatalf("list height %d", m.extraHeight())
	}
	for _, it := range m.extra.items {
		if it.name == "1M monthly listeners" {
			t.Fatal("facts should sit next to the photo, not in the list")
		}
	}
	view := ansi.Strip(strings.Join(m.extraView(90), "\n"))
	if !strings.Contains(view, "1M monthly listeners") || !strings.Contains(view, "✓ Verified artist") || !strings.Contains(view, "Line two.") {
		t.Fatalf("view\n%s", view)
	}
	// Pencere alçaksa fotoğraf yok, sayılar listede.
	m.height = 20
	if u, _, _ := m.extraPhoto(); u != "" {
		t.Fatal("no photo in a short window")
	}
	m.rebuildExtra()
	if m.extra.items[0].name != "1M monthly listeners" {
		t.Fatalf("facts back in the list: %+v", m.extra.items[0])
	}
}

func TestTopResult(t *testing.T) {
	items := []searchItem{
		{kind: "header", section: "search.songs"},
		{kind: "track", name: "Athena Song", uri: "t1"},
		{kind: "header", section: "search.artists"},
		{kind: "artist", name: "Athena", uri: "a1"},
		{kind: "header", section: "search.albums"},
		{kind: "album", name: "Athena", uri: "al1"},
	}
	out := withTopResult(items, "athena")
	if out[0].section != "search.top" || out[1].uri != "a1" || out[1].sub != "artist" {
		t.Fatalf("exact artist should win over an exact album: %+v", out[1])
	}
	out = withTopResult(items, "athena so")
	if out[1].uri != "t1" || !strings.HasPrefix(out[1].sub, "song") {
		t.Fatalf("prefix match on the song: %+v", out[1])
	}
	out = withTopResult(items, "zzz")
	if out[1].uri != "t1" {
		t.Fatalf("no match falls back to the first song: %+v", out[1])
	}
	if out := withTopResult(items[2:4], "x"); out[0].kind != "header" || out[0].section != "search.artists" {
		t.Fatal("without songs and without a match nothing is added")
	}
}

func TestNewsGroupsAndDates(t *testing.T) {
	m := Model{now: time.Date(2026, 11, 21, 12, 0, 0, 0, time.Local)}
	cases := map[string]string{"2026-11-20": "This week", "2026-11-12": "Last week", "2026-10-30": "3 weeks ago"}
	for d, want := range cases {
		if got := m.newsGroup(d); got != want {
			t.Errorf("%s → %q, want %q", d, got, want)
		}
	}
	if got := shortDate("2026-11-14"); got != "Nov 14" {
		t.Errorf("shortDate %q", got)
	}
	if got := niceDate("2006-01-02"); got != "January 2, 2006" {
		t.Errorf("niceDate %q", got)
	}
	if got := niceDate("1998"); got != "1998" {
		t.Errorf("niceDate year %q", got)
	}
}

func TestChipLineFits(t *testing.T) {
	names := []string{"All", "Mine", "Public", "On profile", "Following", "Albums", "Artists", "Podcasts"}
	counts := []string{"", "12", "3", "4", "100", "30", "12", "4"}
	for _, w := range []int{120, 60, 30, 12} {
		s := chipLine(names, counts, 6, w)
		if textWidth(s) > w {
			t.Errorf("w=%d: %d wide: %s", w, textWidth(s), ansi.Strip(s))
		}
		if !strings.Contains(ansi.Strip(s), "Artists") {
			t.Errorf("w=%d: selected chip hidden: %s", w, ansi.Strip(s))
		}
	}
	if s := ansi.Strip(chipLine(names, counts, 0, 200)); !strings.Contains(s, "Following 100") {
		t.Errorf("wide line keeps counts: %s", s)
	}
}

// i: bölümün ve podcast'in ayrıntıları; listenin bilgisi istek atmadan.
func TestInfoEpisodeShowPlaylist(t *testing.T) {
	f, m := newFakeWeb(t)
	f.on("GET episodes/e1", `{"id":"e1","name":"Ep One","uri":"spotify:episode:e1","description":"About <b>things</b>.","duration_ms":1800000,"release_date":"2026-11-02","languages":["tr"],"resume_point":{"fully_played":false,"resume_position_ms":754000},"show":{"id":"sh1","name":"Talk Show","uri":"spotify:show:sh1","total_episodes":40}}`)
	f.on("GET shows/sh1", `{"id":"sh1","name":"Talk Show","uri":"spotify:show:sh1","description":"A show.","total_episodes":40,"languages":["tr","en"],"explicit":true}`)
	m.page, m.focus = pageHome, focusContent

	ep := searchItem{kind: "episode", name: "Ep One", uri: "spotify:episode:e1"}
	m, cmd := m.openInfoFor(ep, nil)
	m = settle(m, cmd)
	view := ansi.Strip(strings.Join(m.extraView(90), "\n"))
	for _, want := range []string{"Ep One", "Talk Show", "November 2, 2026", "30:00", "You stopped at", "12:34", "About things."} {
		if !strings.Contains(view, want) {
			t.Fatalf("episode info lacks %q\n%s", want, view)
		}
	}
	// Podcast satırına enter bölümleri açar.
	if it := m.extra.items[m.extra.cursor]; it.kind != "show" || it.entry.key != "sh1" {
		t.Fatalf("first row %+v", it)
	}

	sh := searchItem{kind: "show", name: "Talk Show", uri: "spotify:show:sh1"}
	m, cmd = m.openInfoFor(sh, nil)
	m = settle(m, cmd)
	view = ansi.Strip(strings.Join(m.extraView(90), "\n"))
	for _, want := range []string{"Episodes", "40 episodes", "tr, en", "A show."} {
		if !strings.Contains(view, want) {
			t.Fatalf("show info lacks %q\n%s", want, view)
		}
	}
	if m.extra.prev == nil || m.extra.prev.info == nil {
		t.Fatal("show info should stack over the episode info")
	}

	pl := entryItem(libEntry{kind: kindPlaylist, key: "pl", uri: "spotify:playlist:pl", name: "Road Trip", owner: "Me", ownerID: "me", total: 30, desc: "for the road"})
	before := len(f.reqs)
	m, cmd = m.openInfoFor(pl, nil)
	if cmd != nil || len(f.reqs) != before {
		t.Fatal("playlist info needs no request")
	}
	view = ansi.Strip(strings.Join(m.extraView(90), "\n"))
	for _, want := range []string{"Open the list", "30 songs", "Made by", "only you", "for the road"} {
		if !strings.Contains(view, want) {
			t.Fatalf("playlist info lacks %q\n%s", want, view)
		}
	}
}
