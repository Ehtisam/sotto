package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/Ehtisam/sotto/internal/setup"
	"github.com/Ehtisam/sotto/internal/soloist"
	"github.com/charmbracelet/x/ansi"
)

// Soloist yokken, anahtarsızken, süresi dolmuşken ya da bozukken sotto bir
// kumanda olarak çalışmaya devam ettiğini ve ne yapılacağını söyler.
func TestEngineFailureFallsBackToRemote(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{soloist.ErrNotInstalled, "sotto setup"},
		{soloist.ErrNoAPIKey, "sotto setup"},
		{soloist.ErrExpired, "90 days"},
		{errors.New("exit status 3"), "exit status 3"},
	}
	for _, c := range cases {
		_, m := newFakeWeb(t)
		m.engine, m.engineErr = engineFailed, c.err
		s := ansi.Strip(strings.Join(m.engineMessage(), "\n"))
		if !strings.Contains(s, c.want) || !strings.Contains(s, "still controls Spotify") {
			t.Errorf("%v:\n%s", c.err, s)
		}
	}
	// Süresi dolmuş ve yeni sürüm iniyor.
	_, m := newFakeWeb(t)
	m.engine, m.engineErr = engineFailed, soloist.ErrExpired
	m.set.updating = &setup.Progress{}
	if s := ansi.Strip(strings.Join(m.engineMessage(), "\n")); !strings.Contains(s, "downloading") {
		t.Errorf("updating:\n%s", s)
	}
}
