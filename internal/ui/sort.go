package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/cli"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// Sıralama: açık listede ya da Kitaplık'ta shift+O sıralama menüsünü açar.
// Seçilen sıralama yeniden seçilince ters döner. Her listenin sıralaması
// %LOCALAPPDATA%\sotto\sort.json'da saklanır (Spotify'daki gibi listeye özel).

// Şarkı listesi sıralamaları.
const (
	sortCustom   = ""
	sortName     = "name"
	sortArtist   = "artist"
	sortAlbum    = "album"
	sortAdded    = "added"
	sortDuration = "duration"
	sortReleased = "released" // albümün çıkış tarihi
	// Kitaplık
	sortRecent = "recent"
	sortOwner  = "owner"
)

// libSortKey: Kitaplık'ın sıralaması sort.json'da bu adla.
const libSortKey = "library"

type sortPref struct {
	By   string `json:"by"`
	Desc bool   `json:"desc,omitempty"`
}

func sortFile(dir string) string { return filepath.Join(dir, "sort.json") }

func loadSorts(dir string) map[string]sortPref {
	out := map[string]sortPref{}
	if b, err := os.ReadFile(sortFile(dir)); err == nil {
		_ = json.Unmarshal(b, &out)
	}
	return out
}

func saveSorts(dir string, s map[string]sortPref) {
	if dir == "" {
		return
	}
	_ = os.MkdirAll(dir, 0o700)
	if b, err := json.MarshalIndent(s, "", "  "); err == nil {
		_ = os.WriteFile(sortFile(dir), b, 0o600)
	}
}

// sortTracks, şarkıların sıralanmış yerleri.
func sortTracks(tl *trackList, by string, desc bool) []int {
	idx := make([]int, len(tl.tracks))
	for i := range idx {
		idx[i] = i
	}
	key := func(i int) string {
		t := tl.tracks[i]
		switch by {
		case sortName:
			return cli.Fold(t.Name)
		case sortArtist:
			return cli.Fold(t.ArtistNames() + "\x00" + t.Album.Name)
		case sortAlbum:
			return cli.Fold(t.Album.Name)
		case sortReleased:
			// "1999", "1999-05" ya da "1999-05-01": dizgi sırası tarih sırası.
			if t.Album.ReleaseDate != "" {
				return t.Album.ReleaseDate + "\x00" + cli.Fold(t.Album.Name)
			}
			return t.ReleaseDate
		case sortAdded:
			if t.AddedAt != "" {
				return t.AddedAt
			}
			return t.ReleaseDate
		}
		return ""
	}
	less := func(a, b int) bool {
		if by == sortDuration {
			return tl.tracks[a].DurationMS < tl.tracks[b].DurationMS
		}
		return key(a) < key(b)
	}
	sort.SliceStable(idx, func(x, y int) bool {
		if desc {
			return less(idx[y], idx[x])
		}
		return less(idx[x], idx[y])
	})
	return idx
}

// sortEntries, Kitaplık girişlerini sıralar (Beğenilenler hep üstte, klasörler
// listelerden önce).
func sortEntries(es []libEntry, by string, desc bool, apiPos map[string]int) []libEntry {
	if by == sortCustom {
		return es
	}
	out := append([]libEntry(nil), es...)
	rank := func(e libEntry) int {
		switch e.kind {
		case kindLiked:
			return 0
		case kindFolder:
			return 1
		}
		return 2
	}
	pos := apiPos
	sort.SliceStable(out, func(x, y int) bool {
		a, b := out[x], out[y]
		if rank(a) != rank(b) {
			return rank(a) < rank(b)
		}
		var l bool
		switch by {
		case sortName:
			l = cli.Fold(a.name) < cli.Fold(b.name)
		case sortOwner:
			l = cli.Fold(a.owner+"\x00"+a.name) < cli.Fold(b.owner+"\x00"+b.name)
		default: // sortRecent: Spotify'ın verdiği sıra (en son eklenen üstte)
			l = pos[a.key] < pos[b.key]
		}
		if desc {
			ka, kb := a, b
			switch by {
			case sortName:
				return cli.Fold(kb.name) < cli.Fold(ka.name)
			case sortOwner:
				return cli.Fold(kb.owner+"\x00"+kb.name) < cli.Fold(ka.owner+"\x00"+ka.name)
			}
			return pos[b.key] < pos[a.key]
		}
		return l
	})
	return out
}

// sortLabel, liste başlığındaki "· ada göre ↑" yazısı.
func sortLabel(by string, desc bool) string {
	if by == "" {
		return ""
	}
	arrow := " ↑"
	if desc {
		arrow = " ↓"
	}
	return " · " + T("sort.by."+by) + arrow
}

// ---- menü ----

type sortMenu struct {
	open    bool
	library bool // Kitaplık için (değilse açık liste)
	choices []string
	cursor  int
}

// sortChoices, bulunulan liste için sıralamalar.
func (m Model) sortChoices(tl *trackList) []string {
	if tl == nil {
		out := []string{sortCustom, sortRecent, sortName}
		return append(out, sortOwner)
	}
	if tl.entry.kind == kindShow {
		return []string{sortCustom, sortName, sortAdded, sortDuration}
	}
	out := []string{sortCustom, sortName, sortArtist}
	if tl.entry.kind != kindAlbum {
		out = append(out, sortAlbum, sortReleased, sortAdded)
	}
	return append(out, sortDuration)
}

// openSort, shift+O: sıralama menüsü. ok=false: burada sıralanacak liste yok.
func (m Model) openSort() (Model, bool) {
	if m.focus != focusContent {
		return m, false
	}
	tl := m.shownList()
	lib := tl == nil && m.page == pageLibrary && m.spState == spReady
	if tl == nil && !lib {
		return m, false
	}
	m.sortm = sortMenu{open: true, library: lib, choices: m.sortChoices(tl)}
	cur := m.lib.sortBy
	if tl != nil {
		cur = tl.sortBy
	}
	for i, c := range m.sortm.choices {
		if c == cur {
			m.sortm.cursor = i
		}
	}
	return m, true
}

func (m Model) sortKey(k string) (Model, tea.Cmd) {
	s := &m.sortm
	switch k {
	case "up", "k":
		s.cursor = max(0, s.cursor-1)
	case "down", "j":
		s.cursor = min(len(s.choices)-1, s.cursor+1)
	case "esc", "left", "backspace", "O", "q":
		m.sortm = sortMenu{}
	case "enter", "right", "space", " ":
		return m.applySort(s.choices[s.cursor])
	}
	return m, nil
}

// applySort, seçilen sıralamayı uygular; aynısı seçildiyse yönü çevirir.
func (m Model) applySort(by string) (Model, tea.Cmd) {
	lib := m.sortm.library
	m.sortm = sortMenu{}
	var cur *sortPref
	key := libSortKey
	pref := sortPref{By: m.lib.sortBy, Desc: m.lib.desc}
	tl := m.shownList()
	if !lib {
		if tl == nil {
			return m, nil
		}
		key, pref = tl.entry.key, sortPref{By: tl.sortBy, Desc: tl.desc}
	}
	cur = &pref
	if cur.By == by && by != sortCustom {
		cur.Desc = !cur.Desc
	} else {
		cur.By, cur.Desc = by, by == sortAdded // en yeni eklenen üstte başlasın
	}
	if lib {
		m.lib.sortBy, m.lib.desc = cur.By, cur.Desc
		m.lib.cursor, m.lib.scroll = 0, 0
		m.clampLib()
	} else {
		keep := tl.cursor
		tl.sortBy, tl.desc = cur.By, cur.Desc
		tl.cursor, tl.scroll = keep, 0
		tl.clamp(m.listHeight())
	}
	if key != "" {
		if cur.By == sortCustom {
			delete(m.rt.sorts, key)
		} else {
			m.rt.sorts[key] = *cur
		}
		saveSorts(m.cfg.LocalDir, m.rt.sorts)
	}
	m = m.note(T("sort.done") + strings.TrimPrefix(sortLabel(cur.By, cur.Desc), " · "))
	if cur.By == sortCustom {
		m = m.note(T("sort.custom.done"))
	}
	return m, m.loadAllForSort()
}

// loadAllForSort: sıralanan listenin bütün sayfaları gelsin (yoksa sıra yanlış görünür).
func (m Model) loadAllForSort() tea.Cmd { return m.continueLoadAll(m.shownList()) }

// sortView, sıralama menüsünün satırları.
func (m Model) sortView(w int) []string {
	s := m.sortm
	cur, desc := m.lib.sortBy, m.lib.desc
	title := T("page.library")
	if tl := m.shownList(); !s.library && tl != nil {
		cur, desc, title = tl.sortBy, tl.desc, tl.entry.name
	}
	rows := []string{theme.Title.Render(T("sort.title")), theme.Dim.Render(truncate(title, w)), "", ""}
	rw := min(w-2, 46)
	for i, c := range s.choices {
		label := T("sort.by." + c)
		if c == sortCustom && s.library {
			label = T("sort.by.libcustom")
		}
		mark := ""
		if c == cur {
			mark = "✓"
			if c != sortCustom {
				mark = map[bool]string{false: "↑", true: "↓"}[desc]
			}
		}
		rows = append(rows, "  "+row(label, mark, rw, i == s.cursor))
	}
	return append(rows, "", theme.Faint.Render(T("sort.again")))
}
