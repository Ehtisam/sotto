package spclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func fake(t *testing.T, routes map[string]string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, ok := routes[r.URL.EscapedPath()]
		if !ok {
			t.Logf("unexpected %s", r.URL.EscapedPath())
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	old := base
	base = srv.URL
	t.Cleanup(func() { base = old })
}

func TestRadioAndContext(t *testing.T) {
	fake(t, map[string]string{
		"/inspiredby-mix/v2/seed_to_playlist/spotify:track:abc":  `{"mediaItems":[{"uri":"spotify:playlist:37i9radio"}],"total":1}`,
		"/inspiredby-mix/v2/seed_to_playlist/spotify:track:none": `{"mediaItems":[],"total":0}`,
		"/context-resolve/v1/spotify:playlist:37i9radio": `{"metadata":{"context_description":"Bilmem Radio","image_url":"https://img/x","format_list_type":"inspiredby-mix"},
			"pages":[{"tracks":[{"uri":"spotify:track:1","uid":"a"},{"uri":"spotify:track:2"}]}]}`,
	})
	ctx := context.Background()
	uri, err := Radio(ctx, "tok", "spotify:track:abc")
	if err != nil || uri != "spotify:playlist:37i9radio" {
		t.Fatalf("radio = %q %v", uri, err)
	}
	if _, err := Radio(ctx, "tok", "spotify:track:none"); err != ErrNoRadio {
		t.Fatalf("empty radio err = %v", err)
	}
	info, err := Context(ctx, "tok", uri)
	if err != nil || info.Name != "Bilmem Radio" || info.Image != "https://img/x" || len(info.Tracks) != 2 {
		t.Fatalf("context = %+v %v", info, err)
	}
	if _, err := Radio(ctx, "bad", "spotify:track:abc"); err != ErrUnauthorized {
		t.Fatalf("bad token err = %v", err)
	}
}
