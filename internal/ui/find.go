package ui

import (
	"sort"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/cli"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// Sayfa içi arama: açık bir listede (ya da Kitaplık'ın liste düzeyinde) /
// ya da ctrl+f ile yazmaya başlanır, liste yazdıkça süzülür. Önce adın,
// sanatçının ya da albümün içinde geçenler (listedeki sırayla), ardından
// harfleri sırayla ama aralıklı geçenler (bulanık; en iyi eşleşen önce).
// enter ya da ↓ sonuçlara geçer, esc önce süzgeci temizler.

type finder struct {
	query  string
	typing bool
}

func (f finder) active() bool { return f.query != "" || f.typing }

// ---- eşleştirme ----

// fuzzyScore: q'nun harfleri text'te sırayla geçiyorsa puanı (büyük iyi).
// Ardışık harfler ve sözcük başları ödüllendirilir; boşluklar cezalandırılır.
func fuzzyScore(q, text string) (int, bool) {
	if q == "" {
		return 0, true
	}
	qr := []rune(q)
	score, qi, run, prev := 0, 0, 0, ' '
	gaps := 0
	for _, r := range text {
		if qi < len(qr) && r == qr[qi] {
			s := 1
			if run > 0 {
				s += 2 * run
			}
			if prev == ' ' || prev == '-' || prev == '(' || prev == '/' {
				s += 4
			}
			score += s
			run++
			qi++
		} else {
			if qi > 0 && qi < len(qr) {
				gaps++
			}
			run = 0
		}
		prev = r
	}
	if qi < len(qr) {
		return 0, false
	}
	return score*4 - gaps, true
}

// matchOrder, sorguya uyan satırların sırası: önce sorgunun bütünüyle geçtiği
// satırlar, sonra bütün sözcüklerinin geçtiği satırlar (ikisi de verilen
// sırayla), en son bulanık uyanlar (puana göre). base nil ise 0..n-1.
func matchOrder(base []int, n int, text func(i int) string, query string) []int {
	q := strings.TrimSpace(cli.Fold(query))
	if base == nil {
		base = make([]int, n)
		for i := range base {
			base[i] = i
		}
	}
	if q == "" {
		return base
	}
	words := strings.Fields(q)
	phrase := strings.Join(words, " ")
	var whole, exact []int
	type scored struct{ i, s, at int }
	var fuzzy []scored
	for at, i := range base {
		t := cli.Fold(text(i))
		if strings.Contains(t, phrase) {
			whole = append(whole, i)
			continue
		}
		all := true
		for _, w := range words {
			if !strings.Contains(t, w) {
				all = false
				break
			}
		}
		if all {
			exact = append(exact, i)
			continue
		}
		if s, ok := fuzzyScore(strings.ReplaceAll(q, " ", ""), t); ok {
			fuzzy = append(fuzzy, scored{i, s, at})
		}
	}
	sort.SliceStable(fuzzy, func(a, b int) bool { return fuzzy[a].s > fuzzy[b].s })
	out := append(whole, exact...)
	for _, f := range fuzzy {
		out = append(out, f.i)
	}
	if out == nil {
		out = []int{}
	}
	return out
}

// ---- listede görünen sıra ----

// view, listede görünen şarkıların sırası (tracks içindeki yerleri): sıralama
// ve sayfa içi arama uygulanmış. nil: hepsi, kendi sırasıyla.
func (tl *trackList) view() []int {
	key := viewKey{tl.ver, len(tl.tracks), tl.find.query, tl.sortBy, tl.desc}
	if tl.cache != nil && tl.cacheKey == key {
		return tl.cache
	}
	var out []int
	if tl.sortBy != "" {
		out = sortTracks(tl, tl.sortBy, tl.desc)
	}
	if tl.find.query != "" {
		out = matchOrder(out, len(tl.tracks), func(i int) string {
			t := tl.tracks[i]
			return t.Name + " " + t.ArtistNames() + " " + t.Album.Name
		}, tl.find.query)
	}
	tl.cache, tl.cacheKey = out, key
	return out
}

type viewKey struct {
	ver, n      int
	query, sort string
	desc        bool
}

// changed, şarkılar değişince (taşıma, silme) görünümü yeniden kurdurur.
func (tl *trackList) changed() { tl.ver++ }

// rows: görünen satır sayısı.
func (tl *trackList) rows() int {
	if v := tl.view(); v != nil {
		return len(v)
	}
	return len(tl.tracks)
}

// at, görünen p. satırın tracks içindeki yeri.
func (tl *trackList) at(p int) int {
	if v := tl.view(); v != nil {
		return v[p]
	}
	return p
}

// pos, imlecin görünen sıradaki yeri (-1: görünmüyor).
func (tl *trackList) pos() int {
	v := tl.view()
	if v == nil {
		return tl.cursor
	}
	for p, i := range v {
		if i == tl.cursor {
			return p
		}
	}
	return -1
}

// natural: liste kendi sırasında ve süzülmemiş (taşıma yalnız böyleyken).
func (tl *trackList) natural() bool { return tl.view() == nil }

// ---- tuşlar ----

// shownFinder, bulunulan yerde sayfa içi aramayı alan süzgeç (yoksa nil).
func (m *Model) shownFinder() *finder {
	if m.focus != focusContent {
		return nil
	}
	if tl := m.shownList(); tl != nil {
		return &tl.find
	}
	if m.page == pageLibrary && m.spState == spReady && m.lib.open == nil {
		return &m.lib.find
	}
	if m.page == pageSettings && !m.extraOn() {
		return &m.set.find
	}
	return nil
}

// startFind, / ya da ctrl+f: sayfa içi aramaya yazmaya başlar.
func (m Model) startFind() (Model, tea.Cmd, bool) {
	f := m.shownFinder()
	if f == nil {
		return m, nil, false
	}
	f.typing = true
	return m, m.loadAllForFind(), true
}

// loadAllForFind: süzülen listenin henüz gelmemiş sayfalarını ister (arama
// bütün listede olsun).
func (m Model) loadAllForFind() tea.Cmd { return m.continueLoadAll(m.shownList()) }

// continueLoadAll: liste süzülüyor ya da sıralanıyorsa bir sonraki sayfayı
// ister (sonuç ve sıra bütün listeye göre olsun).
func (m Model) continueLoadAll(tl *trackList) tea.Cmd {
	if tl == nil || (!tl.find.active() && tl.sortBy == "") || tl.loading || tl.err != nil ||
		len(tl.tracks) >= tl.total || len(tl.tracks) >= maxFindLoad {
		return nil
	}
	tl.loading = true
	return m.loadTracks(tl.entry, len(tl.tracks))
}

// maxFindLoad: sayfa içi arama için en çok bu kadar şarkı yüklenir.
const maxFindLoad = 10000

// findKey, sayfa içi aramaya yazarken tuşlar. ok=false: arama yazılmıyor.
func (m Model) findKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	f := m.shownFinder()
	if f == nil || !f.typing {
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
		if text == "" {
			return m, nil, true
		}
		f.query += text
	}
	m.afterFind()
	return m, m.loadAllForFind(), true
}

// findPaste, yapıştırılan metni sayfa içi aramaya ekler.
func (m Model) findPaste(text string) (Model, bool) {
	f := m.shownFinder()
	if f == nil || !f.typing {
		return m, false
	}
	f.query += strings.Join(strings.Fields(text), " ")
	m.afterFind()
	return m, true
}

// afterFind: süzgeç değişince imleci ilk sonuca (ya da görünür bir yere) alır.
func (m *Model) afterFind() {
	h := m.listHeight()
	if tl := m.shownList(); tl != nil {
		if tl.rows() > 0 && (tl.find.query != "" || tl.pos() < 0) {
			tl.cursor = tl.at(0)
		}
		tl.scroll = 0
		tl.clamp(h)
		return
	}
	if m.page == pageLibrary {
		m.lib.cursor, m.lib.scroll = 0, 0
		m.clampLib()
	}
	if m.page == pageSettings {
		m.set.cursor = 0
	}
}

// clearFind: esc süzgeç açıkken önce onu temizler. ok=false: süzgeç yoktu.
func (m Model) clearFind() (Model, bool) {
	f := m.shownFinder()
	if f == nil || !f.active() {
		return m, false
	}
	keep := -1
	if tl := m.shownList(); tl != nil {
		keep = tl.cursor
	}
	if f == &m.set.find {
		// Seçili ayarda kal.
		sel := m.settingSelected()
		*f = finder{}
		for i, id := range settingIDs() {
			if id == sel {
				m.set.cursor = i
			}
		}
		return m, true
	}
	*f = finder{}
	if tl := m.shownList(); tl != nil && keep >= 0 {
		tl.cursor = keep // seçili şarkıda kal
		tl.clamp(m.listHeight())
	} else {
		m.clampLib()
	}
	return m, true
}

// ---- çizim ----

// findBar, süzgeç açıksa liste başlığının altındaki satır.
func findBar(f finder, shown, total int, w int) string {
	if !f.active() {
		return ""
	}
	q := theme.Hi.Render("/ ") + theme.Plain.Render(f.query)
	if f.typing {
		q += theme.Hi.Render("▏")
	}
	count := theme.Faint.Render("  " + F("find.count", shown, total))
	if f.query == "" {
		count = theme.Faint.Render("  " + T("find.type"))
	}
	return truncate(q+count, w)
}
