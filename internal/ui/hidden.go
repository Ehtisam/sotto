package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/spclient"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// Gizlenen şarkılar (✱): Spotify'daki "Şarkıyı gizle" (collection v2 "ban").
// Eylem menüsünde h gizler ya da gizlemeyi kaldırır; gizli şarkılar listelerde
// ve arama sonuçlarında soluk, ⊘ işaretli görünür (Spotify çalarken atlar).
// Küme kitaplığın sırası (rootlist) okunurken okunur.

type hiddenMsg struct {
	uri, name string
	hide      bool
	login     string
	err       error
}

// isHidden: şarkı gizli mi (ekstralar kapalıysa hiçbir şey gizli sayılmaz).
func (m Model) isHidden(uri string) bool {
	return m.cfg.Extras.Enabled && m.rt.hidden[uri]
}

// setHidden, şarkıyı hemen gizli/açık gösterir ve Spotify'a yazar; olmazsa geri alır.
func (m Model) setHidden(uri, name string, hide bool) (Model, tea.Cmd) {
	if !m.cfg.Extras.Enabled {
		return m.say(T("hide.extras")), nil
	}
	if m.rt.hidden == nil {
		m.rt.hidden = map[string]bool{}
	}
	if hide {
		m.rt.hidden[uri] = true
	} else {
		delete(m.rt.hidden, uri)
	}
	br, sp, user := m.rt.bridge, m.sp, m.me.ID
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		msg := hiddenMsg{uri: uri, name: name, hide: hide}
		if user == "" {
			me, err := sp.Me(ctx)
			if err != nil {
				msg.err = err
				return msg
			}
			user = me.ID
		}
		msg.login, msg.err = withBridge(ctx, br, func(tok string) error {
			return spclient.SetHidden(ctx, tok, user, uri, hide)
		})
		return msg
	}
}

func (m Model) updateHidden(msg hiddenMsg) (Model, tea.Cmd) {
	if msg.login != "" || msg.err != nil { // geri al
		if msg.hide {
			delete(m.rt.hidden, msg.uri)
		} else {
			m.rt.hidden[msg.uri] = true
		}
		if msg.login != "" {
			return m.say(T("queue.edit.login")), nil
		}
		return m.say(msg.err.Error()), nil
	}
	if msg.hide {
		return m.note(F("hide.done", msg.name)), nil
	}
	return m.note(F("hide.undone", msg.name)), nil
}

// hiddenRow, gizli şarkının satırı: soluk ve ⊘ işaretli (seçiliyken normal seçim).
func hiddenRow(left, right string, w int, selected bool) string {
	left = "⊘ " + left
	if selected {
		return row(left, right, w, true)
	}
	l, dots, r := leaderParts(left, right, w)
	if r == "" {
		return theme.Faint.Render(l)
	}
	return theme.Faint.Render(l + " " + dots + " " + r)
}
