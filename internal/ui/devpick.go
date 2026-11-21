package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// Hızlı cihaz seçici: d her sayfadan küçük bir cihaz listesi açar; enter
// müziği seçilen cihaza taşır (kapalı Soloist'i açar), esc kapatır. Liste
// Cihazlar sayfasınınkiyle aynı (deviceRows); açılınca tazelenir.

type devPicker struct {
	open   bool
	cursor int
	moved  bool // kullanıcı imleci oynattı; liste gelince çalan cihaza alınmasın
}

// pickActive, imleci müziğin çaldığı cihaza alır.
func (m *Model) pickActive() {
	for i, r := range m.deviceRows() {
		if r.active {
			m.pick.cursor = i
		}
	}
}

func (m Model) openDevPicker() (Model, tea.Cmd) {
	if m.spState != spReady {
		return m.say(T("flash.noaccount")), nil
	}
	m.pick = devPicker{open: true}
	m.pickActive() // liste tazelenince de (updateDevices)
	m.devs.loading = true
	return m, m.loadDevices()
}

func (m Model) devPickKey(k string) (Model, tea.Cmd) {
	rows := m.deviceRows()
	switch k {
	case "up", "k":
		m.pick.cursor, m.pick.moved = max(0, m.pick.cursor-1), true
	case "down", "j":
		m.pick.cursor, m.pick.moved = min(len(rows)-1, m.pick.cursor+1), true
	case "R":
		m.devs.loading = true
		return m, m.loadDevices()
	case "enter", "right":
		if len(rows) == 0 {
			return m, nil
		}
		r := rows[max(0, min(m.pick.cursor, len(rows)-1))]
		m.pick.open = false
		if r.soloist && r.off {
			return m.startSoloistAndUse()
		}
		if r.id == "" || r.active {
			return m, nil
		}
		return m.beginSwitch(r.name), m.transfer(r.id, r.name)
	case "esc", "left", "backspace", "d", "q":
		m.pick.open = false
	}
	return m, nil
}

func (m Model) devPickView(w int) []string {
	rows := []string{theme.Title.Render(T("pick.title")), theme.Dim.Render(T("pick.sub")), "", ""}
	if m.devs.err != nil {
		rows[1] = theme.Error.Render(truncate(m.devs.err.Error(), w))
	}
	list := m.deviceRows()
	rw := min(w-2, 56)
	for i, r := range list {
		mark := "  "
		if r.active {
			mark = theme.Playing.Render(theme.Icon.Note + " ")
		}
		kind := r.kind
		switch {
		case r.soloist && r.off && m.engine == engineStarting:
			kind = T("dev.kind.starting")
		case r.soloist && r.off:
			kind = T("dev.kind.off")
		case r.soloist:
			kind = T("dev.kind.light")
		}
		rows = append(rows, mark+row(r.name, kind, rw, i == m.pick.cursor))
	}
	if m.devs.loading && len(m.devs.items) == 0 {
		rows = append(rows, theme.Faint.Render("  "+T("lib.loading")))
	}
	return append(rows, "", theme.Faint.Render(T("pick.help")))
}
