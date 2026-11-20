// Package spclient, Spotify'ın masaüstü uygulamasının kullandığı iç
// bağlantılardan (✱) resmî Web API'de olmayan bilgileri okur. Belgesi yok,
// haber verilmeden değişebilir; jetonu bridge paketi sağlar.
package spclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// ErrUnauthorized: jeton reddedildi, yenisi alınmalı.
var ErrUnauthorized = errors.New("spclient: token rejected")

var client = &http.Client{}

// ProfilePlaylist, profilde görünen bir liste.
type ProfilePlaylist struct {
	URI       string `json:"uri"`
	Name      string `json:"name"`
	ImageURL  string `json:"image_url"`
	OwnerName string `json:"owner_name"`
}

// Profile, kullanıcının profil sayfasında görünenler.
type Profile struct {
	Followers int               `json:"followers_count"`
	Following int               `json:"following_count"` // kişiler ve sanatçılar
	Total     int               `json:"total_public_playlists_count"`
	Playlists []ProfilePlaylist `json:"public_playlists"`
}

// ProfileView, kullanıcının profilini getirir: profilde gösterilen listeler
// (herkese açık olmak profilde görünmek demek değil) ve sayılar.
func ProfileView(ctx context.Context, token, userID string) (Profile, error) {
	u := "https://spclient.wg.spotify.com/user-profile-view/v3/profile/" + url.PathEscape(userID) +
		"?playlist_limit=200&artist_limit=0&episode_limit=0&market=from_token"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Profile{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return Profile{}, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return Profile{}, ErrUnauthorized
	default:
		return Profile{}, fmt.Errorf("spotify profile: %s", resp.Status)
	}
	var p Profile
	err = json.NewDecoder(resp.Body).Decode(&p)
	return p, err
}
