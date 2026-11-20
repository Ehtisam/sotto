package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/soloist"
	"github.com/Ehtisam/sotto/internal/spotify"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// Eylem menüsü (a): seçili satır için, seçili bir şey yoksa çalan şarkı
// için. Sıraya ekle, beğen, listeye ekle, albüme git, bağlantıyı kopyala…

type actID int

const (
	actQueue actID = iota
	actLike
	actUnlike
	actAddList
	actRemoveList
	actAlbum
	actArtist
	actPlay
	actOpen
	actSave
	actUnsave
	actLink
)

type menuItem struct {
	id    actID
	key   string // menüdeki kısayol
	label string
}

type actionMenu struct {
	open   bool
	target searchItem
	from   *libEntry // şarkı bir listenin içinden seçildiyse o liste
	saved  int       // -1 bilinmiyor, 0 kitaplıkta değil, 1 kitaplıkta
	items  []menuItem
	cursor int

	picking  bool // "listeye ekle": liste seçiliyor
	lists    []libEntry
	plCursor int
	plScroll int
}

type savedStateMsg struct {
	uri   string
	saved bool
}

type actionDoneMsg struct {
	note    string
	err     error
	removed string // listeden çıkarılan şarkı
	fromKey string
}

// ---- hedef ----

// actionTarget, menünün neyin için açılacağı: içerikte seçili satır, yoksa çalan şarkı.
func (m Model) actionTarget() (searchItem, *libEntry, bool) {
	if m.focus == focusContent {
		var tl *trackList
		switch m.page {
		case pageLibrary:
			tl = m.lib.open
			if tl == nil && len(m.lib.entries) > 0 {
				e := m.lib.entries[m.lib.cursor]
				return entryItem(e), nil, true
			}
		case pageSearch:
			tl = m.search.open
			if tl == nil && !m.search.typing && len(m.search.items) > 0 {
				return m.search.items[m.search.cursor], nil, true
			}
		case pageHome:
			tl = m.home.open
			if tl == nil && len(m.home.items) > 0 {
				return m.home.items[m.home.cursor], nil, true
			}
		case pageQueue:
			if items := m.queueItems(); len(items) > 0 {
				return items[m.queueCursor(items)], nil, true
			}
		}
		if tl != nil && len(tl.tracks) > 0 {
			t := tl.tracks[tl.cursor]
			from := tl.entry
			return trackItem(t), &from, true
		}
	}
	it := m.state.Item
	if it == nil || it.URI == "" {
		return searchItem{}, nil, false
	}
	_, artists, _ := m.describe(it)
	item := trackItem(m.trackFromEntity(it))
	item.sub = artists
	return item, nil, true
}

func trackItem(t spotify.Track) searchItem {
	tt := t
	return searchItem{kind: trackKind(t), name: t.Name, sub: t.ArtistNames(), uri: t.URI, track: &tt}
}

func entryItem(e libEntry) searchItem {
	kind := e.kind
	return searchItem{kind: kind, name: e.name, sub: e.owner, uri: e.uri, entry: e}
}

// ---- açma ----

func (m Model) openActions() (Model, tea.Cmd) {
	if m.spState != spReady {
		return m.say(T("flash.noaccount")), nil
	}
	target, from, ok := m.actionTarget()
	if !ok {
		return m.note(T("act.nothing")), nil
	}
	m.actions = actionMenu{open: true, target: target, from: from, saved: -1}
	m.actions.items = m.menuItems()
	if target.uri == "" || target.kind == "liked" {
		return m, nil
	}
	sp, uri := m.sp, target.uri
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		res, err := sp.InLibrary(ctx, []string{uri})
		if err != nil || len(res) == 0 {
			return nil
		}
		return savedStateMsg{uri, res[0]}
	}
}

// menuItems, hedefin türüne göre eylemler.
func (m Model) menuItems() []menuItem {
	a := m.actions
	t := a.target
	var out []menuItem
	add := func(id actID, key, label string) { out = append(out, menuItem{id, key, T(label)}) }
	save := func(saveLabel, unsaveLabel string) {
		switch a.saved {
		case 1:
			add(actUnsave, "s", unsaveLabel)
		case 0:
			add(actSave, "s", saveLabel)
		}
	}
	switch t.kind {
	case "track":
		add(actQueue, "q", "act.queue")
		switch a.saved {
		case 1:
			add(actUnlike, "l", "act.unlike")
		case 0:
			add(actLike, "l", "act.like")
		}
		add(actAddList, "p", "act.addlist")
		if a.from != nil && m.editable(*a.from) {
			add(actRemoveList, "x", "act.removelist")
		}
		if t.track != nil && t.track.Album.URI != "" {
			add(actAlbum, "b", "act.album")
		}
		if t.track != nil && len(t.track.Artists) > 0 && t.track.Artists[0].URI != "" {
			add(actArtist, "r", "act.artist")
		}
	case "episode":
		add(actQueue, "q", "act.queue")
	case "album", "playlist", "show", "liked":
		add(actOpen, "o", "act.open")
		add(actPlay, "p", "act.play")
		if t.kind == "playlist" {
			save("act.follow", "act.unfollow")
		} else if t.kind != "liked" {
			save("act.save", "act.unsave")
		}
	case "artist":
		add(actPlay, "p", "act.play")
		save("act.follow", "act.unfollow")
	}
	if t.uri != "" && t.kind != "liked" {
		add(actLink, "c", "act.link")
	}
	return out
}

// editable: kullanıcının kendi ya da ortak listesi.
func (m Model) editable(e libEntry) bool {
	return e.kind == kindPlaylist && (e.collaborative || (e.ownerID != "" && e.ownerID == m.me.ID))
}

// ---- tuşlar ----

func (m Model) actionsKey(k string) (Model, tea.Cmd) {
	a := &m.actions
	if a.picking {
		h := m.listHeight()
		switch k {
		case "up", "k":
			a.plCursor = max(0, a.plCursor-1)
		case "down", "j":
			a.plCursor = min(len(a.lists)-1, a.plCursor+1)
		case "enter", "right":
			if len(a.lists) == 0 {
				return m, nil
			}
			pl := a.lists[a.plCursor]
			uri := a.target.uri
			m.actions = actionMenu{}
			return m, m.spotifyAction(func(ctx context.Context, sp *spotify.Client) error {
				return sp.AddToPlaylist(ctx, pl.key, []string{uri})
			}, F("act.added", pl.name))
		case "esc", "left", "backspace":
			a.picking = false
		}
		a.plCursor, a.plScroll = fitScroll(a.plCursor, a.plScroll, len(a.lists), h)
		return m, nil
	}

	switch k {
	case "up", "k":
		a.cursor = max(0, a.cursor-1)
		return m, nil
	case "down", "j":
		a.cursor = min(len(a.items)-1, a.cursor+1)
		return m, nil
	case "esc", "left", "backspace", "a":
		m.actions = actionMenu{}
		return m, nil
	case "enter", "right":
		if len(a.items) == 0 {
			return m, nil
		}
		return m.runAction(a.items[a.cursor].id)
	}
	for _, it := range a.items {
		if it.key == k {
			return m.runAction(it.id)
		}
	}
	return m, nil
}

func (m Model) runAction(id actID) (Model, tea.Cmd) {
	t, from := m.actions.target, m.actions.from
	uri := t.uri
	if id == actAddList {
		m.actions.picking = true
		m.actions.lists = nil
		for _, e := range m.lib.entries {
			if m.editable(e) {
				m.actions.lists = append(m.actions.lists, e)
			}
		}
		m.actions.plCursor, m.actions.plScroll = 0, 0
		return m, nil
	}
	m.actions = actionMenu{}

	switch id {
	case actQueue:
		if m.soloistActive() && t.kind == "track" {
			return m, m.soloistAction(func(ctx context.Context, c *soloist.Client) error { return c.AddToQueue(ctx, uri) }, T("act.queued"))
		}
		return m, m.spotifyAction(func(ctx context.Context, sp *spotify.Client) error { return sp.AddToQueue(ctx, uri) }, T("act.queued"))
	case actLike:
		return m, m.spotifyAction(func(ctx context.Context, sp *spotify.Client) error { return sp.Save(ctx, []string{uri}) }, T("act.liked"))
	case actUnlike:
		return m, m.spotifyAction(func(ctx context.Context, sp *spotify.Client) error { return sp.Unsave(ctx, []string{uri}) }, T("act.unliked"))
	case actSave:
		return m, m.spotifyAction(func(ctx context.Context, sp *spotify.Client) error { return sp.Save(ctx, []string{uri}) }, T("act.saved"))
	case actUnsave:
		return m, m.spotifyAction(func(ctx context.Context, sp *spotify.Client) error { return sp.Unsave(ctx, []string{uri}) }, T("act.unsaved"))
	case actRemoveList:
		key, name := from.key, from.name
		sp := m.sp
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := sp.RemoveFromPlaylist(ctx, key, []string{uri})
			return actionDoneMsg{note: F("act.removed", name), err: err, removed: uri, fromKey: key}
		}
	case actAlbum:
		a := t.track.Album
		e := libEntry{kind: kindAlbum, key: a.ID, uri: a.URI, name: a.Name, owner: artistNames(a.Artists),
			image: pickImage(a.Images), album: a}
		return m.goTo(e)
	case actArtist:
		return m, m.playContext(t.track.Artists[0].URI, "")
	case actPlay:
		if t.kind == "liked" {
			tl, cmd := m.openList(t.entry)
			return m.showList(tl), cmd
		}
		return m, m.playContext(uri, "")
	case actOpen:
		return m.goTo(t.entry)
	case actLink:
		return m.note(T("act.copied")), tea.SetClipboard(spotify.WebLink(uri))
	}
	return m, nil
}

// goTo, bir albümü, listeyi ya da podcast'i açar: bulunulan sayfa açabiliyorsa
// orada, açamıyorsa Ara sayfasında.
func (m Model) goTo(e libEntry) (Model, tea.Cmd) {
	tl, cmd := m.openList(e)
	return m.showList(tl), cmd
}

func (m Model) showList(tl *trackList) Model {
	switch m.page {
	case pageLibrary:
		m.lib.open = tl
	case pageHome:
		m.home.open = tl
	default:
		m.page = pageSearch
		m.search.open, m.search.typing = tl, false
	}
	m.focus = focusContent
	return m
}

func (m Model) spotifyAction(f func(context.Context, *spotify.Client) error, note string) tea.Cmd {
	sp := m.sp
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return actionDoneMsg{note: note, err: f(ctx, sp)}
	}
}

func (m Model) soloistAction(f func(context.Context, *soloist.Client) error, note string) tea.Cmd {
	c := m.rt.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return actionDoneMsg{note: note, err: f(ctx, c)}
	}
}

func (m Model) updateActions(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case savedStateMsg:
		if m.actions.open && m.actions.target.uri == msg.uri {
			m.actions.saved = 0
			if msg.saved {
				m.actions.saved = 1
			}
			cur := ""
			if m.actions.cursor < len(m.actions.items) {
				cur = m.actions.items[m.actions.cursor].key
			}
			m.actions.items = m.menuItems()
			for i, it := range m.actions.items {
				if it.key == cur {
					m.actions.cursor = i
				}
			}
		}
	case actionDoneMsg:
		if msg.err != nil {
			if spotify.IsStatus(msg.err, 404) {
				return m.say(T("flash.nodevice")), nil
			}
			return m.say(msg.err.Error()), nil
		}
		if msg.removed != "" {
			for _, tl := range []*trackList{m.lib.open, m.search.open, m.home.open} {
				if tl != nil && tl.entry.key == msg.fromKey {
					tl.remove(msg.removed)
				}
			}
		}
		return m.note(msg.note), func() tea.Msg { return pollSoonMsg{} }
	}
	return m, nil
}

// ---- çizim ----

func (m Model) actionsView(w int) []string {
	a := m.actions
	if a.picking {
		rows := []string{theme.Title.Render(T("act.addlist.title")), theme.Dim.Render(truncate(a.target.name, w)), "", ""}
		if len(a.lists) == 0 {
			return append(rows, theme.Dim.Render(T("act.nolists")))
		}
		end := min(len(a.lists), a.plScroll+m.listHeight())
		for i := a.plScroll; i < end; i++ {
			e := a.lists[i]
			rows = append(rows, "  "+row(e.name, F("lib.songs", e.total), w-2, i == a.plCursor))
		}
		return rows
	}
	sub := a.target.sub
	if sub == "" {
		sub = T("kind." + a.target.kind)
	}
	rows := []string{theme.Title.Render(truncate(a.target.name, w)), theme.Dim.Render(truncate(sub, w)), "", ""}
	rw := min(w-2, 46) // kısa bir liste; bütün genişliğe yayılmasın
	for i, it := range a.items {
		rows = append(rows, "  "+row(it.label, it.key, rw, i == a.cursor))
	}
	if a.saved == -1 && a.target.kind != "liked" {
		rows = append(rows, "", theme.Faint.Render(T("lib.loading")))
	}
	return rows
}
