package spclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// base, iç uçların adresi (testler değiştirir).
var base = "https://spclient.wg.spotify.com"

// ErrNoRadio: Spotify bu tohumdan radyo üretmedi.
var ErrNoRadio = errors.New("spotify made no radio for this")

// Radio, bir şarkıdan ya da sanatçıdan (seedURI) Spotify'ın radyo listesini
// üretir ve adresini döner. Spotify uygulamasındaki "radyosuna git" ile aynı.
func Radio(ctx context.Context, token, seedURI string) (string, error) {
	var r struct {
		MediaItems []struct {
			URI string `json:"uri"`
		} `json:"mediaItems"`
	}
	if err := getJSON(ctx, token, "/inspiredby-mix/v2/seed_to_playlist/"+url.PathEscape(seedURI)+"?response-format=json", &r); err != nil {
		return "", err
	}
	if len(r.MediaItems) == 0 || r.MediaItems[0].URI == "" {
		return "", ErrNoRadio
	}
	return r.MediaItems[0].URI, nil
}

// ContextInfo, bir listenin (Spotify'ın kendi listeleri dahil) adı, kapağı
// ve ilk sayfadaki şarkıları.
type ContextInfo struct {
	Name   string
	Image  string
	Tracks []string // şarkı adresleri
}

// Context, bir listenin bilgisini iç yoldan getirir. Resmî Web API Spotify'ın
// kendi listelerini (radyo, Daily Mix) kendi uygulamalara vermiyor.
func Context(ctx context.Context, token, uri string) (ContextInfo, error) {
	var r struct {
		Metadata map[string]string `json:"metadata"`
		Pages    []struct {
			Tracks []struct {
				URI string `json:"uri"`
			} `json:"tracks"`
		} `json:"pages"`
	}
	if err := getJSON(ctx, token, "/context-resolve/v1/"+url.PathEscape(uri), &r); err != nil {
		return ContextInfo{}, err
	}
	info := ContextInfo{Name: r.Metadata["context_description"], Image: r.Metadata["image_url"]}
	if len(r.Pages) > 0 {
		for _, t := range r.Pages[0].Tracks {
			if t.URI != "" {
				info.Tracks = append(info.Tracks, t.URI)
			}
		}
	}
	return info, nil
}

func getJSON(ctx context.Context, token, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return ErrUnauthorized
	default:
		return fmt.Errorf("spotify: %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
