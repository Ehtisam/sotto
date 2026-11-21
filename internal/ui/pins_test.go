package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// Sabitlenenler Kitaplık'ta üstte, en son sabitlenen önce, işaretli; ada göre
// sıralamada da üstte kalır; ekstralar kapalıysa yok sayılır.
func TestPinnedOnTop(t *testing.T) {
	_, m := playingModel(t)
	m.cfg.Extras.Enabled = true
	m.page, m.focus = pageLibrary, focusContent
	m.lib.open = nil
	m.lib.entries = []libEntry{
		{kind: kindLiked, key: likedKey, name: "Liked Songs"},
		{kind: kindPlaylist, key: "a", uri: "spotify:playlist:a", name: "Alpha"},
		{kind: kindPlaylist, key: "b", uri: "spotify:playlist:b", name: "Beta"},
		{kind: kindPlaylist, key: "c", uri: "spotify:playlist:c", name: "Gamma"},
	}
	m.lib.pins = []string{"spotify:playlist:c", "spotify:collection"}
	names := func() string {
		var s []string
		for _, e := range m.libVisible() {
			mark := ""
			if e.pinned {
				mark = "*"
			}
			s = append(s, mark+e.name)
		}
		return strings.Join(s, ",")
	}
	if got := names(); got != "*Gamma,*Liked Songs,Alpha,Beta" {
		t.Fatalf("order %s", got)
	}
	m.lib.sortBy, m.lib.desc = sortName, true
	if got := names(); !strings.HasPrefix(got, "*Gamma,*Liked Songs,") {
		t.Fatalf("pins must stay on top when sorted: %s", got)
	}
	m.lib.sortBy, m.lib.desc = sortCustom, false
	if view := ansi.Strip(strings.Join(m.libraryView(100, 30), "\n")); !strings.Contains(view, "✦ Gamma") {
		t.Fatalf("pin mark missing:\n%s", view)
	}
	m.cfg.Extras.Enabled = false
	if got := names(); got != "Liked Songs,Alpha,Beta,Gamma" {
		t.Fatalf("extras off: %s", got)
	}
}
