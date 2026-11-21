package ui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/spclient"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// Özel oturum (✱): Ayarlar → Özel oturum açar ya da kapatır. Spotify ayarı
// geri okutmadığı için bitiş zamanı %LOCALAPPDATA%\sotto\private.json'da
// tutulur; Spotify da 6 saat sonra kendisi kapatır. Açıkken başlıkta işaret.

type privateMsg struct {
	on    bool
	login string
	err   error
}

func privateFile(dir string) string { return filepath.Join(dir, "private.json") }

func loadPrivate(dir string) time.Time {
	var v struct {
		Until time.Time `json:"until"`
	}
	if b, err := os.ReadFile(privateFile(dir)); err == nil {
		_ = json.Unmarshal(b, &v)
	}
	return v.Until
}

func savePrivate(dir string, until time.Time) {
	if dir == "" {
		return
	}
	_ = os.MkdirAll(dir, 0o700)
	b, _ := json.Marshal(map[string]time.Time{"until": until})
	_ = os.WriteFile(privateFile(dir), b, 0o600)
}

// privateOn: özel oturum şu an açık (sotto'nun bildiğine göre).
func (m Model) privateOn() bool { return m.now.Before(m.rt.privUntil) }

func (m Model) togglePrivate() (Model, tea.Cmd) {
	if !m.cfg.Extras.Enabled {
		return m.say(T("priv.extras")), nil
	}
	on := !m.privateOn()
	br := m.rt.bridge
	return m.note(T("priv.sending")), func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		login, err := withBridge(ctx, br, func(tok string) error {
			return spclient.SetPrivateSession(ctx, tok, on)
		})
		return privateMsg{on: on, login: login, err: err}
	}
}

func (m Model) updatePrivate(msg privateMsg) (Model, tea.Cmd) {
	switch {
	case msg.login != "":
		return m.say(T("smart.login")), nil
	case msg.err != nil:
		return m.say(msg.err.Error()), nil
	}
	m.rt.privUntil = time.Time{}
	if msg.on {
		m.rt.privUntil = time.Now().Add(spclient.PrivateSessionLength * time.Second)
		m = m.note(F("priv.on", m.rt.privUntil.Format("15:04")))
	} else {
		m = m.note(T("priv.off"))
	}
	savePrivate(m.cfg.LocalDir, m.rt.privUntil)
	return m, nil
}

// privateMark: başlıkta, açıkken.
func (m Model) privateMark() string {
	if !m.privateOn() {
		return ""
	}
	return theme.GoldText.Render("◐ "+T("priv.mark")) + theme.Faint.Render("  ·  ")
}
