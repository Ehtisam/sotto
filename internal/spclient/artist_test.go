package spclient

import (
	"encoding/json"
	"testing"
)

func TestParseArtist(t *testing.T) {
	raw := `{"header":{"text":{"title":"Athena","accessory":"1M monthly listeners"},"custom":{"verified":true},"images":{"main":{"uri":"img"}}},
"body":[
 {"id":"artist-entity-view-top-tracks-combined","text":{"title":"Popular"}},
 {"id":"artist-entity-view-top-tracks-combined_row0","text":{"title":"Dam","subtitle":"33,324,983"},"metadata":{"sectionId":"artist-entity-view-top-tracks-combined","uri":"spotify:track:p1","album_uri":"spotify:album:al1"}},
 {"id":"artist-entity-view-releases_row0","text":{"title":"Altüst","subtitle":"2014"},"metadata":{"uri":"spotify:album:al1","sectionId":"artist-entity-view-releases"}},
 {"id":"artist-entity-view-releases_link","text":{"title":"See discography"},"metadata":{"uri":"spotify:artist:a1:releases","sectionId":"artist-entity-view-releases"}},
 {"id":"artist-entity-view-you-might-also-like","text":{"title":"Featuring"},"children":[{"id":"artist-entity-view-you-might-also-like_card0","text":{"title":"This Is Athena","description":"d"},"metadata":{"uri":"spotify:playlist:x","sectionId":"artist-entity-view-you-might-also-like"}}]},
 {"id":"artist-entity-view-related","children":[{"id":"artist-entity-view-related_card0","text":{"title":"Kargo"},"metadata":{"uri":"spotify:artist:a2","sectionId":"artist-entity-view-related"}}]},
 {"id":"artist-entity-view-appears-on-section","children":[{"id":"artist-entity-view-appears-on-section_card0","text":{"title":"Comp","subtitle":"2012"},"metadata":{"uri":"spotify:album:al9","sectionId":"artist-entity-view-appears-on-section"}}]}
]}`
	var r struct {
		Header hubNode   `json:"header"`
		Body   []hubNode `json:"body"`
	}
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatal(err)
	}
	v := parseArtist("a1", r.Header, r.Body)
	if v.Name != "Athena" || !v.Verified || v.Listeners != "1M monthly listeners" || v.Image != "img" {
		t.Fatalf("header %+v", v)
	}
	if len(v.Popular) != 1 || v.Popular[0].AlbumURI != "spotify:album:al1" || v.Popular[0].Plays != "33,324,983" {
		t.Fatalf("popular %+v", v.Popular)
	}
	if len(v.Releases) != 1 || v.Releases[0].Sub != "2014" {
		t.Fatalf("releases %+v (the discography link must not be a release)", v.Releases)
	}
	if len(v.Featuring) != 1 || v.Featuring[0].Sub != "d" || len(v.Related) != 1 || len(v.AppearsOn) != 1 {
		t.Fatalf("cards %+v %+v %+v", v.Featuring, v.Related, v.AppearsOn)
	}
}

func TestGID(t *testing.T) {
	g, err := GID("1Hy8tdpH9ygqoohumHZqVl")
	if err != nil || g != "37efbbd9b2ef484e8db0c54d14ae2853" {
		t.Fatalf("%s %v", g, err)
	}
	if _, err := GID("bad!"); err == nil {
		t.Fatal("bad id accepted")
	}
}
