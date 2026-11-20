package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Ehtisam/sotto/internal/spotify"
)

func TestFuzzyMatching(t *testing.T) {
	if _, ok := fuzzyScore("dpnk", "daft punk"); !ok {
		t.Fatal("dpnk should match daft punk")
	}
	if _, ok := fuzzyScore("knp", "daft punk"); ok {
		t.Fatal("out-of-order letters matched")
	}
	a, _ := fuzzyScore("dp", "daft punk") // sözcük başları
	b, _ := fuzzyScore("dp", "deep")      // p sözcük başında değil
	if a <= b {
		t.Fatalf("word starts should score higher: %d <= %d", a, b)
	}
	names := []string{"Seksendört", "Daft Punk - One More Time", "Dua Lipa", "Pinkpantheress"}
	got := matchOrder(nil, len(names), func(i int) string { return names[i] }, "pnk")
	// "pnk" hiçbirinde bitişik geçmiyor; bulanık eşleşenler puan sırasıyla.
	if len(got) != 2 {
		t.Fatalf("pnk matched %v", got)
	}
	got = matchOrder(nil, len(names), func(i int) string { return names[i] }, "seksendort")
	if len(got) != 1 || got[0] != 0 {
		t.Fatalf("accents not folded: %v", got)
	}
	// İçinde geçenler bulanıklardan önce, kendi sıralarıyla.
	got = matchOrder(nil, len(names), func(i int) string { return names[i] }, "pi")
	if len(got) != 2 || got[0] != 3 || got[1] != 1 { // Pinkpantheress içinde geçiyor; "Daft Punk … Time" bulanık
		t.Fatalf("substring order: %v", got)
	}
}

func findList(t *testing.T) (*fakeWeb, Model) {
	f, m := playingModel(t)
	e := libEntry{kind: kindPlaylist, key: "pl", uri: "spotify:playlist:pl", name: "List", ownerID: "me", total: 4}
	m.lib.entries = []libEntry{e}
	m.page, m.focus = pageLibrary, focusContent
	m.lib.open = &trackList{entry: e, total: 4, tracks: []spotify.Track{
		{Name: "Around the World", URI: "spotify:track:a", Artists: []spotify.Artist{{Name: "Daft Punk"}}, DurationMS: 400000, AddedAt: "2024-01-02"},
		{Name: "Bad Guy", URI: "spotify:track:b", Artists: []spotify.Artist{{Name: "Billie Eilish"}}, DurationMS: 190000, AddedAt: "2025-05-01"},
		{Name: "Cola", URI: "spotify:track:c", Artists: []spotify.Artist{{Name: "Arlo Parks"}}, DurationMS: 250000, AddedAt: "2023-03-03"},
		{Name: "Digital Love", URI: "spotify:track:d", Artists: []spotify.Artist{{Name: "Daft Punk"}}, DurationMS: 300000, AddedAt: "2026-06-06"},
	}}
	return f, m
}

func typeFind(m Model, s string) Model {
	for _, r := range s {
		m = m.pressKey(string(r))
	}
	return m
}

func TestFindFiltersOpenListAndPlaysTheRightSong(t *testing.T) {
	f, m := findList(t)
	m = m.pressKey("/")
	if !m.lib.open.find.typing {
		t.Fatal("/ did not start the filter")
	}
	m = typeFind(m, "daft q") // q yazıya gider, çıkmaz
	if m.lib.open.find.query != "daft q" {
		t.Fatalf("query %q", m.lib.open.find.query)
	}
	m = m.pressKey("backspace")
	m = m.pressKey("backspace")
	if got := m.lib.open.rows(); got != 2 {
		t.Fatalf("rows %d", got)
	}
	screen := ansi.Strip(strings.Join(m.libraryView(80, 30), "\n"))
	if !strings.Contains(screen, "/ daft") || strings.Contains(screen, "Bad Guy") {
		t.Fatalf("screen:\n%s", screen)
	}
	// enter: sonuçlara geçer; ↓ ikinci sonuç; enter çalar (Digital Love).
	m = m.pressKey("enter")
	if m.lib.open.find.typing {
		t.Fatal("enter should stop typing")
	}
	m = m.pressKey("j")
	if m.lib.open.cursor != 3 {
		t.Fatalf("cursor %d", m.lib.open.cursor)
	}
	f.on("GET me/player/devices", `{"devices":[]}`)
	m = m.pressKey("enter")
	play := f.sent("PUT me/player/play")
	if len(play) != 1 || !strings.Contains(f.bodyOf(play[0]), "spotify:track:d") {
		t.Fatalf("play %v %s", play, f.bodyOf(play[0]))
	}
	// esc: süzgeç kalkar, seçili şarkıda kalınır; ikinci esc listeden çıkar.
	m = m.pressKey("esc")
	if m.lib.open == nil || m.lib.open.find.active() || m.lib.open.cursor != 3 || m.lib.open.rows() != 4 {
		t.Fatalf("esc: %+v", m.lib.open)
	}
	m = m.pressKey("esc")
	if m.lib.open != nil {
		t.Fatal("second esc should leave the list")
	}
}

func TestFindLoadsTheWholeList(t *testing.T) {
	f, m := playingModel(t)
	e := libEntry{kind: kindPlaylist, key: "big", uri: "spotify:playlist:big", name: "Big", total: 60}
	m.lib.entries = []libEntry{e}
	var first []spotify.Track
	for i := 0; i < 50; i++ {
		first = append(first, spotify.Track{Name: fmt.Sprintf("song %d", i), URI: fmt.Sprintf("spotify:track:t%d", i)})
	}
	var items []string
	for i := 50; i < 60; i++ {
		items = append(items, fmt.Sprintf(`{"item":{"uri":"spotify:track:t%d","name":"late %d"}}`, i, i))
	}
	f.on("GET playlists/big/items", `{"total":60,"items":[`+strings.Join(items, ",")+`]}`)
	m.page, m.focus = pageLibrary, focusContent
	m.lib.open = &trackList{entry: e, tracks: first, total: 60}
	m = m.pressKey("/")
	m = typeFind(m, "late 55")
	if len(m.lib.open.tracks) != 60 || m.lib.open.rows() != 1 {
		t.Fatalf("tracks %d rows %d", len(m.lib.open.tracks), m.lib.open.rows())
	}
}

func TestFindInLibraryLevelSearchesFolders(t *testing.T) {
	_, m := playingModel(t)
	m.lib.loaded = true
	m.lib.entries = []libEntry{
		{kind: kindLiked, key: likedKey, name: "Liked Songs"},
		{kind: kindPlaylist, key: "a", uri: "spotify:playlist:a", name: "Gym", ownerID: "me"},
		{kind: kindPlaylist, key: "b", uri: "spotify:playlist:b", name: "Sleepy", ownerID: "me"},
	}
	m.page, m.focus = pageLibrary, focusContent
	m.pressKey("ctrl+f") // ctrl+f de başlatır
	m = m.pressKey("/")
	m = typeFind(m, "slp")
	vis := m.libVisible()
	if len(vis) != 1 || vis[0].name != "Sleepy" {
		t.Fatalf("visible %v", names(vis))
	}
}

func TestSortMenuSortsFlipsAndRemembers(t *testing.T) {
	_, m := findList(t)
	m = m.pressKey("O")
	if !m.sortm.open {
		t.Fatal("shift+O did not open the sort menu")
	}
	// Seçenekler: kendi sırası, ad, sanatçı, albüm, eklenme, süre.
	m = m.pressKey("j") // ad
	m = m.pressKey("enter")
	tl := m.lib.open
	if tl.sortBy != sortName || tl.desc {
		t.Fatalf("sort %q desc %v", tl.sortBy, tl.desc)
	}
	order := func() string {
		var s []string
		for p := 0; p < tl.rows(); p++ {
			s = append(s, tl.tracks[tl.at(p)].URI[len("spotify:track:"):])
		}
		return strings.Join(s, "")
	}
	if got := order(); got != "abcd" {
		t.Fatalf("by name %s", got)
	}
	m = m.pressKey("O")
	m = m.pressKey("enter") // aynısı: ters
	if got := order(); got != "dcba" || !tl.desc {
		t.Fatalf("flipped %s", got)
	}
	// Süreye göre.
	m = m.pressKey("O")
	for i := 0; i < 5; i++ {
		m = m.pressKey("j")
	}
	m = m.pressKey("enter")
	if got := order(); got != "bcda" {
		t.Fatalf("by duration %s", got)
	}
	// Eklenme: en yeni üstte başlar.
	m = m.pressKey("O")
	m = m.pressKey("k")
	m = m.pressKey("enter")
	if got := order(); got != "dbac" {
		t.Fatalf("by added %s", got)
	}
	// Sıralıyken taşıma yok.
	m = m.pressKey("J")
	if m.flash != T("find.nomove") {
		t.Fatalf("flash %q", m.flash)
	}
	// Saklanır: liste yeniden açılınca aynı sıralama.
	if loadSorts(m.cfg.LocalDir)["pl"] != (sortPref{By: sortAdded, Desc: true}) {
		t.Fatalf("saved %v", loadSorts(m.cfg.LocalDir))
	}
	tl2, _ := m.openList(m.lib.entries[0])
	if tl2.sortBy != sortAdded || !tl2.desc {
		t.Fatal("sort not restored")
	}
	// Kendi sırasına dönünce kayıt silinir.
	m = m.pressKey("O")
	for i := 0; i < 6; i++ {
		m = m.pressKey("k")
	}
	m = m.pressKey("enter")
	if _, ok := loadSorts(m.cfg.LocalDir)["pl"]; ok || tl.sortBy != "" {
		t.Fatal("custom order should clear the saved sort")
	}
}

func TestLibrarySortByName(t *testing.T) {
	_, m := playingModel(t)
	m.lib.loaded = true
	m.lib.entries = []libEntry{
		{kind: kindLiked, key: likedKey, name: "Liked Songs"},
		{kind: kindPlaylist, key: "z", name: "zebra"},
		{kind: kindPlaylist, key: "a", name: "Ärger"},
		{kind: kindPlaylist, key: "m", name: "Mango"},
	}
	m.page, m.focus = pageLibrary, focusContent
	m = m.pressKey("O")
	m = m.pressKey("j")
	m = m.pressKey("j") // ada göre
	m = m.pressKey("enter")
	if got := names(m.libVisible()); strings.Join(got, ",") != "Liked Songs,Ärger,Mango,zebra" {
		t.Fatalf("library by name: %v", got)
	}
	if loadSorts(m.cfg.LocalDir)[libSortKey].By != sortName {
		t.Fatal("library sort not saved")
	}
}

func TestFindPasteAddsToQuery(t *testing.T) {
	_, m := findList(t)
	m = m.pressKey("/")
	out, cmd := m.Update(tea.PasteMsg{Content: "digital\n"})
	m = settle(out.(Model), cmd)
	if m.lib.open.find.query != "digital" || m.lib.open.rows() != 1 {
		t.Fatalf("query %q rows %d", m.lib.open.find.query, m.lib.open.rows())
	}
}
