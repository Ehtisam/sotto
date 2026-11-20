package ui

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/bridge"
	"github.com/Ehtisam/sotto/internal/soloist"
	"github.com/Ehtisam/sotto/internal/spclient"
)

// Radyo (✱): bir şarkının ya da sanatçının radyosu. Spotify uygulamasındaki
// "radyosuna git" gibi: Spotify o an bir liste üretir, sotto onu çalar.
// Resmî Web API bu listeyi ne üretir ne okur; iç yol ve bridge jetonu gerekir.

type radioMsg struct {
	uri, name string
	login     string // ekstralar için giriş gerekiyor
	err       error
}

// withBridge, köprüden jeton alıp f'yi çalıştırır. Jeton reddedilirse bir kez
// yenisini dener. Giriş gerekiyorsa adresini döner.
func withBridge(ctx context.Context, br *bridge.Bridge, f func(tok string) error) (login string, err error) {
	for try := 0; try < 2; try++ {
		tok, err := br.Token(ctx)
		var need *bridge.LoginNeeded
		if errors.As(err, &need) {
			return need.URL, nil
		}
		if err != nil {
			return "", err
		}
		err = f(tok)
		if errors.Is(err, spclient.ErrUnauthorized) {
			br.Invalidate()
			continue
		}
		return "", err
	}
	return "", spclient.ErrUnauthorized
}

// startRadio, seed'in (şarkı ya da sanatçı) radyosunu üretir; çalma radioMsg gelince başlar.
func (m Model) startRadio(seed string) tea.Cmd {
	br := m.rt.bridge
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var msg radioMsg
		login, err := withBridge(ctx, br, func(tok string) error {
			uri, err := spclient.Radio(ctx, tok, seed)
			if err != nil {
				return err
			}
			msg.uri = uri
			if info, err := spclient.Context(ctx, tok, uri); err == nil {
				msg.name = info.Name
			}
			return nil
		})
		msg.login, msg.err = login, err
		return msg
	}
}

func (m Model) updateRadio(msg radioMsg) (Model, tea.Cmd) {
	switch {
	case msg.login != "":
		return m.say(T("act.radio.login")), nil
	case msg.err != nil:
		return m.say(msg.err.Error()), nil
	}
	name := msg.name
	if name == "" {
		name = T("act.radio.name")
	}
	m.rt.contextNames[msg.uri] = name
	return m.note(F("act.radio.on", name)), m.playContext(msg.uri, "")
}

// nameContext, adı gelmemiş bağlama (Spotify'ın kendi listeleri, radyo)
// sotto'nun bildiği adı koyar.
func (m Model) nameContext(e *soloist.Entity) *soloist.Entity {
	if e == nil || e.Name() != "" {
		return e
	}
	if name, ok := m.rt.contextNames[e.URI]; ok {
		c := *e
		c.Decorations.Identity.Name = name
		return &c
	}
	return e
}
