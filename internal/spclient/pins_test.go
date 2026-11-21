package spclient

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPinsParsesPagesAndSkipsRemoved(t *testing.T) {
	item := func(uri string, at uint64, removed bool) []byte {
		var b []byte
		b = pbString(b, 1, uri)
		b = pbVarint(b, 2, at)
		if removed {
			b = pbVarint(b, 3, 1)
		}
		return b
	}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.URL.Path != "/collection/v2/paging" || r.Header.Get("Content-Type") != "application/vnd.collection-v2.spotify.proto" ||
			!bytes.Contains(body, []byte("ylpin")) || !bytes.Contains(body, []byte("me")) {
			http.Error(w, "bad", 400)
			return
		}
		calls++
		var out []byte
		if calls == 1 {
			out = pbBytes(out, 1, item("spotify:playlist:a", 100, false))
			out = pbBytes(out, 1, item("spotify:playlist:gone", 300, true))
			out = pbString(out, 2, "next")
		} else {
			if !bytes.Contains(body, []byte("next")) {
				http.Error(w, "no page token", 400)
				return
			}
			out = pbBytes(out, 1, item("spotify:album:b", 200, false))
		}
		w.Write(out)
	}))
	defer srv.Close()
	old := base
	base = srv.URL
	defer func() { base = old }()
	pins, err := Pins(t.Context(), "tok", "me")
	if err != nil {
		t.Fatal(err)
	}
	if len(pins) != 2 || pins[0].URI != "spotify:album:b" || pins[1].URI != "spotify:playlist:a" {
		t.Fatalf("pins %+v", pins)
	}
}

func TestSetPrivateSessionBody(t *testing.T) {
	var got []byte
	var method, ctype string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = io.ReadAll(r.Body)
		method, ctype = r.Method, r.Header.Get("Content-Type")
		if r.URL.Path != "/clientsettings/api/v1/" {
			http.Error(w, "path", 404)
		}
	}))
	defer srv.Close()
	old := base
	base = srv.URL
	defer func() { base = old }()
	if err := SetPrivateSession(t.Context(), "tok", true); err != nil {
		t.Fatal(err)
	}
	// Masaüstü uygulamasının gönderdiğiyle bayt bayt aynı.
	want := append([]byte{0x0a, 24}, "private-session-duration"...)
	want = append(want, 0x12, 7, 0x22, 5)
	want = append(want, "21600"...)
	if !bytes.Equal(got, want) || method != "PUT" || ctype != "application/x-protobuf" {
		t.Fatalf("%s %s %x\nwant %x", method, ctype, got, want)
	}
	if err := SetPrivateSession(t.Context(), "tok", false); err != nil || !bytes.HasSuffix(got, []byte{0x22, 1, '0'}) {
		t.Fatalf("off body %x %v", got, err)
	}
}
