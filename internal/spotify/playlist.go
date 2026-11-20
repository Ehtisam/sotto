package spotify

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/url"
)

// Liste düzenleme: oluşturma, ad/açıklama/görünürlük, şarkı taşıma, kapak.

// PlaylistDetails, değiştirilecek alanlar; nil olanlar olduğu gibi kalır.
type PlaylistDetails struct {
	Name          *string `json:"name,omitempty"`
	Description   *string `json:"description,omitempty"`
	Public        *bool   `json:"public,omitempty"`
	Collaborative *bool   `json:"collaborative,omitempty"`
}

// CreatePlaylist, kullanıcının hesabında yeni bir liste açar.
func (c *Client) CreatePlaylist(ctx context.Context, userID string, d PlaylistDetails) (Playlist, error) {
	var p Playlist
	err := c.do(ctx, http.MethodPost, "me/playlists", d, &p)
	if (IsStatus(err, http.StatusNotFound) || IsStatus(err, http.StatusMethodNotAllowed)) && userID != "" {
		err = c.do(ctx, http.MethodPost, "users/"+url.PathEscape(userID)+"/playlists", d, &p)
	}
	return p, err
}

// UpdatePlaylist, listenin adını, açıklamasını, görünürlüğünü değiştirir.
func (c *Client) UpdatePlaylist(ctx context.Context, id string, d PlaylistDetails) error {
	return c.do(ctx, http.MethodPut, "playlists/"+url.PathEscape(id), d, nil)
}

// MovePlaylistItem, from'daki şarkıyı before'un önüne taşır (0'dan sayılır;
// sona taşımak için before = şarkı sayısı).
func (c *Client) MovePlaylistItem(ctx context.Context, id string, from, before int) error {
	body := map[string]int{"range_start": from, "insert_before": before, "range_length": 1}
	err := c.do(ctx, http.MethodPut, "playlists/"+url.PathEscape(id)+"/items", body, nil)
	if IsStatus(err, http.StatusNotFound) {
		err = c.do(ctx, http.MethodPut, "playlists/"+url.PathEscape(id)+"/tracks", body, nil)
	}
	return err
}

// UploadPlaylistCover, listenin kapağını JPEG ile değiştirir (en çok 256 KB;
// ugc-image-upload izni gerekir).
func (c *Client) UploadPlaylistCover(ctx context.Context, id string, jpeg []byte) error {
	body := []byte(base64.StdEncoding.EncodeToString(jpeg))
	return c.doRaw(ctx, http.MethodPut, "playlists/"+url.PathEscape(id)+"/images", "image/jpeg", body, nil)
}

// Playlist, tek bir listenin bilgisi.
func (c *Client) Playlist(ctx context.Context, id string) (Playlist, error) {
	var p Playlist
	err := c.get(ctx, "playlists/"+url.PathEscape(id)+"?fields=id,name,uri,description,collaborative,public,owner,images,items(total),tracks(total)", &p)
	return p, err
}

// PlaylistHas, şarkının listede olup olmadığını söyler (en çok 2000 şarkıya
// bakar; daha uzun listelerde bulamazsa var sayar).
func (c *Client) PlaylistHas(ctx context.Context, id, uri string) (bool, error) {
	for off := 0; off < 2000; off += 100 {
		p, err := c.PlaylistItems(ctx, id, off, 100)
		if err != nil {
			return false, err
		}
		for _, it := range p.Items {
			if t := it.T(); t != nil && t.URI == uri {
				return true, nil
			}
		}
		if p.Next == "" || len(p.Items) == 0 {
			return false, nil
		}
	}
	return true, nil
}
