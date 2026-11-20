package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/spotify"
)

func TestRecentSearches(t *testing.T) {
	m := queueModel(t)
	m.cfg.LocalDir = t.TempDir()
	m.page, m.focus = pageSearch, focusContent

	tr := spotify.Track{URI: "spotify:track:t1", Name: "Song", Album: spotify.Album{URI: "spotify:album:a1"}}
	song := searchItem{kind: "track", name: "Song", sub: "Singer", uri: tr.URI, track: &tr}
	list := searchItem{kind: "playlist", name: "Mix", uri: "spotify:playlist:p1",
		entry: libEntry{kind: kindPlaylist, key: "p1", uri: "spotify:playlist:p1", name: "Mix", ownerID: "me", public: true}}
	artist := searchItem{kind: "artist", name: "Band", uri: "spotify:artist:b1"}

	for _, it := range []searchItem{song, list, artist, song} {
		mm, c := m.rememberSearch(it)
		m = mm
		if c != nil {
			c() // dosyaya yaz
		}
	}
	// En son açılan başta, aynı öğe bir kez.
	got := []string{}
	for _, s := range m.search.recent {
		got = append(got, s.Name)
	}
	if len(got) != 3 || got[0] != "Song" || got[1] != "Band" || got[2] != "Mix" {
		t.Fatalf("recent = %v", got)
	}

	// Dosyadan geri okunur; liste açılabilir, şarkı albümünde çalınabilir.
	back := loadRecent(m.cfg.LocalDir)
	if len(back) != 3 {
		t.Fatalf("loaded %d", len(back))
	}
	if it := back[2].item(); it.entry.kind != kindPlaylist || it.entry.key != "p1" || !it.entry.public {
		t.Fatalf("playlist entry = %+v", it.entry)
	}
	if it := back[0].item(); it.track == nil || it.track.Album.URI != "spotify:album:a1" {
		t.Fatalf("track = %+v", it.track)
	}
	if it := back[1].item(); it.sub != "Artist" {
		t.Fatalf("artist sub = %q", it.sub)
	}

	// Kutu boşken son aramalar listelenir; x seçiliyi siler.
	m.search.query = ""
	m = m.enterSearch()
	if kinds := kindsOf(m.search.items); len(kinds) != 4 || kinds[0] != "header" {
		t.Fatalf("items = %v", kinds)
	}
	m.search.typing = false
	m.search.cursor = 2 // Band
	mm, _, ok := m.searchKey(keyPress("x"), "x")
	if !ok || len(mm.search.recent) != 2 || mm.search.recent[1].Name != "Mix" {
		t.Fatalf("after x: %+v", mm.search.recent)
	}
	if it := mm.search.items[mm.search.cursor]; !it.selectable() {
		t.Fatalf("cursor on %q", it.kind)
	}
}

// Arama yoldayken kutu boşaltılırsa "yükleniyor" kalmamalı (kullanıcı buldu).
func TestSearchClearedWhileLoading(t *testing.T) {
	m := queueModel(t)
	m.page, m.focus = pageSearch, focusContent
	m = m.enterSearch()
	m.search.query = "ab"
	m, _ = m.runSearch(0)
	if !m.search.loading {
		t.Fatal("search should be loading")
	}
	m.search.query = ""
	m, _ = m.runSearch(0)
	out, _ := m.Update(searchResultMsg{query: "ab"}) // geç gelen eski sonuç
	m = out.(Model)
	if s := strings.Join(m.searchView(80), "\n"); strings.Contains(s, "loading") {
		t.Fatalf("still loading:\n%s", s)
	}
}

// ESC ile boşaltıp aynı sözcüğü hemen yeniden yazınca yeniden aranmalı.
func TestSearchRetypeAfterEsc(t *testing.T) {
	m := queueModel(t)
	m.page, m.focus = pageSearch, focusContent
	m = m.enterSearch()
	m.search.query, m.search.last = "tarkan", "tarkan"
	m.search.items = []searchItem{{kind: "track", name: "x", uri: "u"}}
	m, _, _ = m.searchKey(tea.KeyPressMsg{Code: tea.KeyEscape}, "esc")
	if m.search.last != "" || m.search.query != "" {
		t.Fatalf("esc: query %q last %q", m.search.query, m.search.last)
	}
	m = typeText2(m, "tarkan")
	mm, cmd := m.updateSearch(searchFireMsg{seq: m.search.seq})
	if cmd == nil || !mm.(Model).search.loading {
		t.Fatal("retyped word was not searched again")
	}
}

func typeText2(m Model, s string) Model {
	for _, r := range s {
		m, _, _ = m.searchKey(tea.KeyPressMsg{Text: string(r), Code: r}, string(r))
	}
	return m
}
