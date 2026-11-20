package spotify

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Spotify 2026'da beğenme, kaydetme ve takip etmeyi tek uçta topladı:
// /me/library şarkı, albüm, liste, sanatçı ve podcast adreslerini (URI) alır.

func uriQuery(uris []string) string {
	return "uris=" + url.QueryEscape(strings.Join(uris, ","))
}

// InLibrary, her adresin kitaplıkta (beğenilen, kaydedilen, takip edilen) olup olmadığı.
func (c *Client) InLibrary(ctx context.Context, uris []string) ([]bool, error) {
	var out []bool
	err := c.get(ctx, "me/library/contains?"+uriQuery(uris), &out)
	return out, err
}

// Save, kitaplığa ekler: şarkıyı beğenir, albümü ya da podcast'i kaydeder,
// listeyi ya da sanatçıyı takip eder.
func (c *Client) Save(ctx context.Context, uris []string) error {
	return c.do(ctx, http.MethodPut, "me/library?"+uriQuery(uris), nil, nil)
}

// Unsave, kitaplıktan çıkarır.
func (c *Client) Unsave(ctx context.Context, uris []string) error {
	return c.do(ctx, http.MethodDelete, "me/library?"+uriQuery(uris), nil, nil)
}

// AddToQueue, şarkıyı ya da bölümü etkin cihazın sırasına ekler.
func (c *Client) AddToQueue(ctx context.Context, uri string) error {
	return c.do(ctx, http.MethodPost, "me/player/queue?uri="+url.QueryEscape(uri), nil, nil)
}

// AddToPlaylist, şarkıları listenin sonuna ekler.
func (c *Client) AddToPlaylist(ctx context.Context, playlistID string, uris []string) error {
	return c.do(ctx, http.MethodPost, fmt.Sprintf("playlists/%s/items", url.PathEscape(playlistID)),
		map[string]any{"uris": uris}, nil)
}

// RemoveFromPlaylist, şarkının listedeki bütün kopyalarını çıkarır.
func (c *Client) RemoveFromPlaylist(ctx context.Context, playlistID string, uris []string) error {
	items := make([]map[string]string, 0, len(uris))
	for _, u := range uris {
		items = append(items, map[string]string{"uri": u})
	}
	// Yeni uç "items", eskisi "tracks" bekliyordu; ikisi de gönderilir.
	return c.do(ctx, http.MethodDelete, fmt.Sprintf("playlists/%s/items", url.PathEscape(playlistID)),
		map[string]any{"items": items, "tracks": items}, nil)
}

// WebLink, bir Spotify adresinin paylaşılabilir bağlantısı.
func WebLink(uri string) string {
	parts := strings.Split(uri, ":")
	if len(parts) < 3 {
		return uri
	}
	return "https://open.spotify.com/" + parts[len(parts)-2] + "/" + parts[len(parts)-1]
}
