package ui

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Uçtan uca: N ile liste aç, aramadan iki şarkı ekle, listede J ile taşı,
// e ile açıklamayı değiştir, son aramalarda x ile sil. Hiçbir şey çalınmaz.
// Deneme listesi sonunda silinir.
func TestDrivePlaylistFlow(t *testing.T) {
	d := drive(t)
	const name = "sotto drive test"
	var listID string
	t.Cleanup(func() {
		if listID != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := d.model().sp.Unsave(ctx, []string{"spotify:playlist:" + listID})
			t.Logf("deleted the test playlist: %v", err)
		}
	})
	search := func(q string) {
		d.do("/<wait:200>")
		if d.model().search.query != "" {
			d.do("<esc>")
		}
		d.do(q + "<wait:2500><enter><wait:300>")
	}
	findList := func() (int, bool) {
		for i, e := range d.model().lib.entries {
			if e.name == name {
				listID = e.key // silindikten sonra da temizlik için kalır
				return i, true
			}
		}
		return 0, false
	}

	// 1. N → ad → ctrl+s
	d.do("4<wait:2500>n<wait:300>" + name + "<ctrl+s><wait:4000>")
	if d.model().form.on {
		t.Fatalf("form still open: %q\n%s", d.model().form.err, d.screen())
	}
	if _, ok := findList(); !ok {
		t.Fatalf("new playlist not in the library\n%s", d.screen())
	}

	// 2. Aramadan iki şarkıyı listeye ekle.
	addSong := func(nth int) {
		search("tarkan")
		// nth'inci şarkıya git.
		for seen := 0; ; {
			it := d.model().search.items[d.model().search.cursor]
			if it.kind == "track" {
				if seen == nth {
					break
				}
				seen++
			}
			d.do("<down>")
		}
		it := d.model().search.items[d.model().search.cursor]
		if it.kind != "track" {
			t.Fatalf("expected a song, got %q %q", it.kind, it.name)
		}
		d.do("a<wait:300>p<wait:200>")
		a := d.model().actions
		idx := -1
		for i, e := range a.lists {
			if e.name == name {
				idx = i
			}
		}
		if !a.picking || idx < 0 {
			t.Fatalf("picker: picking %v idx %d\n%s", a.picking, idx, d.screen())
		}
		for range idx + 1 { // ilk satır "yeni liste"
			d.do("<down>")
		}
		d.do("<enter><wait:2000>")
		if !strings.Contains(d.model().flash, name) {
			t.Fatalf("add flash = %q", d.model().flash)
		}
		t.Logf("added %s", it.name)
	}
	addSong(0)
	addSong(1)

	// Kitaplık'ta deneme listesinin satırına git.
	gotoList := func() {
		d.do("<esc><esc>4<wait:2500>")
		vis := d.model().libVisible()
		pos := -1
		for i, e := range vis {
			if e.name == name {
				pos = i
			}
		}
		if pos < 0 {
			t.Fatalf("playlist not visible in Library\n%s", d.screen())
		}
		for d.model().lib.cursor > pos {
			d.do("<up>")
		}
		for d.model().lib.cursor < pos {
			d.do("<down>")
		}
	}

	// 3. Kitaplık'ta listeyi aç, ilk şarkıyı J ile aşağı taşı.
	gotoList()
	d.do("<enter><wait:2000>")
	tl := d.model().lib.open
	if tl == nil || len(tl.tracks) != 2 {
		t.Fatalf("open list: %+v\n%s", tl, d.screen())
	}
	first := tl.tracks[0].URI
	d.do("J<wait:2500>") // shift+J
	t.Log("\n" + d.screen())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	page, err := d.model().sp.PlaylistItems(ctx, listID, 0, 10)
	if err != nil || len(page.Items) != 2 || page.Items[1].T().URI != first || page.Items[0].T().URI == first {
		t.Fatalf("Spotify order after J: err %v items %d", err, len(page.Items))
	}

	// 4. e ile açıklama.
	d.do("<esc><wait:300>a<wait:600>")
	t.Log("\n" + d.screen())
	d.do("e<wait:300>")
	if !d.model().form.on || d.model().form.create {
		t.Fatalf("edit form not open\n%s", d.screen())
	}
	d.do("<down>made by the drive test<ctrl+s><wait:2500>")
	if d.model().form.on || !strings.Contains(d.model().flash, name) {
		t.Fatalf("edit: form %v flash %q", d.model().form.on, d.model().flash)
	}
	pl, err := d.model().sp.Playlist(ctx, listID)
	if err != nil || pl.Description != "made by the drive test" {
		t.Fatalf("description on Spotify = %q (%v)", pl.Description, err)
	}

	// 4b. Açık listede x ile bir şarkı çıkar.
	d.do("<enter><wait:2000>x<wait:2500>")
	if tl := d.model().lib.open; tl == nil || len(tl.tracks) != 1 {
		t.Fatalf("x in the open list: %+v\n%s", tl, d.screen())
	}
	page, err = d.model().sp.PlaylistItems(ctx, listID, 0, 10)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("Spotify after x: err %v items %d", err, len(page.Items))
	}
	t.Log("\n" + d.screen())

	// 4c. Kitaplık satırında x: onay açılır, esc vazgeçer.
	d.do("<esc><wait:300>x<wait:300>")
	if a := d.model().actions; !a.open || !a.confirm {
		t.Fatalf("x on the row did not ask\n%s", d.screen())
	}
	t.Log("\n" + d.screen())
	d.do("<esc><wait:300>")
	if _, ok := findList(); !ok || d.model().actions.open {
		t.Fatal("esc should keep the playlist and close the question")
	}

	// 5. Son aramalar: bir albüm aç, boş kutuda görünsün, x ile sil.
	search("tarkan")
	items := d.model().search.items
	album := -1
	for i, it := range items {
		if it.kind == "album" {
			album = i
			break
		}
	}
	if album < 0 {
		t.Fatalf("no album in results (query %q)\n%s", d.model().search.query, d.screen())
	}
	for d.model().search.cursor < album {
		d.do("<down>")
	}
	albumURI := items[album].uri
	d.do("<enter><wait:1500><esc><wait:200>/<wait:200>")
	if d.model().search.query != "" {
		d.do("<esc><wait:600>")
	}
	t.Log("\n" + d.screen())
	if len(d.model().search.recent) == 0 || d.model().search.recent[0].URI != albumURI {
		t.Fatalf("album not first in recent searches")
	}
	if s := d.screen(); !strings.Contains(s, "Recent searches") && !strings.Contains(s, "Son aramalar") {
		t.Fatalf("recent searches not shown")
	}
	n := len(d.model().search.recent)
	d.do("<down><wait:200>x<wait:300>")
	if len(d.model().search.recent) != n-1 {
		t.Fatalf("x did not remove: %d → %d", n, len(d.model().search.recent))
	}

	// 6. Menüden sil: a → d → enter.
	gotoList()
	d.do("a<wait:400>")
	hasDelete := false
	for _, it := range d.model().actions.items {
		if it.id == actDelete {
			hasDelete = true
		}
		if it.id == actUnsave || it.id == actSave {
			t.Error("own playlist should not offer follow/unfollow")
		}
	}
	if !hasDelete {
		t.Fatalf("no delete in the menu\n%s", d.screen())
	}
	d.do("d<wait:300><enter><wait:2500>")
	if _, ok := findList(); ok {
		t.Fatalf("still in Library after delete\n%s", d.screen())
	}
	in, err := d.model().sp.InLibrary(ctx, []string{"spotify:playlist:" + listID})
	if err != nil || len(in) != 1 || in[0] {
		t.Fatalf("still on Spotify after delete: %v %v", in, err)
	}
	t.Logf("deleted: %q", d.model().flash)
}
