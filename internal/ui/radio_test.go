package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/Ehtisam/sotto/internal/soloist"
)

func TestRadioInTheActionMenu(t *testing.T) {
	m := libraryWithOpenList(t)
	m.actions = actionMenu{open: true, target: trackItem(m.lib.open.tracks[0]), saved: 0}
	if slicesHas(ids(m.menuItems()), actRadio) {
		t.Fatal("no radio without the extras")
	}
	m.cfg.Extras.Enabled = true
	if !slicesHas(ids(m.menuItems()), actRadio) {
		t.Fatal("songs should offer their radio")
	}
	m.actions.target = searchItem{kind: "artist", name: "Art", uri: "spotify:artist:ar"}
	if !slicesHas(ids(m.menuItems()), actRadio) {
		t.Fatal("artists should offer their radio")
	}
	m.actions.target = searchItem{kind: "album", uri: "spotify:album:al"}
	if slicesHas(ids(m.menuItems()), actRadio) {
		t.Fatal("albums have no radio here")
	}
}

func TestRadioPlaysAndShowsItsName(t *testing.T) {
	m := homeModel(t)
	m.spState = spReady
	mm, cmd := m.updateRadio(radioMsg{uri: "spotify:playlist:37i9radio", name: "Bilmem Radio"})
	if cmd == nil || !strings.Contains(mm.flash, "Bilmem Radio") {
		t.Fatalf("flash %q", mm.flash)
	}
	// Soloist bağlamı adsız gönderir; sotto bildiği adı koyar (müzik Soloist'te).
	mm.engine, mm.rt.client = engineReady, &soloist.Client{}
	mm.auth = soloist.AuthState{LoggedIn: true, IsActive: true}
	ctx := &soloist.Entity{URI: "spotify:playlist:37i9radio", EntityType: "playlist"}
	mm.apply(soloist.ContextChanged{Context: ctx})
	if mm.state.Context.Name() != "Bilmem Radio" {
		t.Fatalf("context name = %q", mm.state.Context.Name())
	}
	// Kumanda modunda da (Web API) ad bulunur.
	if got := mm.contextName("spotify:playlist:37i9radio", "playlist"); got != "Bilmem Radio" {
		t.Fatalf("remote context name = %q", got)
	}
	// Başka bir listenin adına dokunulmaz.
	other := &soloist.Entity{URI: "spotify:playlist:x"}
	other.Decorations.Identity.Name = "Mine"
	if mm.nameContext(other).Name() != "Mine" {
		t.Fatal("named contexts stay as they are")
	}
}

func TestRadioErrors(t *testing.T) {
	m := homeModel(t)
	if mm, _ := m.updateRadio(radioMsg{login: "https://login"}); !strings.Contains(mm.flash, "sotto setup") {
		t.Fatalf("login flash = %q", mm.flash)
	}
	if mm, cmd := m.updateRadio(radioMsg{err: errors.New("boom")}); cmd != nil || mm.flash != "boom" {
		t.Fatalf("error flash = %q", mm.flash)
	}
}

func slicesHas(xs []actID, x actID) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
