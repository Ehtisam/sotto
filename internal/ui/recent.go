package ui

import (
	"encoding/json"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/spotify"
)

// Son aramalar: Spotify'daki gibi aranan sözler değil, aramadan açılan ya da
// çalınan öğeler. Arama kutusu boşken listelenir, x ile silinir. Yerelde
// %LOCALAPPDATA%\sotto\recent-searches.json dosyasında durur.

const maxRecent = 20

// savedSearch, bir son arama satırının dosyadaki hali.
type savedSearch struct {
	Kind  string         `json:"kind"`
	Name  string         `json:"name"`
	Sub   string         `json:"sub,omitempty"`
	URI   string         `json:"uri"`
	Track *spotify.Track `json:"track,omitempty"`
	Entry *savedEntry    `json:"entry,omitempty"`
}

type savedEntry struct {
	Kind          string         `json:"kind"`
	Key           string         `json:"key"`
	URI           string         `json:"uri"`
	Name          string         `json:"name"`
	Owner         string         `json:"owner,omitempty"`
	Desc          string         `json:"desc,omitempty"`
	Image         string         `json:"image,omitempty"`
	Total         int            `json:"total,omitempty"`
	Album         *spotify.Album `json:"album,omitempty"`
	OwnerID       string         `json:"owner_id,omitempty"`
	Collaborative bool           `json:"collaborative,omitempty"`
	Public        bool           `json:"public,omitempty"`
}

func saveItem(it searchItem) savedSearch {
	s := savedSearch{Kind: it.kind, Name: it.name, Sub: it.sub, URI: it.uri, Track: it.track}
	if e := it.entry; e.kind != "" {
		s.Entry = &savedEntry{Kind: e.kind, Key: e.key, URI: e.uri, Name: e.name, Owner: e.owner, Desc: e.desc,
			Image: e.image, Total: e.total, OwnerID: e.ownerID, Collaborative: e.collaborative, Public: e.public}
		if e.kind == kindAlbum {
			a := e.album
			s.Entry.Album = &a
		}
	}
	return s
}

func (s savedSearch) item() searchItem {
	it := searchItem{kind: s.Kind, name: s.Name, sub: s.Sub, uri: s.URI, track: s.Track}
	if e := s.Entry; e != nil {
		it.entry = libEntry{kind: e.Kind, key: e.Key, uri: e.URI, name: e.Name, owner: e.Owner, desc: e.Desc,
			image: e.Image, total: e.Total, ownerID: e.OwnerID, collaborative: e.Collaborative, public: e.Public}
		if e.Album != nil {
			it.entry.album = *e.Album
		}
	}
	if it.kind == "artist" && it.sub == "" {
		it.sub = T("recent.artist")
	}
	return it
}

func recentFile(localDir string) string { return filepath.Join(localDir, "recent-searches.json") }

// loadRecent, son aramaları dosyadan okur (yoksa boş).
func loadRecent(localDir string) []savedSearch {
	data, err := os.ReadFile(recentFile(localDir))
	if err != nil {
		return nil
	}
	var out []savedSearch
	if json.Unmarshal(data, &out) != nil {
		return nil
	}
	return out
}

// saveRecent, listeyi arka planda dosyaya yazar.
func (m Model) saveRecent() tea.Cmd {
	if m.cfg.LocalDir == "" {
		return nil
	}
	list, path := append([]savedSearch(nil), m.search.recent...), recentFile(m.cfg.LocalDir)
	return func() tea.Msg {
		data, err := json.MarshalIndent(list, "", "  ")
		if err == nil {
			_ = os.MkdirAll(filepath.Dir(path), 0o700)
			_ = os.WriteFile(path, data, 0o600)
		}
		return nil
	}
}

// rememberSearch, aramadan açılan öğeyi listenin başına koyar.
func (m Model) rememberSearch(it searchItem) (Model, tea.Cmd) {
	if it.uri == "" || !it.selectable() {
		return m, nil
	}
	list := []savedSearch{saveItem(it)}
	for _, s := range m.search.recent {
		if s.URI != it.uri && len(list) < maxRecent {
			list = append(list, s)
		}
	}
	m.search.recent = list
	return m, m.saveRecent()
}

// forgetSearch, son aramalardan birini siler.
func (m Model) forgetSearch(uri string) (Model, tea.Cmd) {
	var list []savedSearch
	for _, s := range m.search.recent {
		if s.URI != uri {
			list = append(list, s)
		}
	}
	m.search.recent = list
	return m, m.saveRecent()
}

// recentItems, kutu boşken gösterilen satırlar.
func (m Model) recentItems() []searchItem {
	if len(m.search.recent) == 0 {
		return nil
	}
	out := []searchItem{{kind: "header", section: "search.recent"}}
	for _, s := range m.search.recent {
		out = append(out, s.item())
	}
	return out
}
