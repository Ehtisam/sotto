package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Ehtisam/sotto/internal/spclient"
)

// Klasörler, uçtan uca ve yalnızca kendi açtığı deneme klasörü ile deneme
// listesiyle: f ile aç → içine gir, n ile liste aç (klasöre girmeli) →
// adını değiştir → listeyi en üste çıkar ve geri koy → shift+K/J ile klasörü
// taşı → x ile klasörü sil (liste kalmalı) → temizlik.
func TestDriveFolders(t *testing.T) {
	d := drive(t)
	const folder, folder2, list = "sotto drive folder", "sotto drive folder 2", "sotto drive folder list"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	user := d.model().me.ID
	var folderID, listURI string
	root := func() spclient.Rootlist {
		t.Helper()
		tok, err := d.model().rt.bridge.Token(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if user == "" {
			user = d.model().me.ID
		}
		r, err := spclient.GetRootlist(ctx, tok, user)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	t.Cleanup(func() {
		if listURI != "" {
			t.Logf("cleanup list: %v", d.model().sp.Unsave(context.Background(), []string{listURI}))
		}
		if folderID == "" {
			return
		}
		r := root()
		if f, ok := r.FolderByID(folderID); ok {
			tok, _ := d.model().rt.bridge.Token(ctx)
			t.Logf("cleanup folder: %v", spclient.ChangeRootlist(ctx, tok, user, r.Revision, spclient.DeleteFolderOps(f)))
		}
	})
	cursorName := func() string {
		vis := d.model().libVisible()
		if len(vis) == 0 {
			return ""
		}
		return vis[min(d.model().lib.cursor, len(vis)-1)].name
	}

	// Kitaplık, "Hepsi", klasörler okunmuş olmalı.
	d.do("4<wait:3500>")
	if d.model().lib.root == nil {
		t.Fatalf("folders not loaded\n%s", d.screen())
	}

	// 1. f → yeni klasör
	d.do("f<wait:300>" + folder + "<ctrl+s><wait:4000>")
	if d.model().form.on {
		t.Fatalf("folder form still open: %q", d.model().form.err)
	}
	for _, f := range d.model().lib.root.Folders() {
		if f.Name == folder {
			folderID = f.ID
		}
	}
	if folderID == "" || cursorName() != folder {
		t.Fatalf("folder not made or cursor not on it (cursor %q)\n%s", cursorName(), d.screen())
	}
	t.Log("\n" + d.screen())

	// 2. Klasöre gir, n ile içinde liste aç.
	d.do("<enter><wait:400>n<wait:300>" + list + "<ctrl+s><wait:7000>")
	if d.model().form.on {
		t.Fatalf("list form still open: %q", d.model().form.err)
	}
	for _, e := range d.model().lib.entries {
		if e.name == list {
			listURI = e.uri
		}
	}
	r := root()
	if listURI == "" || r.Parent(r.Index(listURI)) != folderID {
		t.Fatalf("new list is not in the folder (uri %q)\n%s", listURI, d.screen())
	}
	t.Log("\n" + d.screen())

	// 3. esc ile çık (imleç klasörde), a → e ile adını değiştir.
	d.do("<esc><wait:400>")
	if d.model().lib.folder != "" || cursorName() != folder {
		t.Fatalf("esc did not leave the folder (cursor %q)", cursorName())
	}
	d.do("a<wait:300>e<wait:300><ctrl+u>" + folder2 + "<ctrl+s><wait:4000>")
	if f, ok := root().FolderByID(folderID); !ok || f.Name != folder2 {
		t.Fatalf("rename did not take: %+v", f)
	}

	// 4. Klasöre gir, listede a → m → en üst; sonra geri.
	d.do("<enter><wait:400>a<wait:300>m<wait:300>")
	if !d.model().actions.folderPick {
		t.Fatalf("no folder picker\n%s", d.screen())
	}
	t.Log("\n" + d.screen())
	d.do("<enter><wait:4000>") // ilk satır: kitaplığın en üstü
	r = root()
	if p := r.Parent(r.Index(listURI)); p != "" {
		t.Fatalf("list still in a folder: %q", p)
	}
	// Liste artık kökte; klasörden çık, listeyi bul, klasöre geri taşı.
	d.do("<esc><wait:300>")
	for i, e := range d.model().libVisible() {
		if e.uri == listURI {
			for d.model().lib.cursor > i {
				d.do("<up>")
			}
			for d.model().lib.cursor < i {
				d.do("<down>")
			}
		}
	}
	d.do("a<wait:300>m<wait:300>")
	idx := -1
	for i, c := range d.model().actions.folders {
		if c.id == folderID {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatalf("test folder not in the picker")
	}
	for range idx {
		d.do("<down>")
	}
	d.do("<enter><wait:4000>")
	r = root()
	if p := r.Parent(r.Index(listURI)); p != folderID {
		t.Fatalf("list not back in the folder: %q", p)
	}

	// 5. shift+J / shift+K: klasörü aşağı, sonra yukarı taşı.
	for i, e := range d.model().libVisible() {
		if e.key == folderID {
			for d.model().lib.cursor > i {
				d.do("<up>")
			}
			for d.model().lib.cursor < i {
				d.do("<down>")
			}
		}
	}
	before, _ := root().FolderByID(folderID)
	d.do("J<wait:4000>")
	after, _ := root().FolderByID(folderID)
	if after.Start <= before.Start || cursorName() != folder2 {
		t.Fatalf("shift+J did not move the folder down: %d → %d (cursor %q)", before.Start, after.Start, cursorName())
	}
	d.do("K<wait:4000>")
	if back, _ := root().FolderByID(folderID); back.Start != before.Start {
		t.Fatalf("shift+K did not move it back: %d → %d", after.Start, back.Start)
	}

	// 6. x → enter: klasör silinir, içindeki liste kalır.
	d.do("x<wait:300>")
	if !d.model().actions.confirm {
		t.Fatalf("x did not ask\n%s", d.screen())
	}
	t.Log("\n" + d.screen())
	d.do("<enter><wait:4000>")
	r = root()
	if _, ok := r.FolderByID(folderID); ok {
		t.Fatal("folder still there")
	}
	if r.Index(listURI) < 0 {
		t.Fatal("deleting the folder removed the list in it")
	}
	if !strings.Contains(d.model().flash, folder2) {
		t.Errorf("flash = %q", d.model().flash)
	}
	folderID = ""
}
