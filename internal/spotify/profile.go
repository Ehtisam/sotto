package spotify

import "context"

// Profil sayfasının sayıları. Her biri tek öğelik bir sayfa ister; yalnızca
// toplam kullanılır.

// FollowedArtists, takip edilen sanatçı sayısı.
func (c *Client) FollowedArtists(ctx context.Context) (int, error) {
	var r struct {
		Artists struct {
			Total int `json:"total"`
		} `json:"artists"`
	}
	err := c.get(ctx, "me/following?type=artist&limit=1", &r)
	return r.Artists.Total, err
}

// SavedAlbums, kitaplıktaki albüm sayısı.
func (c *Client) SavedAlbums(ctx context.Context) (int, error) {
	return c.total(ctx, "me/albums?limit=1")
}

// SavedShows, kitaplıktaki podcast sayısı.
func (c *Client) SavedShows(ctx context.Context) (int, error) {
	return c.total(ctx, "me/shows?limit=1")
}

func (c *Client) total(ctx context.Context, path string) (int, error) {
	var p Paging[struct{}]
	err := c.get(ctx, path, &p)
	return p.Total, err
}

// ProfileLink, kullanıcının Spotify'daki profil sayfası.
func ProfileLink(userID string) string { return "https://open.spotify.com/user/" + userID }
