package ui

import (
	"strings"
	"testing"

	"github.com/Ehtisam/sotto/internal/spclient"
	"github.com/Ehtisam/sotto/internal/spotify"
)

func profileModel(t *testing.T) Model {
	t.Helper()
	m := homeModel(t)
	m.spState = spReady
	m.page, m.focus = pageProfile, focusContent
	m.me = spotify.User{ID: "me", DisplayName: "Me", Product: "premium"}
	m.me.Followers.Total = 6
	m.lib.loaded = true
	m.lib.entries = []libEntry{
		{kind: kindLiked, key: likedKey, name: "Liked Songs", total: 340},
		{kind: kindPlaylist, key: "pub", uri: "spotify:playlist:pub", name: "Public mine", ownerID: "me", public: true, total: 12},
		{kind: kindPlaylist, key: "priv", uri: "spotify:playlist:priv", name: "Private mine", ownerID: "me"},
		{kind: kindPlaylist, key: "other", uri: "spotify:playlist:other", name: "Someone's", ownerID: "x", public: true},
	}
	m.prof.tops[0] = profileTop{
		artists: []spotify.FullArtist{{Artist: spotify.Artist{Name: "A1", URI: "spotify:artist:a1"}}},
		tracks:  []spotify.Track{{Name: "T1", URI: "spotify:track:t1", Artists: []spotify.Artist{{Name: "A1"}}}},
	}
	m.prof.tops[0].fetched = m.now
	at := m.now.Add(-5 * 60e9)
	m.home.recent = []spotify.PlayHistory{play("x", "", "", at), play("y", "", "", at)} // ikisi de "Art"
	return m
}

func TestProfilePanels(t *testing.T) {
	m := profileModel(t)
	titles, items := m.profilePanels()
	want := [profilePanels][2]string{
		{"Top artists · last 4 weeks", "A1"},
		{"Top songs", "T1"},
		{"Recently played artists", "Art"}, // iki şarkı, aynı sanatçı: bir kez
		{"Your public playlists", "Public mine"},
	}
	for i, w := range want {
		if titles[i] != w[0] || len(items[i]) != 1 || items[i][0].name != w[1] {
			t.Errorf("panel %d = %q %+v, want %v", i, titles[i], items[i], w)
		}
	}
	if got := len(m.publicPlaylists()); got != 1 {
		t.Fatalf("public playlists = %d, want 1 (only own and public)", got)
	}
}

func TestProfileNavigation(t *testing.T) {
	m := profileModel(t)
	steps := []struct {
		key   string
		panel int
	}{
		{"right", 1}, // sanatçılar → şarkılar
		{"down", 3},  // şarkıların sonu → alttaki listeler
		{"left", 2},  // listeler → son dinlenen sanatçılar
		{"up", 0},    // yukarıdaki sanatçılar
	}
	for _, s := range steps {
		m, _, _ = m.profileKey(s.key)
		if m.prof.panel != s.panel {
			t.Fatalf("after %s panel = %d, want %d", s.key, m.prof.panel, s.panel)
		}
	}
	m, _, _ = m.profileKey("left") // sol sütunda sol: menüye döner
	if m.focus != focusMenu {
		t.Fatalf("left on the left column should go back to the menu")
	}
}

func TestProfileRangeKeys(t *testing.T) {
	m := profileModel(t)
	m, cmd, _ := m.profileKey("]")
	if m.prof.rng != 1 || cmd == nil || !m.prof.tops[1].loading {
		t.Fatalf("] → rng %d loading %v", m.prof.rng, m.prof.tops[1].loading)
	}
	m, _, _ = m.profileKey("[")
	m, _, _ = m.profileKey("[")
	if m.prof.rng != 2 {
		t.Fatalf("[ [ → rng %d, want 2 (wraps)", m.prof.rng)
	}
}

func TestProfileActionsTarget(t *testing.T) {
	m := profileModel(t)
	m, _, _ = m.profileKey("right") // A1 → T1
	target, _, ok := m.actionTarget()
	if !ok || target.uri != "spotify:track:t1" {
		t.Fatalf("target = %+v", target)
	}
}

func TestProfileShowsWhatIsOnTheProfile(t *testing.T) {
	m := profileModel(t)
	// Profilde: kitaplıktaki özel bir liste (herkese açık olmak şart değil) ve kitaplıkta olmayan biri.
	m.prof.view = &spclient.Profile{Total: 2, Playlists: []spclient.ProfilePlaylist{
		{URI: "spotify:playlist:priv", Name: "Private mine"},
		{URI: "spotify:playlist:zzz", Name: "Not in library", OwnerName: "me"},
	}}
	titles, items := m.profilePanels()
	if titles[3] != "On your profile" || len(items[3]) != 2 {
		t.Fatalf("panel 3 = %q %+v", titles[3], items[3])
	}
	if items[3][1].entry.key != "zzz" || items[3][1].sub != "me" {
		t.Fatalf("entry outside the library = %+v", items[3][1])
	}
	head := strings.Join(m.profileHeader(), " ")
	if !strings.Contains(head, "3 playlists") || !strings.Contains(head, "2 on your profile") || strings.Contains(head, "public") {
		t.Fatalf("header = %q", head)
	}
}

// Update iç yoldan gelen cevabı profile işlemeli (önceden yok sayılıyordu).
func TestProfileViewMsgReachesTheModel(t *testing.T) {
	m := profileModel(t)
	m.prof.viewLoading = true
	v := &spclient.Profile{Total: 1, Playlists: []spclient.ProfilePlaylist{{URI: "spotify:playlist:priv", Name: "Private mine"}}}
	mm, _ := m.Update(profileViewMsg{v})
	m = mm.(Model)
	if m.prof.view != v || m.prof.viewLoading {
		t.Fatalf("view = %v loading = %v", m.prof.view, m.prof.viewLoading)
	}
	if titles, _ := m.profilePanels(); titles[3] != "On your profile" {
		t.Fatalf("panel 3 = %q", titles[3])
	}
}

// Menüde ok tuşlarıyla Profil'e gelince veri hemen istenir (içine girmeyi beklemez).
func TestArrowIntoProfileStartsLoading(t *testing.T) {
	m := profileModel(t)
	m.prof.tops[0] = profileTop{}
	m.page, m.focus = pageDevices, focusMenu
	mm, cmd := m.key(keyPress("j"))
	m = mm.(Model)
	if m.page != pageProfile || m.focus != focusMenu {
		t.Fatalf("j → page %d focus %d", m.page, m.focus)
	}
	if cmd == nil || !m.prof.tops[0].loading {
		t.Fatal("profile data should start loading while just passing over it in the menu")
	}
}
