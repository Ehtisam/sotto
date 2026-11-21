package spclient

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Gerçek hesapta 21.11.2026'da çalışan gövdeyle bayt bayt aynı olmalı.
func TestHiddenWriteBody(t *testing.T) {
	now := time.Unix(1763750000, 0)
	var item []byte
	item = pbString(item, 1, "spotify:track:x")
	item = pbVarint(item, 2, 1763750000)
	var want []byte
	want = pbString(want, 1, "me")
	want = pbString(want, 2, "ban")
	want = pbBytes(want, 3, item)
	if got := hiddenWriteBody("me", "spotify:track:x", true, now); !bytes.Equal(got, want) {
		t.Fatalf("hide body\n got %x\nwant %x", got, want)
	}
	item = pbVarint(item, 3, 1)
	want = nil
	want = pbString(want, 1, "me")
	want = pbString(want, 2, "ban")
	want = pbBytes(want, 3, item)
	if got := hiddenWriteBody("me", "spotify:track:x", false, now); !bytes.Equal(got, want) {
		t.Fatalf("unhide body\n got %x\nwant %x", got, want)
	}
}

func TestHiddenReadAndWrite(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		paths = append(paths, r.URL.Path)
		if !bytes.Contains(body, []byte("ban")) {
			http.Error(w, "bad set", 400)
			return
		}
		if r.URL.Path == "/collection/v2/paging" {
			var it []byte
			it = pbString(it, 1, "spotify:track:h")
			it = pbVarint(it, 2, 5)
			w.Write(pbBytes(nil, 1, it))
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	defer SetBaseForTest(srv.URL)()
	got, err := Hidden(context.Background(), "tok", "me")
	if err != nil || len(got) != 1 || got[0] != "spotify:track:h" {
		t.Fatalf("hidden = %v, %v", got, err)
	}
	if err := SetHidden(context.Background(), "tok", "me", "spotify:track:h", false); err != nil {
		t.Fatal(err)
	}
	if paths[len(paths)-1] != "/collection/v2/write" {
		t.Fatalf("paths %v", paths)
	}
}
