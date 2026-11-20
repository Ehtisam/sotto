package spotify

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// SearchLimit, geliştirici uygulamalarında arama başına en çok sonuç (Spotify'ın sınırı).
const SearchLimit = 10

type FullArtist struct {
	Artist
	Images []Image `json:"images"`
}

type SimpleAlbum struct {
	Album
	TotalTracks int    `json:"total_tracks"`
	AlbumType   string `json:"album_type"`
}

type Show struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	URI         string  `json:"uri"`
	Images      []Image `json:"images"`
	Description string  `json:"description"`
	// Spotify 2026'da yayıncı (publisher) alanını Web API'den kaldırdı.
	TotalEpisodes int `json:"total_episodes"`
}

// SearchResult, /search yanıtı. Spotify bazı türlerde boş (null) öğe döndürebiliyor.
type SearchResult struct {
	Tracks    Paging[*Track]       `json:"tracks"`
	Artists   Paging[*FullArtist]  `json:"artists"`
	Albums    Paging[*SimpleAlbum] `json:"albums"`
	Playlists Paging[*Playlist]    `json:"playlists"`
	Shows     Paging[*Show]        `json:"shows"`
	Episodes  Paging[*Track]       `json:"episodes"`
}

// Search, verilen türlerde arar (track, artist, album, playlist, show).
func (c *Client) Search(ctx context.Context, query string, types []string, offset int) (SearchResult, error) {
	var r SearchResult
	q := fmt.Sprintf("search?q=%s&type=%s&limit=%d&offset=%d",
		url.QueryEscape(query), strings.Join(types, ","), SearchLimit, offset)
	err := c.get(ctx, q, &r)
	if err == nil {
		for _, t := range r.Tracks.Items {
			if t != nil {
				c.remember(*t)
			}
		}
	}
	return r, err
}

// ShowEpisodes, podcast'in bölümleri (en yeniden eskiye).
func (c *Client) ShowEpisodes(ctx context.Context, id string, offset, limit int) (Paging[*Track], error) {
	var p Paging[*Track]
	err := c.get(ctx, fmt.Sprintf("shows/%s/episodes?offset=%d&limit=%d", url.PathEscape(id), offset, limit), &p)
	for _, e := range p.Items {
		if e != nil {
			e.Type = "episode"
		}
	}
	return p, err
}

// AlbumTracks, albümün şarkıları. Yanıtta albüm bilgisi olmadığından verilen albüm eklenir.
func (c *Client) AlbumTracks(ctx context.Context, album Album, offset, limit int) (Paging[Track], error) {
	var p Paging[Track]
	err := c.get(ctx, fmt.Sprintf("albums/%s/tracks?offset=%d&limit=%d", url.PathEscape(album.ID), offset, limit), &p)
	if err == nil {
		for i := range p.Items {
			p.Items[i].Album = album
		}
		c.remember(p.Items...)
	}
	return p, err
}

// AlbumInfo, bir albümün bilgisi ve şarkı sayısı (bağlantıdan açılırken).
func (c *Client) AlbumInfo(ctx context.Context, id string) (Album, int, error) {
	var a struct {
		Album
		TotalTracks int `json:"total_tracks"`
	}
	err := c.get(ctx, "albums/"+url.PathEscape(id), &a)
	return a.Album, a.TotalTracks, err
}

// Show, bir podcast'in bilgisi.
func (c *Client) Show(ctx context.Context, id string) (Show, error) {
	var s Show
	err := c.get(ctx, "shows/"+url.PathEscape(id), &s)
	return s, err
}
