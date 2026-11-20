package spclient

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ehtisam/sotto/internal/bridge"
	"github.com/Ehtisam/sotto/internal/config"
	"github.com/Ehtisam/sotto/internal/spotify"
)

// liveAccount, gerçek hesabın bridge jetonu ve kullanıcı kimliği.
func liveAccount(t *testing.T) (ctx context.Context, tok, user string) {
	t.Helper()
	if os.Getenv("SOTTO_LIVE") == "" {
		t.Skip("SOTTO_LIVE not set")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	me, err := spotify.NewClient(cfg.Spotify.ClientID, cfg.TokenFile()).Me(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tok, err = bridge.New(filepath.Join(cfg.LocalDir, "bridge")).Token(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, tok, me.ID
}

// TestLiveRootlistRead yalnızca okur.
// SOTTO_LIVE=1 go test ./internal/spclient -run LiveRootlistRead -v
func TestLiveRootlistRead(t *testing.T) {
	ctx, tok, user := liveAccount(t)
	r, err := GetRootlist(ctx, tok, user)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("revision %q, %d rows", r.Revision, len(r.Items))
	for _, f := range r.Folders() {
		t.Logf("folder %q id=%s rows %d..%d depth %d", f.Name, f.ID, f.Start, f.End, f.Depth)
	}
	for i, it := range r.Items {
		if i < 8 {
			t.Logf("%3d %s ts=%s", i, it.URI, it.Timestamp)
		}
	}
}

// TestLiveRootlistFolders, yalnızca kendi açtığı deneme klasörüne ve deneme
// listesine dokunur: klasör aç → adını değiştir → listeyi içine taşı →
// dışarı al → klasörü sil → listeyi sil. Her işlemden önce hedef satırın
// beklenen satır olduğu doğrulanır.
func TestLiveRootlistFolders(t *testing.T) {
	ctx, tok, user := liveAccount(t)
	cfg, _ := config.Load()
	sp := spotify.NewClient(cfg.Spotify.ClientID, cfg.TokenFile())
	read := func() Rootlist {
		t.Helper()
		r, err := GetRootlist(ctx, tok, user)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	must := func(r Rootlist, i int, uri string) {
		t.Helper()
		if i < 0 || i >= len(r.Items) || r.Items[i].URI != uri {
			t.Fatalf("row %d is not %s; refusing to touch it", i, uri)
		}
	}

	id := NewFolderID()
	r := read()
	start, end := StartURI(id, "sotto test folder"), EndURI(id)
	if err := ChangeRootlist(ctx, tok, user, r.Revision, []Op{AddAt(0, start, end)}); err != nil {
		t.Fatalf("create folder: %v", err)
	}
	r = read()
	f, ok := r.FolderByID(id)
	if !ok {
		t.Fatal("folder not there after create")
	}
	t.Logf("created %q rows %d..%d", f.Name, f.Start, f.End)
	defer func() { // ne olursa olsun klasörü kaldır
		r := read()
		if f, ok := r.FolderByID(id); ok {
			must(r, f.Start, r.Items[f.Start].URI)
			err := ChangeRootlist(ctx, tok, user, r.Revision, DeleteFolderOps(f))
			t.Logf("cleanup folder: %v", err)
		}
	}()

	must(r, f.Start, start)
	if err := ChangeRootlist(ctx, tok, user, r.Revision, RenameOps(f, "sotto test folder 2")); err != nil {
		t.Fatalf("rename: %v", err)
	}
	r = read()
	if f, _ = r.FolderByID(id); f.Name != "sotto test folder 2" {
		t.Fatalf("rename did not take: %q", f.Name)
	}

	name, private := "sotto folder test list", false
	pl, err := sp.CreatePlaylist(ctx, user, spotify.PlaylistDetails{Name: &name, Public: &private})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { t.Logf("cleanup list: %v", sp.Unsave(context.Background(), []string{pl.URI})) }()
	time.Sleep(time.Second)
	r = read()
	li := r.Index(pl.URI)
	f, _ = r.FolderByID(id)
	must(r, li, pl.URI)
	if err := ChangeRootlist(ctx, tok, user, r.Revision, []Op{Move(li, 1, f.End)}); err != nil {
		t.Fatalf("move in: %v", err)
	}
	r = read()
	if p := r.Parent(r.Index(pl.URI)); p != id {
		t.Fatalf("list not in the folder (parent %q)", p)
	}
	t.Log("moved into the folder")

	li = r.Index(pl.URI)
	f, _ = r.FolderByID(id)
	if err := ChangeRootlist(ctx, tok, user, r.Revision, []Op{Move(li, 1, f.End+1)}); err != nil {
		t.Fatalf("move out: %v", err)
	}
	r = read()
	if p := r.Parent(r.Index(pl.URI)); p != "" {
		t.Fatalf("list still in a folder (%q)", p)
	}
	t.Log("moved out")

	// Klasör silinince içindeki liste kalmalı: önce geri koy, sonra klasörü sil.
	li = r.Index(pl.URI)
	f, _ = r.FolderByID(id)
	if err := ChangeRootlist(ctx, tok, user, r.Revision, []Op{Move(li, 1, f.End)}); err != nil {
		t.Fatal(err)
	}
	r = read()
	f, _ = r.FolderByID(id)
	must(r, f.Start, StartURI(id, "sotto test folder 2"))
	must(r, f.End, end)
	if err := ChangeRootlist(ctx, tok, user, r.Revision, DeleteFolderOps(f)); err != nil {
		t.Fatalf("delete folder: %v", err)
	}
	r = read()
	if _, ok := r.FolderByID(id); ok {
		t.Fatal("folder still there")
	}
	if r.Index(pl.URI) < 0 {
		t.Fatal("deleting the folder removed the list inside it")
	}
	t.Log("folder deleted, list kept")
}
