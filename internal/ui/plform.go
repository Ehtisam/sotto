package ui

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/cover"
	"github.com/Ehtisam/sotto/internal/spotify"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// Liste formu: yeni liste açmak ya da kendi listenin adını, açıklamasını,
// kimlerin görebileceğini ve kapağını değiştirmek. Eylem menüsünden (e),
// "listeye ekle"nin "yeni liste" satırından ya da Kitaplık'ta N ile açılır.

const (
	plRowName = iota
	plRowDesc
	plRowVis
	plRowCover
	plRowSave
)

// Görünürlük: gizli, herkese açık, ortak (ortak listeler gizli olmak zorunda).
const (
	visPrivate = iota
	visPublic
	visCollab
	visCount
)

var visKeys = [visCount]string{"pl.vis.private", "pl.vis.public", "pl.vis.collab"}

type plForm struct {
	on     bool
	create bool
	entry  libEntry // düzenlenen liste
	addURI string   // yeni liste açılınca içine eklenecek şarkı

	name, desc, cover string
	vis, origVis      int
	row               int

	busy bool
	err  string
	auth bool // kapak için yeniden giriş gerekiyor (ctrl+l)

	folder   bool   // klasör formu: yalnızca ad (folders.go)
	folderID string // yeniden adlandırılan klasör ("): yeni klasör
	inFolder string // yeni liste bu klasöre konacak
}

type plSavedMsg struct {
	create  bool
	key     string
	name    string
	details spotify.PlaylistDetails
	saved   bool // ayrıntılar kaydedildi
	covered bool // kapak değişti
	err     error
	scope   bool // kapak izni yok
}

func visOf(e libEntry) int {
	switch {
	case e.collaborative:
		return visCollab
	case e.public:
		return visPublic
	}
	return visPrivate
}

// openPlaylistForm, yeni liste (e boşsa) ya da düzenleme formunu açar.
func (m Model) openPlaylistForm(e *libEntry, addURI string) Model {
	f := plForm{on: true, create: e == nil, addURI: addURI}
	// Kitaplık'ta bir klasörün içindeyken yeni liste o klasöre girer.
	if e == nil && m.page == pageLibrary && m.foldersOn() && m.lib.filter == libAll {
		f.inFolder = m.lib.folder
	}
	if e != nil {
		f.entry, f.name, f.desc = *e, e.name, e.desc
		f.vis = visOf(*e)
		f.origVis = f.vis
	}
	m.actions = actionMenu{}
	m.form = f
	return m
}

func (f plForm) rows() []int {
	if f.folder {
		return []int{plRowName, plRowSave}
	}
	if f.create {
		return []int{plRowName, plRowDesc, plRowVis, plRowSave}
	}
	return []int{plRowName, plRowDesc, plRowVis, plRowCover, plRowSave}
}

func (f *plForm) field() *string {
	switch f.rows()[f.row] {
	case plRowName:
		return &f.name
	case plRowDesc:
		return &f.desc
	case plRowCover:
		return &f.cover
	}
	return nil
}

func (m Model) formKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	k := msg.String()
	f := &m.form
	if f.busy {
		if k == "esc" {
			m.form = plForm{}
		}
		return m, nil
	}
	n := len(f.rows())
	switch k {
	case "esc":
		m.form = plForm{}
		return m, nil
	case "up", "shift+tab":
		f.row = (f.row + n - 1) % n
		return m, nil
	case "down", "tab":
		f.row = (f.row + 1) % n
		return m, nil
	case "ctrl+s":
		return m.submitPlaylistForm()
	case "ctrl+l":
		if f.auth {
			m.form = plForm{}
			m.spState = spLoggingIn
			return m.note(T("pl.relogin")), m.login()
		}
		return m, nil
	}
	row := f.rows()[f.row]
	if row == plRowVis {
		switch k {
		case "left", "h":
			f.vis = (f.vis + visCount - 1) % visCount
		case "right", "l", "space", " ", "enter":
			f.vis = (f.vis + 1) % visCount
		}
		return m, nil
	}
	if row == plRowSave {
		if k == "enter" {
			return m.submitPlaylistForm()
		}
		return m, nil
	}
	p := f.field()
	switch k {
	case "enter":
		f.row = min(f.row+1, n-1)
	case "backspace":
		if *p != "" {
			_, size := utf8.DecodeLastRuneInString(*p)
			*p = (*p)[:len(*p)-size]
		}
	case "ctrl+u":
		*p = ""
	default:
		text := msg.Key().Text
		if k == "space" {
			text = " "
		}
		*p += text
	}
	f.err = ""
	return m, nil
}

// submitPlaylistForm, formu Spotify'a gönderir.
func (m Model) submitPlaylistForm() (Model, tea.Cmd) {
	f := &m.form
	name := strings.TrimSpace(f.name)
	if name == "" {
		f.err = T("pl.noname")
		if f.folder {
			f.err = T("folder.noname")
		}
		f.row = 0
		return m, nil
	}
	if f.folder {
		id, old := f.folderID, f.entry.name
		m.form = plForm{}
		switch {
		case id == "":
			return m, m.createFolder(name)
		case name != old:
			return m, m.renameFolder(id, name)
		}
		return m, nil
	}
	desc := strings.TrimSpace(f.desc)
	var d spotify.PlaylistDetails
	if f.create || name != f.entry.name {
		d.Name = &name
	}
	if f.create || desc != f.entry.desc {
		d.Description = &desc
	}
	if f.create || f.vis != f.origVis {
		pub, collab := f.vis == visPublic, f.vis == visCollab
		d.Public, d.Collaborative = &pub, &collab
	}
	coverPath := strings.TrimSpace(f.cover)
	if !f.create && d == (spotify.PlaylistDetails{}) && coverPath == "" {
		m.form = plForm{}
		return m, nil // değişen bir şey yok
	}
	f.busy, f.err, f.auth = true, "", false
	sp, create, key, addURI, me := m.sp, f.create, f.entry.key, f.addURI, m.me.ID
	br, inFolder := m.rt.bridge, f.inFolder
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		msg := plSavedMsg{create: create, key: key, name: name, details: d}
		if create {
			p, err := sp.CreatePlaylist(ctx, me, d)
			if err != nil {
				msg.err = err
				return msg
			}
			msg.key = p.ID
			if addURI != "" {
				if msg.err = sp.AddToPlaylist(ctx, p.ID, []string{addURI}); msg.err != nil {
					return msg
				}
			}
			if inFolder != "" {
				msg.err = moveNewIntoFolder(ctx, br, sp, me, p.URI, inFolder)
			}
			return msg
		}
		if d != (spotify.PlaylistDetails{}) {
			if msg.err = sp.UpdatePlaylist(ctx, key, d); msg.err != nil {
				return msg
			}
		}
		msg.saved = true
		if coverPath != "" {
			jpg, err := cover.CoverJPEG(coverPath)
			if err != nil {
				msg.err = err
				return msg
			}
			if err := sp.UploadPlaylistCover(ctx, key, jpg); err != nil {
				msg.err = err
				msg.scope = spotify.IsStatus(err, 403) || spotify.IsStatus(err, 401)
				return msg
			}
			msg.covered = true
		}
		return msg
	}
}

func (m Model) updatePlaylistForm(msg plSavedMsg) (Model, tea.Cmd) {
	f := &m.form
	f.busy = false
	// Ayrıntılar kaydedildiyse (kapak olmasa bile) kitaplıktaki hali güncellensin.
	if !msg.create && msg.saved {
		m = m.applyDetails(msg.key, msg.details)
		f.entry.name, f.origVis = msg.name, f.vis
		if msg.details.Description != nil {
			f.entry.desc = *msg.details.Description
		}
	}
	switch {
	case msg.scope:
		f.err, f.auth = T("pl.cover.scope"), true
		return m, nil
	case msg.err != nil:
		if f.on {
			f.err = msg.err.Error()
			return m, nil
		}
		return m.say(msg.err.Error()), nil
	}
	m.form = plForm{}
	note := F("pl.saved", msg.name)
	if msg.create {
		note = F("pl.created", msg.name)
	}
	m = m.note(note)
	if msg.create || msg.covered {
		m.lib.loading = true
		return m, m.loadLibrary()
	}
	return m, nil
}

// applyDetails, kaydedilen ayrıntıları kitaplıkta ve açık listelerde gösterir.
func (m Model) applyDetails(key string, d spotify.PlaylistDetails) Model {
	apply := func(e *libEntry) {
		if e.key != key {
			return
		}
		if d.Name != nil {
			e.name = *d.Name
		}
		if d.Description != nil {
			e.desc = *d.Description
		}
		if d.Public != nil {
			e.public = *d.Public
		}
		if d.Collaborative != nil {
			e.collaborative = *d.Collaborative
		}
	}
	entries := append([]libEntry(nil), m.lib.entries...)
	for i := range entries {
		apply(&entries[i])
	}
	m.lib.entries = entries
	for _, tl := range []*trackList{m.lib.open, m.search.open, m.home.open, m.prof.open} {
		if tl != nil {
			apply(&tl.entry)
		}
	}
	return m
}

// ---- çizim ----

func (m Model) playlistFormView(w int) []string {
	f := m.form
	title := T("pl.title.edit")
	switch {
	case f.folder && f.folderID == "":
		title = T("folder.title.new")
	case f.folder:
		title = T("folder.title.rename")
	case f.create:
		title = T("pl.title.new")
	}
	rows := []string{theme.Title.Render(title)}
	switch {
	case f.folder && f.folderID != "":
		rows = append(rows, theme.Dim.Render(truncate(f.entry.name, w)))
	case f.folder:
		rows = append(rows, theme.Dim.Render(T("folder.new.where")))
	case !f.create:
		rows = append(rows, theme.Dim.Render(truncate(f.entry.name, w)))
	case f.addURI != "":
		rows = append(rows, theme.Dim.Render(T("pl.new.add")))
	case f.inFolder != "":
		rows = append(rows, theme.Dim.Render(T("pl.new.infolder")))
	default:
		rows = append(rows, "")
	}
	rows = append(rows, "", "")
	lw := 16
	fw := max(min(w-lw-4, 60), 10)
	for i, r := range f.rows() {
		sel := i == f.row
		label := ""
		value := ""
		switch r {
		case plRowName:
			label, value = T("pl.name"), f.name
		case plRowDesc:
			label, value = T("pl.desc"), f.desc
		case plRowVis:
			label = T("pl.vis")
			var opts []string
			for v, key := range visKeys {
				if v == f.vis {
					opts = append(opts, theme.Hi.Underline(true).Render(T(key)))
				} else {
					opts = append(opts, theme.Dim.Render(T(key)))
				}
			}
			value = strings.Join(opts, theme.Faint.Render(" · "))
		case plRowCover:
			label, value = T("pl.cover"), f.cover
		case plRowSave:
			text := T("pl.save")
			if f.busy {
				text = T("pl.saving")
			}
			if sel {
				rows = append(rows, "", "  "+theme.SelectedRow.Render(" "+text+" "))
			} else {
				rows = append(rows, "", "  "+theme.Hi.Render(text))
			}
			continue
		}
		lab := theme.Dim.Render(padRight(label, lw))
		if sel {
			lab = theme.Hi.Render(padRight(label, lw))
		}
		if r != plRowVis {
			shown := value
			if textWidth(shown) > fw {
				shown = "…" + tail(shown, fw-1)
			}
			switch {
			case sel:
				value = theme.Plain.Render(shown) + theme.Hi.Render("▏")
			case value == "" && r == plRowCover:
				value = theme.Faint.Render(T("pl.cover.hint"))
			default:
				value = theme.Plain.Render(shown)
			}
		}
		mark := "  "
		if sel {
			mark = theme.Hi.Render("› ")
		}
		rows = append(rows, mark+lab+value)
	}
	if f.err != "" {
		rows = append(rows, "", theme.Error.Render(truncate(f.err, w)))
		if f.auth {
			rows = append(rows, theme.Dim.Render(T("pl.cover.relogin")))
		}
	}
	return rows
}

func padRight(s string, w int) string {
	if n := textWidth(s); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}

// tail, metnin son w sütunu (uzun dosya yolları için).
func tail(s string, w int) string {
	r := []rune(s)
	for len(r) > 0 && textWidth(string(r)) > w {
		r = r[1:]
	}
	return string(r)
}

// ---- silme ----

type plDeletedMsg struct {
	key, name string
	err       error
}

// ownList: kullanıcının kendi listesi (ortak olsa da sahibi o).
func (m Model) ownList(e libEntry) bool {
	return e.kind == kindPlaylist && e.ownerID != "" && e.ownerID == m.me.ID
}

// deletePlaylist, kendi listeni siler. Spotify'da bu, listeyi takipten
// çıkarmaktır; Spotify hesap sayfasından 90 gün içinde geri alınabilir.
func (m Model) deletePlaylist(e libEntry) (Model, tea.Cmd) {
	m.actions = actionMenu{}
	if !m.ownList(e) {
		return m, nil
	}
	sp := m.sp
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return plDeletedMsg{key: e.key, name: e.name, err: sp.Unsave(ctx, []string{e.uri})}
	}
}

func (m Model) updatePlaylistDeleted(msg plDeletedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		return m.say(msg.err.Error()), nil
	}
	var entries []libEntry
	for _, e := range m.lib.entries {
		if e.key != msg.key {
			entries = append(entries, e)
		}
	}
	m.lib.entries = entries
	m.clampLib()
	// Silinen liste açıksa kapat.
	if tl := m.lib.open; tl != nil && tl.entry.key == msg.key {
		m.lib.open = nil
	}
	if tl := m.search.open; tl != nil && tl.entry.key == msg.key {
		m.search.open = nil
	}
	if tl := m.home.open; tl != nil && tl.entry.key == msg.key {
		m.home.open = nil
	}
	if tl := m.prof.open; tl != nil && tl.entry.key == msg.key {
		m.prof.open = nil
	}
	// Son aramalarda da kalmasın.
	m, cmd := m.forgetSearch("spotify:playlist:" + msg.key)
	return m.note(F("act.deleted", msg.name)), tea.Batch(cmd, m.loadRootlist())
}

// openFolderForm, yeni klasör (e nil) ya da klasörün adını değiştirme formu.
func (m Model) openFolderForm(e *libEntry) Model {
	f := plForm{on: true, folder: true, create: e == nil}
	if e != nil {
		f.entry, f.name, f.folderID = *e, e.name, e.key
	}
	m.actions = actionMenu{}
	m.form = f
	return m
}

// inRootlist: liste kitaplık sırasında var (klasöre taşınabilir).
func (m Model) inRootlist(e libEntry) bool {
	return m.lib.root != nil && e.kind == kindPlaylist && m.lib.root.Index(e.uri) >= 0
}
