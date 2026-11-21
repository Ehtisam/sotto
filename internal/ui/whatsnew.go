package ui

import (
	_ "embed"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Ehtisam/sotto/internal/cli"
	"github.com/Ehtisam/sotto/internal/i18n"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// "Yeni sürümde neler var": sotto güncellenince bir kez açılan pencere.
// Notlar exe'ye gömülü; son görülen sürüm LocalDir/seen-version'da. İlk
// kurulumda gösterilmez (yalnız sürüm yazılır). Herhangi bir tuş kapatır;
// j/k ve pgup/pgdown kaydırır.

//go:embed whatsnew_en.md
var whatsNewEN string

//go:embed whatsnew_tr.md
var whatsNewTR string

type whatsNew struct {
	open    bool
	version string
	notes   []versionNotes
	scroll  int
}

type versionNotes struct {
	version string
	lines   []string
}

// parseNotes, "## x.y.z" başlıklı bölümleri okur (HTML yorumları atlanır).
func parseNotes(text string) []versionNotes {
	var out []versionNotes
	inComment := false
	for _, l := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		t := strings.TrimSpace(l)
		switch {
		case inComment:
			inComment = !strings.Contains(t, "-->")
			continue
		case strings.HasPrefix(t, "<!--"):
			inComment = !strings.Contains(t, "-->")
			continue
		case strings.HasPrefix(t, "## "):
			out = append(out, versionNotes{version: strings.TrimSpace(t[3:])})
			continue
		}
		if len(out) == 0 {
			continue
		}
		last := &out[len(out)-1]
		if t == "" && (len(last.lines) == 0 || last.lines[len(last.lines)-1] == "") {
			continue
		}
		last.lines = append(last.lines, strings.TrimRight(l, " \t"))
	}
	for i := range out { // sondaki boş satırlar
		for n := len(out[i].lines); n > 0 && out[i].lines[n-1] == ""; n-- {
			out[i].lines = out[i].lines[:n-1]
		}
	}
	return out
}

// versionLess, a < b mi ("0.2.10" > "0.2.9"; baştaki v ve -ek yok sayılır).
func versionLess(a, b string) bool {
	pa, pb := versionParts(a), versionParts(b)
	for i := range max(len(pa), len(pb)) {
		x, y := 0, 0
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			return x < y
		}
	}
	return false
}

func versionParts(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+ "); i >= 0 {
		v = v[:i]
	}
	var out []int
	for _, p := range strings.Split(v, ".") {
		n, _ := strconv.Atoi(p)
		out = append(out, n)
	}
	return out
}

// notesBetween: seen'den sonra gelen, current'a kadar olan sürümlerin notları.
func notesBetween(all []versionNotes, seen, current string) []versionNotes {
	var out []versionNotes
	for _, n := range all {
		if versionLess(seen, n.version) && !versionLess(current, n.version) {
			out = append(out, n)
		}
	}
	return out
}

func seenVersionFile(localDir string) string { return filepath.Join(localDir, "seen-version") }

// checkWhatsNew, açılışta: sürüm değiştiyse notları açar ve yeni sürümü yazar.
func (m Model) checkWhatsNew(current string) Model {
	if m.cfg.LocalDir == "" || current == "" {
		return m
	}
	path := seenVersionFile(m.cfg.LocalDir)
	b, err := os.ReadFile(path)
	seen := strings.TrimSpace(string(b))
	if seen == current {
		return m
	}
	_ = os.MkdirAll(m.cfg.LocalDir, 0o700)
	_ = os.WriteFile(path, []byte(current+"\n"), 0o600)
	if err != nil || seen == "" || m.wiz.on {
		return m // ilk kurulum: gösterilecek bir "yeni" yok
	}
	text := whatsNewEN
	if i18n.Lang() == "tr" {
		text = whatsNewTR
	}
	if notes := notesBetween(parseNotes(text), seen, current); len(notes) > 0 {
		m.whatsnew = whatsNew{open: true, version: current, notes: notes}
	}
	return m
}

func (m Model) whatsNewLines() []string {
	var out []string
	for i, n := range m.whatsnew.notes {
		if i > 0 {
			out = append(out, "")
		}
		out = append(out, theme.Hi.Bold(true).Render(n.version))
		for _, l := range n.lines {
			out = append(out, theme.Plain.Render(l))
		}
	}
	return out
}

func (m Model) whatsNewView(w, h int) []string {
	head := []string{theme.Title.Render(F("new.title", m.whatsnew.version)), theme.Dim.Render(T("new.sub")), ""}
	lines := m.whatsNewLines()
	room := max(h-len(head)-2, 1)
	sc := max(0, min(m.whatsnew.scroll, len(lines)-room))
	n := len(lines)
	if n > room {
		lines = lines[sc:min(n, sc+room)]
	}
	rows := withScrollbar(append(head, lines...), len(head), n, sc, room, w)
	return append(rows, "", theme.Faint.Render(T("new.close")))
}

// whatsNewKey: pencere açıkken her tuş buraya gelir. Kaydırma tuşları
// kaydırır; başka her tuş kapatır (ve o tuş başka bir iş yapmaz).
func (m Model) whatsNewKey(k string) Model {
	step := 0
	switch k {
	case "j", "down":
		step = 1
	case "k", "up":
		step = -1
	case "pgdown", "space":
		step = 10
	case "pgup":
		step = -10
	default:
		m.whatsnew = whatsNew{}
		return m
	}
	_, h := m.contentSize()
	m.whatsnew.scroll = max(0, min(m.whatsnew.scroll+step, len(m.whatsNewLines())-(h-5)))
	return m
}

// appVersion: testler değiştirebilsin diye.
var appVersion = func() string { return cli.Version }
