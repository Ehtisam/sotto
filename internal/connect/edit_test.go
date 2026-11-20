package connect

import (
	"encoding/json"
	"strings"
	"testing"
)

func rows(spec string) []Track {
	var out []Track
	for _, s := range strings.Fields(spec) {
		t := Track{URI: "spotify:track:" + s, UID: "u" + s, Provider: "context"}
		switch {
		case s == "|":
			t = Track{URI: "spotify:delimiter", UID: "delimiter0"}
		case strings.HasPrefix(s, "q"):
			t.Provider, t.Metadata = "queue", map[string]string{"is_queued": "true"}
		}
		out = append(out, t)
	}
	return out
}

func names(ts []Track) string {
	var out []string
	for _, t := range ts {
		if t.Delimiter() {
			out = append(out, "|")
		} else {
			out = append(out, strings.TrimPrefix(t.URI, "spotify:track:"))
		}
	}
	return strings.Join(out, " ")
}

func TestLocate(t *testing.T) {
	next := rows("q1 q2 | c1 c2 c1")
	cases := []struct {
		pos      int
		uri, uid string
		want     int
	}{
		{0, "spotify:track:q1", "uq1", 0},
		{2, "spotify:track:c1", "", 3},                 // uid yok: yer
		{4, "spotify:track:c1", "", 5},                 // aynı şarkı iki kez: yer belirler
		{1, "spotify:track:c2", "", 4},                 // yer tutmuyor: adres
		{0, "spotify:track:c2", "uc2", 4},              // uid her şeyden önce
		{2, "spotify:track:c1", "spotify:track:c1", 3}, // Soloist uid yerine adres verebilir
		{0, "spotify:track:zz", "", -1},
	}
	for _, c := range cases {
		if got := Locate(next, c.pos, c.uri, c.uid); got != c.want {
			t.Errorf("Locate(%d, %s, %q) = %d, want %d", c.pos, c.uri, c.uid, got, c.want)
		}
	}
}

func TestRemoveAndMove(t *testing.T) {
	next := rows("q1 q2 | c1 c2 c3")
	if got := names(Remove(next, 3)); got != "q1 q2 | c2 c3" {
		t.Fatalf("remove = %q", got)
	}
	if names(next) != "q1 q2 | c1 c2 c3" {
		t.Fatal("remove changed its input")
	}
	cases := []struct {
		i, d int
		want string
		ok   bool
	}{
		{0, 1, "q2 q1 | c1 c2 c3", true},
		{4, -1, "q1 q2 | c2 c1 c3", true},
		{1, 1, "q1 q2 | c1 c2 c3", false},  // sıradan listeye geçmez
		{3, -1, "q1 q2 | c1 c2 c3", false}, // listeden sıraya geçmez
		{5, 1, "q1 q2 | c1 c2 c3", false},
		{0, -1, "q1 q2 | c1 c2 c3", false},
	}
	for _, c := range cases {
		got, ok := Move(next, c.i, c.d)
		if names(got) != c.want || ok != c.ok {
			t.Errorf("Move(%d, %d) = %q %v, want %q %v", c.i, c.d, names(got), ok, c.want, c.ok)
		}
	}
}

// Kümeden gelen satır, Spotify'ın gönderdiği haliyle geri gider.
func TestTrackRoundTrip(t *testing.T) {
	in := `{"uri":"spotify:track:x","uid":"abc","metadata":{"is_queued":"true","title":"X"},"provider":"queue","removed":[],"restrictions":{}}`
	var tr Track
	if err := json.Unmarshal([]byte(in), &tr); err != nil {
		t.Fatal(err)
	}
	if !tr.Queued() || tr.UID != "abc" {
		t.Fatalf("parsed %+v", tr)
	}
	out, _ := json.Marshal([]Track{tr})
	if string(out) != "["+in+"]" {
		t.Fatalf("round trip = %s", out)
	}
}
