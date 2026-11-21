package ui

import (
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/keys"
)

// Tuş haritası (internal/keys, keymap.toml): basılan tuş önce haritadan
// geçer, haritadaki işin arayüzün beklediği tuşuna (Canon) çevrilir; sayfalar
// hep o tuşu görür. Yazı yazılan yerlerde (arama kutusu, form, kurulum)
// tuşlar çevrilmez. "g l" gibi diziler ve vim tarzı sayılar (5j) da burada.

// countable: başına sayı alabilen işler (5j, 3K…).
var countable = map[string]bool{
	"up": true, "down": true, "page_up": true, "page_down": true,
	"move_up": true, "move_down": true, "seek_back": true, "seek_forward": true,
	"volume_up": true, "volume_down": true, "next": true, "previous": true,
}

// navActions: menüler (eylem, sıralama) ve ? listesi açıkken de çevrilen
// işler; öteki tuşlar o menülerin kendi harfleridir.
var navActions = map[string]bool{
	"up": true, "down": true, "page_up": true, "page_down": true, "top": true,
	"bottom": true, "open": true, "back": true, "help": true, "quit": true,
}

// maxCount: bir sayıyla en çok bu kadar tekrar.
const maxCount = 500

type seqTimeoutMsg struct{ at time.Time }
type countTimeoutMsg struct{ at time.Time }

// typing: harfler yazıya gidiyor (tuşlar çevrilmez).
func (m Model) typing() bool {
	if m.wiz.on || m.form.on {
		return true
	}
	if m.focus == focusContent && m.page == pageSearch && m.search.typing && m.search.open == nil && !m.actions.open && !m.sortm.open && !m.help {
		return true
	}
	mm := m
	if f := mm.shownFinder(); f != nil && f.typing && !m.actions.open && !m.sortm.open && !m.help {
		return true
	}
	return false
}

// overlay: üstte bir menü ya da liste açık.
func (m Model) overlay() bool { return m.actions.open || m.sortm.open || m.help }

func (m Model) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	raw := msg.String()
	if raw == "ctrl+c" {
		return m, m.quit()
	}
	if m.typing() {
		m.seq, m.count = "", ""
		return m.keyCanon(msg, raw)
	}

	// Eylem ve sıralama menülerinde harfler menünün kendi kısayollarıdır
	// (eylem menüsünde g radyo): dizi başlamaz, yalnız gezinme çevrilir.
	if m.actions.open || m.sortm.open {
		m.seq, m.count = "", ""
		action, bound := m.km.Lookup(raw)
		return m.keyAction(msg, raw, action, bound)
	}

	// Dizi: önceki tuş bir dizinin başıysa bu tuşla birleşir.
	full := raw
	if m.seq != "" {
		full = m.seq + " " + raw
		m.seq = ""
		if raw == "esc" {
			return m, nil // diziden vazgeç
		}
	}
	if m.km.IsPrefix(full) {
		m.seq, m.seqAt = full, m.now
		at := m.now
		return m, tea.Tick(m.km.Timeout, func(time.Time) tea.Msg { return seqTimeoutMsg{at} })
	}
	action, bound := m.km.Lookup(full)
	if !bound && full != raw {
		return m, nil // tamamlanmayan dizi
	}

	// Sayı: 5j. Yalnızca içerikteyken (menüde rakamlar hemen sayfaya gider).
	if m.km.Counts && !m.overlay() && m.focus == focusContent && len(raw) == 1 && raw[0] >= '0' && raw[0] <= '9' && (raw != "0" || m.count != "") {
		m.count += raw
		m.countAt = m.now
		at := m.now
		return m, tea.Tick(m.km.Timeout*6/10, func(time.Time) tea.Msg { return countTimeoutMsg{at} })
	}
	n := 1
	if m.count != "" {
		c := m.count
		m.count = ""
		if bound && countable[action] {
			n, _ = strconv.Atoi(c)
			n = max(1, min(n, maxCount))
		} else if len(c) == 1 && c >= "1" && c <= "8" {
			// Sayıdan sonra sayılamayan bir tuş: rakam sayfaya götürür, sonra tuş işlenir.
			out, cmd := m.keyCanon(msg, c)
			mm := out.(Model)
			out2, cmd2 := mm.keyAction(msg, raw, action, bound)
			return out2, tea.Batch(cmd, cmd2)
		}
	}
	if n == 1 {
		return m.keyAction(msg, raw, action, bound)
	}
	var cmds []tea.Cmd
	var out tea.Model = m
	for i := 0; i < n; i++ {
		var cmd tea.Cmd
		out, cmd = out.(Model).keyAction(msg, raw, action, bound)
		cmds = append(cmds, cmd)
	}
	return out, tea.Batch(cmds...)
}

// keyAction, haritadaki işi yürütür: Canon tuşuyla ya da komut olarak.
func (m Model) keyAction(msg tea.KeyPressMsg, raw, action string, bound bool) (tea.Model, tea.Cmd) {
	if !bound {
		return m.keyCanon(msg, raw)
	}
	if action == "" {
		return m, nil // tuş boşa çıkarılmış
	}
	a, _ := keys.ByName(action)
	if a.Canon == "" && !m.actions.open && !m.sortm.open {
		// Dizi komutu (g n, g t…): ? listesi açıksa onu kapatıp gider.
		m.help = false
		return m.runCommand(action)
	}
	if m.overlay() && !navActions[action] {
		return m.keyCanon(msg, raw) // menünün kendi harfi
	}
	return m.keyCanon(msg, a.Canon)
}

// runCommand, tuşu olmayan (yalnız haritadan gelen) işler.
func (m Model) runCommand(action string) (tea.Model, tea.Cmd) {
	digit := map[string]string{
		"go_now": "1", "go_home": "2", "go_search": "3", "go_library": "4",
		"go_queue": "5", "go_devices": "6", "go_profile": "7", "go_settings": "8",
	}
	if d, ok := digit[action]; ok {
		return m.keyCanon(tea.KeyPressMsg{Text: d, Code: rune(d[0])}, d)
	}
	switch action {
	case "go_top":
		return m.openExtra(extraTop)
	case "go_recent":
		return m.openExtra(extraRecent)
	case "go_log":
		return m.openExtra(extraLog)
	case "go_news":
		return m.openNews()
	case "go_people":
		return m.openPeople("", "", pplFollowers)
	case "go_history":
		return m.openHistory()
	}
	return m, nil
}

func (m Model) updateKeyTimeout(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case seqTimeoutMsg:
		if m.seq != "" && m.seqAt.Equal(msg.at) {
			m.seq = ""
		}
	case countTimeoutMsg:
		if m.count != "" && m.countAt.Equal(msg.at) {
			c := m.count
			m.count = ""
			if len(c) == 1 && c >= "1" && c <= "8" {
				return m.keyCanon(tea.KeyPressMsg{Text: c, Code: rune(c[0])}, c)
			}
		}
	}
	return m, nil
}

// kn, bir işin gösterilecek tuşu (ipuçları ve yardım için).
func (m Model) kn(action string) string {
	k := m.km.Key(action)
	if k == "space" {
		return T("key.space")
	}
	return k
}

// pendingKeys, yarım kalan dizi ya da sayı (alt satırda gösterilir).
func (m Model) pendingKeys() string {
	switch {
	case m.seq != "":
		return m.seq + "…"
	case m.count != "":
		return m.count + "…"
	}
	return ""
}

// helpKeys, bir işin ? listesinde gösterilecek tuşları.
func (m Model) helpKeys(action string) string {
	ks := m.km.KeysOf(action)
	out := make([]string, 0, len(ks))
	for _, k := range ks {
		switch k {
		case "space":
			k = T("key.space")
		case "up":
			k = "↑"
		case "down":
			k = "↓"
		}
		out = append(out, k)
	}
	if len(out) == 0 {
		return "–"
	}
	return strings.Join(out, "  ")
}
