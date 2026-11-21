package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/bridge"
	"github.com/Ehtisam/sotto/internal/spclient"
	"github.com/Ehtisam/sotto/internal/spotify"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// Kitaplık klasörleri (✱). Spotify kitaplığın sırasını ve klasörlerini iç
// "rootlist"te tutar (spclient/rootlist.go). Kitaplık'ın "Hepsi" görünümü bu
// sırayla çizilir: klasörler satır olarak görünür, enter içine girer, esc
// çıkar. f yeni klasör, shift+J/K kardeşleri arasında taşır; klasörde a: aç,
// adını değiştir, klasöre taşı, sil; listede a: klasöre taşı.
//
// Her değişiklik Spotify'dan taze rootlist okunarak, satırlar adreslerine
// göre bulunarak kurulur; kitaplık bu arada değiştiyse bir kez yeniden denenir.

const kindFolder = "folder"

type rootlistMsg struct {
	root   *spclient.Rootlist
	note   string // başarılı bir değişiklikten sonra gösterilecek
	follow string // imleç bu anahtara (liste ya da klasör kimliği) gitsin
	login  string
	err    error
	pins   []string // sabitlenenler (✱, en son sabitlenen önce); okunduysa pinsRead
	pinsOK bool
}

var errFolderSelf = errors.New("folder into itself")

// foldersOn: klasörler kullanılabilir (ekstralar açık ve rootlist okundu).
func (m Model) foldersOn() bool { return m.cfg.Extras.Enabled && m.lib.root != nil }

// loadRootlist, kitaplığın sırasını ve klasörlerini okur.
func (m Model) loadRootlist() tea.Cmd {
	if !m.cfg.Extras.Enabled || m.spState != spReady {
		return nil
	}
	return m.rootEdit(nil, "", "")
}

// rootEdit, rootlist'i okur; build verilmişse değişiklikleri kurup gönderir ve
// yeniden okur. build taze rootlist'le çağrılır; satırları adreslerine göre bulmalı.
func (m Model) rootEdit(build func(r spclient.Rootlist) ([]spclient.Op, error), note, follow string) tea.Cmd {
	br, sp, user := m.rt.bridge, m.sp, m.me.ID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		msg := rootlistMsg{follow: follow}
		if user == "" {
			me, err := sp.Me(ctx)
			if err != nil {
				msg.err = err
				return msg
			}
			user = me.ID
		}
		msg.login, msg.err = withBridge(ctx, br, func(tok string) error {
			for try := 0; ; try++ {
				r, err := spclient.GetRootlist(ctx, tok, user)
				if err != nil {
					return err
				}
				if build == nil {
					msg.root = &r
					// Sabitlenenler de (yalnız okuma); okunamazsa sessizce yok.
					if ps, err := spclient.Pins(ctx, tok, user); err == nil {
						for _, p := range ps {
							msg.pins = append(msg.pins, p.URI)
						}
						msg.pinsOK = true
					}
					return nil
				}
				ops, err := build(r)
				if err != nil {
					return err
				}
				if len(ops) == 0 { // yapılacak bir şey yok (örn. en üstte yukarı)
					msg.root = &r
					return nil
				}
				err = spclient.ChangeRootlist(ctx, tok, user, r.Revision, ops)
				if errors.Is(err, spclient.ErrConflict) && try == 0 {
					continue // kitaplık bu arada değişti: yeniden oku, yeniden kur
				}
				if err != nil {
					return err
				}
				if r, err = spclient.GetRootlist(ctx, tok, user); err == nil {
					msg.root = &r
				}
				msg.note = note
				return nil
			}
		})
		return msg
	}
}

func (m Model) updateRootlist(msg rootlistMsg) (Model, tea.Cmd) {
	switch {
	case msg.login != "":
		if msg.note != "" || msg.follow != "" {
			return m.say(T("folder.login")), nil
		}
		return m, nil // yalnızca okuma: sessizce düz listeye kal
	case msg.err != nil:
		if errors.Is(msg.err, errFolderSelf) {
			return m.say(T("folder.into")), nil
		}
		if msg.note == "" && msg.follow == "" && m.lib.root == nil {
			return m, nil // okunamadıysa düz liste
		}
		return m.say(msg.err.Error()), m.loadRootlist()
	}
	if msg.root != nil {
		m.lib.root = msg.root
	}
	if msg.pinsOK {
		m.lib.pins = msg.pins
	}
	if m.lib.folder != "" {
		if _, ok := m.lib.root.FolderByID(m.lib.folder); !ok {
			m.lib.folder = "" // içinde olunan klasör silindi
		}
	}
	if msg.follow != "" {
		m = m.libCursorTo(msg.follow)
	}
	m.clampLib()
	if msg.note != "" {
		m = m.note(msg.note)
	}
	return m, nil
}

// libCursorTo, imleci anahtarı key olan satıra koyar.
func (m Model) libCursorTo(key string) Model {
	for i, e := range m.libVisible() {
		if e.key == key {
			m.lib.cursor = i
			m.lib.cursor, m.lib.scroll = fitScroll(i, m.lib.scroll, len(m.libVisible()), m.listHeight())
		}
	}
	return m
}

// levelEntries, Kitaplık'ın "Hepsi" görünümünde bulunulan klasörün (""
// kök) satırları: rootlist sırasıyla listeler ve alt klasörler.
func (m Model) levelEntries(folder string) []libEntry {
	r := *m.lib.root
	byURI := map[string]libEntry{}
	var liked *libEntry
	for _, e := range m.lib.entries {
		if e.kind == kindLiked {
			e := e
			liked = &e
			continue
		}
		byURI[e.uri] = e
	}
	folders := r.Folders()
	startAt := map[int]spclient.Folder{}
	for _, f := range folders {
		startAt[f.Start] = f
	}
	lo, hi := -1, len(r.Items)
	if folder != "" {
		if f, ok := r.FolderByID(folder); ok {
			lo, hi = f.Start, f.End
		} else {
			folder = ""
		}
	}
	var out []libEntry
	if folder == "" && liked != nil {
		out = append(out, *liked)
	}
	seen := map[string]bool{}
	for i := lo + 1; i < hi; i++ {
		it := r.Items[i]
		if f, ok := startAt[i]; ok {
			out = append(out, libEntry{kind: kindFolder, key: f.ID, uri: it.URI, name: f.Name, total: m.folderCount(r, f)})
			i = f.End
			continue
		}
		if e, ok := byURI[it.URI]; ok {
			out = append(out, e)
			seen[it.URI] = true
		}
	}
	if folder == "" { // rootlist'te olmayan (yeni gelmiş) listeler sona
		for _, e := range m.lib.entries {
			if e.kind == kindPlaylist && !seen[e.uri] && r.Index(e.uri) < 0 {
				out = append(out, e)
			}
		}
	}
	return out
}

// folderCount, klasörün (alt klasörleri dahil) içindeki liste sayısı.
func (m Model) folderCount(r spclient.Rootlist, f spclient.Folder) int {
	n := 0
	for i := f.Start + 1; i < f.End; i++ {
		if strings.HasPrefix(r.Items[i].URI, "spotify:playlist:") {
			n++
		}
	}
	return n
}

// span, bir satırın (liste ya da tüm klasör) düz listedeki aralığı.
func span(r spclient.Rootlist, e libEntry) (start, n int, ok bool) {
	if e.kind == kindFolder {
		f, ok := r.FolderByID(e.key)
		if !ok {
			return 0, 0, false
		}
		return f.Start, f.End - f.Start + 1, true
	}
	i := r.Index(e.uri)
	return i, 1, i >= 0
}

// siblings, start satırıyla aynı düzeydeki satırların aralıkları (sırayla).
func siblings(r spclient.Rootlist, start int) [][2]int {
	parent := r.Parent(start)
	lo, hi := -1, len(r.Items)
	if parent != "" {
		f, _ := r.FolderByID(parent)
		lo, hi = f.Start, f.End
	}
	startAt := map[int]spclient.Folder{}
	for _, f := range r.Folders() {
		startAt[f.Start] = f
	}
	var out [][2]int
	for i := lo + 1; i < hi; i++ {
		if f, ok := startAt[i]; ok {
			out = append(out, [2]int{f.Start, f.End})
			i = f.End
			continue
		}
		if spclient.IsEnd(r.Items[i].URI) {
			continue
		}
		out = append(out, [2]int{i, i})
	}
	return out
}

// ---- işlemler ----

// moveInLibrary, imleçteki liste ya da klasörü kardeşleri arasında d yönünde taşır.
func (m Model) moveInLibrary(d int) (Model, tea.Cmd) {
	if !m.foldersOn() || m.lib.filter != libAll {
		return m, nil
	}
	vis := m.libVisible()
	if len(vis) == 0 {
		return m, nil
	}
	e := vis[min(m.lib.cursor, len(vis)-1)]
	if e.kind != kindPlaylist && e.kind != kindFolder {
		return m, nil
	}
	return m, m.rootEdit(func(r spclient.Rootlist) ([]spclient.Op, error) {
		s, n, ok := span(r, e)
		if !ok {
			return nil, spclient.ErrConflict
		}
		sib := siblings(r, s)
		k := -1
		for i, sp := range sib {
			if sp[0] == s {
				k = i
			}
		}
		j := k + d
		if k < 0 || j < 0 || j >= len(sib) {
			return nil, nil
		}
		to := sib[j][0] // yukarı: öncekinin önüne
		if d > 0 {
			to = sib[j][1] + 1 // aşağı: sonrakinin ardına
		}
		return []spclient.Op{spclient.Move(s, n, to)}, nil
	}, "", e.key)
}

// createFolder, bulunulan klasörün (ya da kökün) en üstüne yeni klasör açar.
func (m Model) createFolder(name string) tea.Cmd {
	parent, id := m.lib.folder, spclient.NewFolderID()
	return m.rootEdit(func(r spclient.Rootlist) ([]spclient.Op, error) {
		at := 0
		if parent != "" {
			if f, ok := r.FolderByID(parent); ok {
				at = f.Start + 1
			}
		}
		return []spclient.Op{spclient.AddAt(at, spclient.StartURI(id, name), spclient.EndURI(id))}, nil
	}, F("folder.created", name), id)
}

func (m Model) renameFolder(id, name string) tea.Cmd {
	return m.rootEdit(func(r spclient.Rootlist) ([]spclient.Op, error) {
		f, ok := r.FolderByID(id)
		if !ok {
			return nil, spclient.ErrConflict
		}
		return spclient.RenameOps(f, name), nil
	}, F("folder.renamed", name), id)
}

func (m Model) deleteFolder(e libEntry) (Model, tea.Cmd) {
	m.actions = actionMenu{}
	return m, m.rootEdit(func(r spclient.Rootlist) ([]spclient.Op, error) {
		f, ok := r.FolderByID(e.key)
		if !ok {
			return nil, spclient.ErrConflict
		}
		return spclient.DeleteFolderOps(f), nil
	}, F("folder.deleted", e.name), "")
}

// moveToFolder, listeyi ya da klasörü target klasörünün sonuna ("" ise
// kitaplığın en üstüne) taşır.
func (m Model) moveToFolder(e libEntry, target, targetName string) tea.Cmd {
	note := F("folder.moved", targetName)
	if target == "" {
		note = T("folder.movedtop")
	}
	return m.rootEdit(func(r spclient.Rootlist) ([]spclient.Op, error) {
		s, n, ok := span(r, e)
		if !ok {
			return nil, spclient.ErrConflict
		}
		to := 0
		if target != "" {
			f, ok := r.FolderByID(target)
			if !ok {
				return nil, spclient.ErrConflict
			}
			if s <= f.Start && f.End < s+n {
				return nil, errFolderSelf
			}
			to = f.End
		}
		return []spclient.Op{spclient.Move(s, n, to)}, nil
	}, note, e.key)
}

// moveNewIntoFolder, yeni açılan listeyi klasöre koyar (Spotify yeni listeyi
// kitaplığın başına ekler; birkaç kez bekleyip arar).
func moveNewIntoFolder(ctx context.Context, br *bridge.Bridge, sp *spotify.Client, user, uri, folder string) error {
	_, err := withBridge(ctx, br, func(tok string) error {
		for try := 0; try < 6; try++ {
			r, err := spclient.GetRootlist(ctx, tok, user)
			if err != nil {
				return err
			}
			i := r.Index(uri)
			f, ok := r.FolderByID(folder)
			if i < 0 || !ok {
				time.Sleep(500 * time.Millisecond)
				continue
			}
			err = spclient.ChangeRootlist(ctx, tok, user, r.Revision, []spclient.Op{spclient.Move(i, 1, f.End)})
			if errors.Is(err, spclient.ErrConflict) {
				continue
			}
			return err
		}
		return spclient.ErrConflict
	})
	return err
}

// ---- klasör seçici (Klasöre taşı…) ----

type folderChoice struct {
	id, name string
	depth    int
}

// folderChoices, e'nin taşınabileceği yerler: en üst ve bütün klasörler
// (klasörün kendisi ve içindekiler hariç).
func (m Model) folderChoices(e libEntry) []folderChoice {
	out := []folderChoice{{id: "", name: T("folder.top")}}
	r := *m.lib.root
	var self spclient.Folder
	isFolder := false
	if e.kind == kindFolder {
		self, isFolder = r.FolderByID(e.key)
	}
	for _, f := range r.Folders() {
		if isFolder && self.Start <= f.Start && f.End <= self.End {
			continue
		}
		out = append(out, folderChoice{id: f.ID, name: f.Name, depth: f.Depth + 1})
	}
	return out
}

func (m Model) folderPickView(w int) []string {
	a := m.actions
	rows := []string{theme.Title.Render(T("act.movefolder.title")), theme.Dim.Render(truncate(a.target.name, w)), "", ""}
	end := min(len(a.folders), a.plScroll+m.listHeight())
	for i := a.plScroll; i < end; i++ {
		c := a.folders[i]
		name := strings.Repeat("  ", max(c.depth-1, 0)) + theme.Icon.Folder + " " + c.name
		if c.id == "" {
			name = c.name
		}
		rows = append(rows, "  "+row(name, "", w-2, i == a.plCursor))
	}
	return rows
}

// folderTitle, Kitaplık başlığında bulunulan klasörün yolu.
func (m Model) folderTitle() string {
	if !m.foldersOn() || m.lib.folder == "" || m.lib.filter != libAll {
		return ""
	}
	r := *m.lib.root
	var parts []string
	for id := m.lib.folder; id != ""; {
		f, ok := r.FolderByID(id)
		if !ok {
			break
		}
		parts = append([]string{f.Name}, parts...)
		id = r.Parent(f.Start)
	}
	return strings.Join(parts, " › ")
}

// enterFolder / leaveFolder: klasöre gir, bir üst düzeye çık (imleç çıkılan klasörde).
func (m Model) enterFolder(id string) Model {
	m.lib.folder, m.lib.cursor, m.lib.scroll = id, 0, 0
	return m
}

func (m Model) leaveFolder() Model {
	cur := m.lib.folder
	f, ok := m.lib.root.FolderByID(cur)
	m.lib.folder = ""
	if ok {
		m.lib.folder = m.lib.root.Parent(f.Start)
	}
	m.lib.cursor, m.lib.scroll = 0, 0
	return m.libCursorTo(cur)
}

func folderInfo(n int) string { return fmt.Sprintf(T("lib.folder.count"), n) }
