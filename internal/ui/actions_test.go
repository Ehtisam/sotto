package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/spotify"
)

func libraryWithOpenList(t *testing.T) Model {
	t.Helper()
	m := homeModel(t)
	m.spState = spReady
	m.me.ID = "me"
	m.lib.entries = []libEntry{
		{kind: kindLiked, key: likedKey, name: "Liked Songs"},
		{kind: kindPlaylist, key: "mine", uri: "spotify:playlist:mine", name: "Mine", ownerID: "me"},
		{kind: kindPlaylist, key: "theirs", uri: "spotify:playlist:theirs", name: "Theirs", ownerID: "someone"},
		{kind: kindPlaylist, key: "shared", uri: "spotify:playlist:shared", name: "Shared", ownerID: "someone", collaborative: true},
	}
	m.page, m.focus = pageLibrary, focusContent
	m.lib.open = &trackList{entry: m.lib.entries[1], tracks: []spotify.Track{{
		Name: "Song", URI: "spotify:track:s",
		Album:   spotify.Album{ID: "al", URI: "spotify:album:al", Name: "Album"},
		Artists: []spotify.Artist{{Name: "Art", URI: "spotify:artist:ar"}},
	}}, total: 1}
	return m
}

func ids(items []menuItem) []actID {
	var out []actID
	for _, it := range items {
		out = append(out, it.id)
	}
	return out
}

func TestActionsOnTrackInOwnPlaylist(t *testing.T) {
	m := libraryWithOpenList(t)
	m, cmd := m.openActions()
	if !m.actions.open || cmd == nil {
		t.Fatal("menu should open and ask whether the song is liked")
	}
	if m.actions.target.uri != "spotify:track:s" {
		t.Fatalf("target = %q", m.actions.target.uri)
	}
	// Beğeni durumu gelmeden "beğen" seçeneği yok; listeden çıkar var.
	want := []actID{actQueue, actAddList, actRemoveList, actAlbum, actArtist, actInfo, actLink}
	if got := ids(m.actions.items); len(got) != len(want) {
		t.Fatalf("items = %v, want %v", got, want)
	}
	out, _ := m.Update(savedStateMsg{uri: "spotify:track:s", saved: true})
	m = out.(Model)
	if m.actions.items[1].id != actUnlike {
		t.Fatalf("liked song should offer unlike, got %v", ids(m.actions.items))
	}
}

func TestQDoesNotQuitInsideMenu(t *testing.T) {
	m := libraryWithOpenList(t)
	m, _ = m.openActions()
	out, cmd := m.Update(tea.KeyPressMsg{Text: "q", Code: 'q'})
	m = out.(Model)
	if m.actions.open {
		t.Fatal("q should run 'add to queue' and close the menu")
	}
	if cmd == nil {
		t.Fatal("q should send the queue request")
	}
	if msg := cmd(); msg == nil {
		t.Fatal("expected a result message")
	} else if _, quit := msg.(tea.QuitMsg); quit {
		t.Fatal("q must not quit sotto while the menu is open")
	}
}

func TestAddToPlaylistListsOnlyEditable(t *testing.T) {
	m := libraryWithOpenList(t)
	m, _ = m.openActions()
	m, _ = m.runAction(actAddList)
	if !m.actions.picking {
		t.Fatal("should be picking a playlist")
	}
	var names []string
	for _, e := range m.actions.lists {
		names = append(names, e.name)
	}
	if len(names) != 2 || names[0] != "Mine" || names[1] != "Shared" {
		t.Fatalf("lists = %v, want [Mine Shared]", names)
	}
	out, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = out.(Model)
	if m.actions.picking || !m.actions.open {
		t.Fatal("esc should go back to the menu")
	}
}

func TestRemoveUpdatesOpenList(t *testing.T) {
	m := libraryWithOpenList(t)
	out, _ := m.Update(actionDoneMsg{note: "x", removed: "spotify:track:s", fromKey: "mine"})
	m = out.(Model)
	if len(m.lib.open.tracks) != 0 || m.lib.open.total != 0 {
		t.Fatal("removed song should disappear from the open list")
	}
}
