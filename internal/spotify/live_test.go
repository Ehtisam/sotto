package spotify

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ehtisam/sotto/internal/config"
)

// TestLivePlaylistEdit, gerçek hesapta gizli bir deneme listesi açar,
// düzenler ve siler. Başka listeye dokunmaz.
// Çalıştırmak için: SOTTO_LIVE=1 go test ./internal/spotify -run Live -v
func TestLivePlaylistEdit(t *testing.T) {
	if os.Getenv("SOTTO_LIVE") == "" {
		t.Skip("SOTTO_LIVE not set")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	c := NewClient(cfg.Spotify.ClientID, filepath.Join(cfg.LocalDir, "spotify-token.json"))
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	me, err := c.Me(ctx)
	if err != nil {
		t.Fatal(err)
	}
	name, desc, private := "sotto test", "temporary, made by sotto's tests", false
	p, err := c.CreatePlaylist(ctx, me.ID, PlaylistDetails{Name: &name, Description: &desc, Public: &private})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Logf("created %s %q public=%v", p.ID, p.Name, p.Public != nil && *p.Public)
	defer func() {
		err := c.Unsave(context.Background(), []string{p.URI})
		t.Logf("removed: %v", err)
	}()

	newName, newDesc := "sotto test 2", "edited"
	if err := c.UpdatePlaylist(ctx, p.ID, PlaylistDetails{Name: &newName, Description: &newDesc}); err != nil {
		t.Fatalf("update: %v", err)
	}
	collab := true
	err = c.UpdatePlaylist(ctx, p.ID, PlaylistDetails{Collaborative: &collab, Public: &private})
	t.Logf("collaborative: %v", err)

	uris := []string{"spotify:track:0gOBDLTWIG5f3YgShZHQWM", "spotify:track:0Qng6YqwJ3TkeQjYWLC07X", "spotify:track:5ukniNohEgaerrbHbVlBLf"}
	if err := c.AddToPlaylist(ctx, p.ID, uris); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := c.MovePlaylistItem(ctx, p.ID, 0, 3); err != nil {
		t.Fatalf("move: %v", err)
	}
	items, err := c.PlaylistItems(ctx, p.ID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, it := range items.Items {
		got = append(got, it.T().URI)
	}
	t.Logf("order after move: %v", got)
	if len(got) != 3 || got[2] != uris[0] {
		t.Errorf("move did not take: %v", got)
	}

	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for i := range img.Pix {
		img.Pix[i] = 0x80
	}
	img.Set(1, 1, color.White)
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, nil)
	err = c.UploadPlaylistCover(ctx, p.ID, buf.Bytes())
	t.Logf("cover upload: %v", err)

	back, err := c.Playlist(ctx, p.ID)
	t.Logf("read back: %q %q collab=%v err=%v", back.Name, back.Description, back.Collaborative, err)
}
