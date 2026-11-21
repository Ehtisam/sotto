package ui

import (
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Ehtisam/sotto/internal/cli"
	"github.com/Ehtisam/sotto/internal/keys"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// ? kısayol tablosu: her satırda ne yaptığı, tuşu ve nerede çalıştığı.
// Bölümler keys.Groups sırasıyla. / ile süzülür (açıklama, tuş ya da yer
// içinde sözcük eşleşmesi); esc önce süzgeci temizler, sonra kapatır.

type helpRow struct {
	header      string // bölüm başlığı satırı ise dolu
	desc, where string
	keys        string
}

// helpWhere: bir işin nerede çalıştığı (çeviri anahtarı). Yazılmayanlar
// bölümünün varsayılanını alır (helpGroupWhere).
var helpWhere = map[string]string{
	"find":           "where.find",
	"sort":           "where.sort",
	"find_playing":   "where.lists",
	"search":         "where.everywhere",
	"like":           "where.likeq",
	"queue_add":      "where.likeq",
	"open_clipboard": "where.everywhere",
	"move_down":      "where.ownlist",
	"move_up":        "where.ownlist",
	"delete":         "where.ownlist",
	"new_playlist":   "where.library",
	"new_folder":     "where.library",
	"refresh":        "where.page",
	"lyrics_login":   "where.now",
	"next_tab":       "where.tabs",
	"prev_tab":       "where.tabs",
}

var helpGroupWhere = map[string]string{
	"move": "where.lists", "pages": "where.everywhere", "play": "where.everywhere",
	"item": "where.selected", "other": "where.everywhere",
}

// helpRows, tablonun bütün satırları (süzgeçsiz).
func (m Model) helpRows() []helpRow {
	var out []helpRow
	for _, g := range keys.Groups {
		out = append(out, helpRow{header: T(g.Title)})
		for _, a := range keys.Catalog {
			if a.Group != g.ID {
				continue
			}
			where := helpWhere[a.Name]
			if where == "" {
				where = helpGroupWhere[g.ID]
			}
			out = append(out, helpRow{desc: T(a.Help), keys: m.helpKeys(a.Name), where: T(where)})
		}
		switch g.ID {
		case "move":
			out = append(out, helpRow{desc: T("help.pages"), keys: "1 – 8", where: T("where.everywhere")})
			if m.km.Counts {
				out = append(out, helpRow{desc: T("help.counts"), keys: "5j  3K", where: T("where.lists")})
			}
		case "other":
			out = append(out, helpRow{desc: T("help.lyrpick"), keys: "enter  ↑ ↓", where: T("where.now")})
		}
	}
	return out
}

// shownHelpRows: süzgeç açıksa uyan satırlar; içi boş kalan başlıklar düşer.
func (m Model) shownHelpRows() []helpRow {
	all := m.helpRows()
	words := strings.Fields(cli.Fold(m.helpFind.query))
	if len(words) == 0 {
		return all
	}
	var out []helpRow
	header := -1
	for _, r := range all {
		if r.header != "" {
			out = append(out, r)
			header = len(out) - 1
			continue
		}
		t := cli.Fold(r.desc + " " + r.keys + " " + r.where)
		ok := true
		for _, w := range words {
			if !strings.Contains(t, w) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, r)
			header = -1
		}
	}
	if header >= 0 { // sondaki boş başlık
		out = out[:header]
	}
	// Ortadaki boş başlıkları at.
	var clean []helpRow
	for i, r := range out {
		if r.header != "" && (i+1 >= len(out) || out[i+1].header != "") {
			continue
		}
		clean = append(clean, r)
	}
	return clean
}

// helpLinesAll, tablonun çizilmiş satırları (başlık hariç).
func (m Model) helpLinesAll(w int) []string {
	rows := m.shownHelpRows()
	if len(rows) == 0 {
		return []string{theme.Faint.Render(T("find.none"))}
	}
	keyW, whereW := 16, 24
	if w < 70 {
		whereW = 0 // dar pencerede yer sütunu yok
	}
	descW := max(10, w-keyW-whereW-2)
	pad := func(s string, n int) string {
		s = truncate(s, n)
		return s + strings.Repeat(" ", max(0, n-lipgloss.Width(s)))
	}
	var lines []string
	for i, r := range rows {
		if r.header != "" {
			if i > 0 {
				lines = append(lines, "")
			}
			head := theme.Dim.Italic(true).Render(pad(r.header, descW))
			if whereW > 0 && i == 0 {
				head += theme.Faint.Render(pad(T("help.col.key"), keyW) + T("help.col.where"))
			}
			lines = append(lines, head)
			continue
		}
		line := theme.Plain.Render(pad(r.desc, descW-1)) + " " + theme.Hi.Render(pad(r.keys, keyW))
		if whereW > 0 {
			line += theme.Faint.Render(truncate(r.where, whereW))
		}
		lines = append(lines, line)
	}
	return lines
}

func (m Model) helpView(w, h int) []string {
	head := []string{theme.Title.Render(T("help.title")) + theme.Faint.Render("  ·  "+T("help.keymap")), ""}
	if m.helpFind.active() {
		all := 0
		for _, r := range m.helpRows() {
			if r.header == "" {
				all++
			}
		}
		shown := 0
		for _, r := range m.shownHelpRows() {
			if r.header == "" {
				shown++
			}
		}
		head[1] = findBar(m.helpFind, shown, all, w)
	}
	lines := m.helpLinesAll(w)
	room := max(h-len(head), 1)
	sc := max(0, min(m.helpScroll, len(lines)-room))
	if len(lines) > room {
		return append(head, lines[sc:min(len(lines), sc+room)]...)
	}
	return append(head, lines...)
}

// helpKey, ? listesi açıkken kaydırma ve süzme tuşları. ok=false: tuş genel kısayola düşer.
func (m Model) helpKey(k string) (Model, bool) {
	step := 0
	switch k {
	case "/", "ctrl+f":
		m.helpFind.typing, m.helpScroll = true, 0
		return m, true
	case "j", "down":
		step = 1
	case "k", "up":
		step = -1
	case "pgdown":
		step = 10
	case "pgup":
		step = -10
	case "home":
		m.helpScroll = 0
		return m, true
	case "G", "end":
		step = 1 << 20
	default:
		return m, false
	}
	// En çok: satır sayısı - görünen yükseklik (helpView ile aynı hesap).
	w, h := m.contentSize()
	rows := m.helpView(w, 1<<20)
	m.helpScroll = max(0, min(m.helpScroll+step, len(rows)-2-(h-2)))
	return m, true
}

// helpFindKey: ? listesinde süzgece yazarken. ok=false: yazılmıyor.
func (m Model) helpFindKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	f := &m.helpFind
	if !f.typing {
		return m, nil, false
	}
	k := msg.String()
	switch k {
	case "esc":
		*f = finder{}
	case "enter", "down", "tab":
		f.typing = false
		if f.query == "" {
			*f = finder{}
		}
	case "backspace":
		if f.query == "" {
			*f = finder{}
			break
		}
		_, size := utf8.DecodeLastRuneInString(f.query)
		f.query = f.query[:len(f.query)-size]
	case "ctrl+u":
		f.query = ""
	case "ctrl+c":
		return m, m.quit(), true
	default:
		text := msg.Key().Text
		if k == "space" {
			text = " "
		}
		f.query += text
	}
	m.helpScroll = 0
	return m, nil, true
}
