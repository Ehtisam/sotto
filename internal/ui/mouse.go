package ui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// Fare (config [look] mouse): menüde bir sayfaya tıklamak oraya gider;
// listede bir satıra tıklamak onu seçer, seçili satıra yeniden tıklamak açar
// ya da çalar; ilerleme çubuğuna tıklamak oraya sarar; şeritteki düğme çalar
// ya da duraklatır, şarkının adı Şimdi çalıyor'a götürür. Tekerlek listeleri
// kaydırır; şeridin üstünde sesi değiştirir.

// layout, ekranın parçalarının satırları (render ile aynı hesap).
type layout struct {
	bodyTop, bodyH int
	stripY         int // şeridin ilk satırı (ikincisi ilerleme çubuğu)
	menu           bool
	contentX       int
}

func (m Model) layout() layout {
	bodyH := max(m.height-7, 1)
	l := layout{bodyTop: 2, bodyH: bodyH, stripY: 2 + bodyH + 1}
	if m.stripTop() {
		l.stripY, l.bodyTop = 2, 5
	}
	cw := m.width - margin - menuWidth - gutter - 1
	l.menu = cw >= 24
	l.contentX = margin
	if l.menu {
		l.contentX = margin + menuWidth + gutter
	}
	return l
}

// barZone: ilerleme çubuğunun yeri (strip çizerken yazar).
type barZone struct{ x, w int }

func (m Model) mouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if !m.cfg.Look.Mouse || m.wiz.on || m.form.on {
		return m, nil
	}
	ms := msg.Mouse()
	if m.miniMode() {
		if _, ok := msg.(tea.MouseWheelMsg); ok {
			return m.wheelVolume(ms.Button)
		}
		return m, nil
	}
	l := m.layout()
	inStrip := ms.Y == l.stripY || ms.Y == l.stripY+1
	inBody := ms.Y >= l.bodyTop && ms.Y < l.bodyTop+l.bodyH
	inMenu := inBody && l.menu && ms.X >= margin && ms.X < margin+menuWidth

	switch msg.(type) {
	case tea.MouseWheelMsg:
		switch {
		case inStrip:
			return m.wheelVolume(ms.Button)
		case inMenu && !m.overlay():
			m.focus = focusMenu
			return m.keyCanon(tea.KeyPressMsg{}, wheelKey(ms.Button))
		case inBody:
			if m.page.hasContent() || m.extraOn() || m.overlay() {
				if !m.overlay() {
					m.focus = focusContent
				}
				return m.keyCanon(tea.KeyPressMsg{}, wheelKey(ms.Button))
			}
		}
		return m, nil
	case tea.MouseClickMsg:
		if ms.Button != tea.MouseLeft {
			return m, nil
		}
	default:
		return m, nil
	}

	switch {
	case ms.Y == l.stripY+1: // ilerleme çubuğu
		z := m.rt.bar
		if z.w > 0 && ms.X >= z.x && ms.X < z.x+z.w && m.state.Item != nil {
			d := m.state.Item.Duration()
			frac := float64(ms.X-z.x) / float64(max(z.w-1, 1))
			return m, m.seekTo(time.Duration(frac * float64(d)))
		}
	case ms.Y == l.stripY:
		if ms.X < margin+4 { // düğme
			return m, m.playPause()
		}
		m.page, m.focus, m.help, m.extra = pageNow, focusMenu, false, extraState{rng: m.extra.rng}
		return m, nil
	case ms.Y == 0 && ms.X > m.width/2: // başlıkta cihaz
		return m.keyCanon(tea.KeyPressMsg{Text: "6", Code: '6'}, "6")
	case inMenu && !m.overlay():
		i := ms.Y - l.bodyTop - 2 // "İçindekiler" ve boş satır
		if i >= 0 && i < len(pages) {
			d := string(rune('1' + i))
			return m.keyCanon(tea.KeyPressMsg{Text: d, Code: rune(d[0])}, d)
		}
	case inBody && ms.X >= l.contentX:
		return m.clickRow(ms.Y - l.bodyTop)
	}
	return m, nil
}

func wheelKey(b tea.MouseButton) string {
	if b == tea.MouseWheelUp {
		return "k"
	}
	return "j"
}

func (m Model) wheelVolume(b tea.MouseButton) (tea.Model, tea.Cmd) {
	switch b {
	case tea.MouseWheelUp:
		return m, m.setVolume(m.state.Volume + 5)
	case tea.MouseWheelDown:
		return m, m.setVolume(m.state.Volume - 5)
	}
	return m, nil
}

// clickRow, içerikte target. satıra tıklama: seçili satır oraya gelene kadar
// imleci yürütür; zaten seçiliyse açar (enter).
func (m Model) clickRow(target int) (tea.Model, tea.Cmd) {
	if !m.page.hasContent() && !m.extraOn() && !m.overlay() {
		return m, nil
	}
	if !m.overlay() {
		m.focus = focusContent
	}
	sel := m.selectedContentRow()
	if sel < 0 {
		return m, nil
	}
	if sel == target {
		return m.keyCanon(tea.KeyPressMsg{Code: tea.KeyEnter}, "enter")
	}
	var cmds []tea.Cmd
	var out tea.Model = m
	for step := 0; step < 300 && sel != target; step++ {
		k := "j"
		if target < sel {
			k = "k"
		}
		var cmd tea.Cmd
		out, cmd = out.(Model).keyCanon(tea.KeyPressMsg{}, k)
		cmds = append(cmds, cmd)
		next := out.(Model).selectedContentRow()
		if next == sel || next < 0 { // listenin ucu ya da seçilemeyen satır
			break
		}
		if (k == "j" && next > target) || (k == "k" && next < target) {
			break // başlık atlandı: tıklanan satır seçilemez
		}
		sel = next
	}
	return out, tea.Batch(cmds...)
}

// selectedContentRow, içerikte seçili satırın sırası (-1: yok). İçeriği
// çizip seçili satır renginin geçtiği satırı arar; resim yerleşimini bozmaz.
func (m Model) selectedContentRow() int {
	w, h := m.contentSize()
	saved := m.rt.gfx.slots
	s := m.content(w, h)
	m.rt.gfx.slots = saved
	mark := selectedMark()
	if mark == "" {
		return -1
	}
	for i, line := range strings.Split(s, "\n") {
		if strings.Contains(line, mark) {
			return i
		}
	}
	return -1
}

// selectedMark, seçili satır stilinin başındaki renk kodu.
func selectedMark() string {
	r := theme.SelectedRow.Render("\x01")
	i := strings.IndexByte(r, '\x01')
	if i <= 0 {
		return ""
	}
	return r[:i]
}
