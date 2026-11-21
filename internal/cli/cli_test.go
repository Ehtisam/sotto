package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/Ehtisam/sotto/internal/config"
	"github.com/Ehtisam/sotto/internal/history"
	"github.com/Ehtisam/sotto/internal/spotify"
)

func TestParseSeek(t *testing.T) {
	for in, want := range map[string]int64{"90": 90000, "1:30": 90000, "+10": 70000, "-10": 50000, "-100": 0, "0:05.5": 5500} {
		got, err := ParseSeek(in, 60000)
		if err != nil || got != want {
			t.Errorf("%s: %d %v, want %d", in, got, err, want)
		}
	}
	if _, err := ParseSeek("abc", 0); err == nil {
		t.Error("abc parsed")
	}
}

func TestParseVolumeRepeatOnOff(t *testing.T) {
	if v, _ := ParseVolume("+5", 98); v != 100 {
		t.Errorf("vol = %d", v)
	}
	if v, _ := ParseVolume("-10", 40); v != 30 {
		t.Errorf("vol = %d", v)
	}
	if v, _ := ParseVolume("55", 0); v != 55 {
		t.Errorf("vol = %d", v)
	}
	if r, _ := NextRepeat("", "track"); r != "off" {
		t.Errorf("repeat = %s", r)
	}
	if r, _ := NextRepeat("", ""); r != "context" {
		t.Errorf("repeat = %s", r)
	}
	if _, err := NextRepeat("sometimes", "off"); err == nil {
		t.Error("bad repeat accepted")
	}
	if on, _ := ParseOnOff("", true); on {
		t.Error("toggle")
	}
}

func TestToURI(t *testing.T) {
	cases := map[string]string{
		"https://open.spotify.com/intl-tr/track/4uLU6hMCjMI75M1A2tKUQC?si=abc": "spotify:track:4uLU6hMCjMI75M1A2tKUQC",
		"open.spotify.com/album/1ATL5GLyefJaxhQzSPVrLX":                        "spotify:album:1ATL5GLyefJaxhQzSPVrLX",
		"spotify:playlist:37i9dQZF1DXcBWIGoYBM5M":                              "spotify:playlist:37i9dQZF1DXcBWIGoYBM5M",
		"https://open.spotify.com/user/bob/playlist/abc":                       "spotify:playlist:abc",
	}
	for in, want := range cases {
		if got, err := ToURI(in); err != nil || got != want {
			t.Errorf("%s: %s %v", in, got, err)
		}
	}
	for _, bad := range []string{"Daft Punk - One More Time", "https://example.com/track/x"} {
		if _, err := ToURI(bad); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
}

func TestFlags(t *testing.T) {
	f := parse([]string{"new", "Road", "trip", "--public", "--desc", "for the car", "--limit=3"})
	if f.rest(1) != "Road trip" || !f.has("public") || f.vals["desc"] != "for the car" || f.int("limit", 0) != 3 {
		t.Fatalf("%+v", f)
	}
}

func TestFindPlaylistAndDevice(t *testing.T) {
	lists := []spotify.Playlist{{ID: "a", URI: "spotify:playlist:a", Name: "KaBoom.2"}, {ID: "b", URI: "spotify:playlist:b", Name: "Kahve"}, {ID: "c", URI: "spotify:playlist:c", Name: "kaboom.2"}}
	if _, err := findPlaylist(lists, "ka"); err == nil || !strings.Contains(err.Error(), "more than one") {
		t.Fatalf("ambiguous: %v", err)
	}
	if p, err := findPlaylist(lists, "Kah"); err != nil || p.ID != "b" {
		t.Fatalf("prefix: %v", err)
	}
	if p, err := findPlaylist(lists, "https://open.spotify.com/playlist/c"); err != nil || p.ID != "c" {
		t.Fatalf("link: %v", err)
	}
	ds := []spotify.Device{{Name: "sotto"}, {Name: "Pixel 8"}, {Name: "sotto helper"}}
	if d, err := FindDevice(ds, "SOTTO"); err != nil || d.Name != "sotto" {
		t.Fatalf("exact: %v", err)
	}
	if d, err := FindDevice(ds, "pix"); err != nil || d.Name != "Pixel 8" {
		t.Fatalf("prefix: %v", err)
	}
}

func TestStatusText(t *testing.T) {
	s := Status{Playing: true, Title: "Song", Artist: "Band", Album: "LP", URI: "spotify:track:x", PositionMS: 61000, DurationMS: 185000, Volume: 40, Device: "sotto", Context: "KaBoom.2"}
	txt := s.Text()
	for _, want := range []string{"▶ Song — Band", "1:01 / 3:05", "vol 40%", "from KaBoom.2"} {
		if !strings.Contains(txt, want) {
			t.Errorf("%q lacks %q", txt, want)
		}
	}
	if (Status{}).Text() != "Nothing is playing." {
		t.Error("empty status")
	}
}

func TestMainHelpAndUnknown(t *testing.T) {
	var out, errb bytes.Buffer
	cfg := config.Config{LocalDir: t.TempDir()}
	if code := Main(cfg, []string{"help"}, &out, &errb, nil); code != 0 || !strings.Contains(out.String(), "sotto playlist import") {
		t.Fatalf("help: %d %s", code, out.String())
	}
	out.Reset()
	// Kurulum yokken açık bir hata.
	if code := Main(cfg, []string{"next", "--json"}, &out, &errb, nil); code != 1 || !strings.Contains(out.String(), `"ok": false`) || !strings.Contains(out.String(), "setup") {
		t.Fatalf("not set up: %d %s", code, out.String())
	}
	if IsCommand("bogus") || !IsCommand("next") || !IsCommand("--help") {
		t.Fatal("IsCommand")
	}
}

func TestMatches(t *testing.T) {
	if !Matches("Radiohead - Karma Police", "Karma Police", "Radiohead") {
		t.Error("exact")
	}
	if !Matches("daft punk one more time", "One More Time", "Daft Punk") {
		t.Error("no dash")
	}
	if Matches("zzqqxx nonexistent song qqzz 12345", "Bakkal Amca", "Kukuli") {
		t.Error("nonsense matched")
	}
	if !Matches("seksendort neyin var", "Neyin Var", "Seksendört") || !Matches("SEKSENDÖRT", "x", "Seksendört") {
		t.Error("diacritics")
	}
	if Matches("Radiohead - Creep", "Karma Police", "Radiohead") {
		t.Error("wrong song by the right artist")
	}
}

func TestFriendlyAndDisallowed(t *testing.T) {
	err := Friendly(&spotify.HTTPError{Status: 403, Message: "Player command failed: Cannot control device volume"}, "Pixel")
	if err == nil || !strings.Contains(err.Error(), "Pixel doesn't let other devices change its volume") {
		t.Fatalf("%v", err)
	}
	err = Friendly(&spotify.HTTPError{Status: 403, Message: "Player command failed: Restriction violated"}, "")
	if err == nil || !strings.Contains(err.Error(), "doesn't allow that") {
		t.Fatalf("%v", err)
	}
	no := false
	st := &spotify.PlayerState{}
	st.Device.SupportsVolume = &no
	st.Actions.Disallows = map[string]bool{"toggling_repeat_context": true}
	if !Disallowed(st, "volume", "") || !Disallowed(st, "repeat:context", "off") || !Disallowed(st, "repeat:off", "context") || Disallowed(st, "repeat:track", "off") || Disallowed(nil, "volume", "") {
		t.Fatal("Disallowed")
	}
}

func TestHistoryCommand(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{LocalDir: dir, ConfigDir: dir}
	now := time.Now()
	for i, title := range []string{"A", "A", "B"} {
		_ = history.Append(dir, history.Play{At: now.Add(-time.Duration(i+1) * time.Hour), URI: "spotify:track:" + title, Title: title, Artist: "X", MS: 200000})
	}
	var out, errb strings.Builder
	if code := Main(cfg, []string{"history"}, &out, &errb, strings.NewReader("")); code != 0 {
		t.Fatalf("code %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "3 plays") || !strings.Contains(out.String(), "1. A — X  (2)") {
		t.Fatalf("output:\n%s", out.String())
	}
	out.Reset()
	if code := Main(cfg, []string{"history", "all", "--json"}, &out, &errb, strings.NewReader("")); code != 0 || !strings.Contains(out.String(), `"plays": 3`) {
		t.Fatalf("json (%d):\n%s", code, out.String())
	}
	if code := Main(cfg, []string{"history", "decade"}, &out, &errb, strings.NewReader("")); code != 2 {
		t.Fatalf("bad range code %d", code)
	}
}
