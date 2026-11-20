package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/spclient"
)

func names(es []libEntry) []string {
	var out []string
	for _, e := range es {
		out = append(out, e.name)
	}
	return out
}

func TestLibraryFilters(t *testing.T) {
	m := profileModel(t) // Liked, Public mine, Private mine, Someone's
	m.page, m.focus = pageLibrary, focusContent
	m.prof.view = &spclient.Profile{Playlists: []spclient.ProfilePlaylist{{URI: "spotify:playlist:priv"}}}
	want := map[libFilter][]string{
		libAll:       {"Liked Songs", "Public mine", "Private mine", "Someone's"},
		libMine:      {"Liked Songs", "Public mine", "Private mine"},
		libPublic:    {"Public mine"}, // başkasının herkese açık listesi sayılmaz
		libOnProfile: {"Private mine"},
		libFollowed:  {"Someone's"},
	}
	for f, w := range want {
		m.lib.filter = f
		got := names(m.libVisible())
		if len(got) != len(w) {
			t.Fatalf("filter %d = %v, want %v", f, got, w)
		}
		for i := range w {
			if got[i] != w[i] {
				t.Fatalf("filter %d = %v, want %v", f, got, w)
			}
		}
	}
}

func TestLibraryTabCyclesFiltersAndOpensFiltered(t *testing.T) {
	m := profileModel(t)
	m.page, m.focus = pageLibrary, focusContent
	m.lib.filter = libAll
	mm, _ := m.key(keyPress("tab"))
	m = mm.(Model)
	if m.lib.filter != libMine || m.focus != focusContent {
		t.Fatalf("tab → filter %d focus %d", m.lib.filter, m.focus)
	}
	mm, _ = m.key(keyPress("tab"))
	m = mm.(Model)
	if m.lib.filter != libPublic {
		t.Fatalf("second tab → filter %d", m.lib.filter)
	}
	// Süzgeçteki ilk liste açılır, kitaplığın ilki değil.
	mm, _ = m.key(keyPress("enter"))
	m = mm.(Model)
	if m.lib.open == nil || m.lib.open.entry.name != "Public mine" {
		t.Fatalf("opened %+v", m.lib.open)
	}
}

func TestTabStillLeavesOtherPages(t *testing.T) {
	m := profileModel(t)
	m.page, m.focus = pageHome, focusContent
	mm, _ := m.key(keyPress("tab"))
	if mm.(Model).focus != focusMenu {
		t.Fatal("tab on Home should go back to the menu")
	}
}

func TestProfileTabAndPageJumps(t *testing.T) {
	m := profileModel(t)
	for _, want := range []int{1, 2, 3, 0} {
		m, _, _ = m.profileKey("tab")
		if m.prof.panel != want {
			t.Fatalf("tab → panel %d, want %d", m.prof.panel, want)
		}
	}
	m, _, _ = m.profileKey("shift+tab")
	if m.prof.panel != 3 {
		t.Fatalf("shift+tab → panel %d, want 3", m.prof.panel)
	}
	m.prof.panel = 1
	m, _, _ = m.profileKey("pgdown") // şarkılardan doğrudan profildeki listelere
	if m.prof.panel != 3 {
		t.Fatalf("pgdown → panel %d, want 3", m.prof.panel)
	}
}

func keyPress(k string) tea.KeyPressMsg {
	switch k {
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	}
	return tea.KeyPressMsg{Text: k, Code: rune(k[0])}
}

func TestNumberKeysGoIntoThePage(t *testing.T) {
	m := profileModel(t)
	m.km.Counts = false // sayılar açıkken içerikte rakam kısa bir süre bekler (keymap_test)
	m.page, m.focus = pageHome, focusContent
	mm, _ := m.key(keyPress("7"))
	m = mm.(Model)
	if m.page != pageProfile || m.focus != focusContent {
		t.Fatalf("7 → page %d focus %d, want profile, content", m.page, m.focus)
	}
	mm, _ = m.key(keyPress("1")) // Şimdi çalıyor'un gezilecek içi yok: menüde kalır
	m = mm.(Model)
	if m.page != pageNow || m.focus != focusMenu {
		t.Fatalf("1 → page %d focus %d", m.page, m.focus)
	}
	mm, _ = m.key(keyPress("3")) // Ara: yazı kutusu hazır
	m = mm.(Model)
	if m.page != pageSearch || m.focus != focusContent || !m.search.typing {
		t.Fatalf("3 → page %d focus %d typing %v", m.page, m.focus, m.search.typing)
	}
}

func TestLibraryShowsOwnerOfFollowedPlaylists(t *testing.T) {
	m := profileModel(t)
	m.page, m.focus = pageLibrary, focusMenu
	m.lib.entries[3].owner = "Someone Else"
	view := strings.Join(m.libraryView(100, 30), " ")
	if !strings.Contains(view, "Someone Else") {
		t.Fatal("followed playlist should show its owner")
	}
	if strings.Count(view, "Me ·") > 0 {
		t.Fatal("own playlists should not show the owner")
	}
}

func TestLibraryMarksProfileThenPublic(t *testing.T) {
	m := profileModel(t)
	m.page, m.focus = pageLibrary, focusMenu
	m.lib.entries = append(m.lib.entries, libEntry{kind: kindPlaylist, key: "pp", uri: "spotify:playlist:pp", name: "On profile", ownerID: "me", public: true, total: 5})
	// Profil bilgisi yokken: herkese açık olanlar "public".
	view := strings.Join(m.libraryView(100, 30), " ")
	if strings.Count(view, "public ·") != 2 || strings.Contains(view, "on profile ·") {
		t.Fatalf("without the profile view: %q", view)
	}
	// Profil bilgisi gelince: profildeki "on profile", diğeri yine "public", gizli işaretsiz.
	m.prof.view = &spclient.Profile{Playlists: []spclient.ProfilePlaylist{{URI: "spotify:playlist:pp"}}}
	view = strings.Join(m.libraryView(100, 30), " ")
	if strings.Count(view, "on profile ·") != 1 || strings.Count(view, "public ·") != 1 {
		t.Fatalf("with the profile view: %q", view)
	}
}

func TestEnteringLibraryAsksForTheProfileView(t *testing.T) {
	m := profileModel(t)
	m.cfg.Extras.Enabled = true
	m.page, m.focus = pageHome, focusMenu
	mm, cmd := m.key(keyPress("4"))
	m = mm.(Model)
	if m.page != pageLibrary || cmd == nil || !m.prof.viewLoading {
		t.Fatalf("4 → page %d, view loading %v", m.page, m.prof.viewLoading)
	}
}
