package spotify

import "testing"

func TestArtistAlbumGroup(t *testing.T) {
	own := []Artist{{ID: "a1", URI: "spotify:artist:a1"}}
	other := []Artist{{ID: "va", URI: "spotify:artist:va"}}
	mk := func(group, typ string, artists []Artist) ArtistAlbum {
		var a ArtistAlbum
		a.AlbumGroup, a.AlbumType, a.Artists = group, typ, artists
		return a
	}
	cases := []struct {
		a    ArtistAlbum
		want string
	}{
		{mk("single", "album", own), "single"}, // Spotify söylüyorsa o
		{mk("", "album", own), "album"},
		{mk("", "single", own), "single"},
		{mk("", "compilation", own), "compilation"},
		{mk("", "compilation", other), "appears_on"},
		{mk("", "album", other), "appears_on"},
	}
	for i, c := range cases {
		if got := c.a.Group("a1"); got != c.want {
			t.Errorf("%d: %q, want %q", i, got, c.want)
		}
	}
}
