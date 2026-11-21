package spclient

import (
	"encoding/json"
	"testing"
)

func TestProfileDecode(t *testing.T) {
	raw := `{"followers_count":6,"following_count":12,"total_public_playlists_count":2,
		"public_playlists":[{"uri":"spotify:playlist:a","name":"A","image_url":"x","owner_name":"me","is_following":true},
		{"uri":"spotify:playlist:b","name":"B"}]}`
	var p Profile
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatal(err)
	}
	if p.Followers != 6 || p.FollowingN != 12 || p.Total != 2 || len(p.Playlists) != 2 || p.Playlists[0].Name != "A" {
		t.Fatalf("decoded %+v", p)
	}
}
