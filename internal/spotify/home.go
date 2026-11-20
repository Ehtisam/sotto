package spotify

import (
	"context"
	"fmt"
	"time"
)

// PlayHistory, son çalınanlardan bir giriş.
type PlayHistory struct {
	Track    Track     `json:"track"`
	PlayedAt time.Time `json:"played_at"`
	Context  *struct {
		URI  string `json:"uri"`
		Type string `json:"type"` // playlist, album, artist, collection
	} `json:"context"`
}

// RecentlyPlayed, en yeniden eskiye son çalınan şarkılar (en çok 50).
func (c *Client) RecentlyPlayed(ctx context.Context, limit int) ([]PlayHistory, error) {
	var r struct {
		Items []PlayHistory `json:"items"`
	}
	err := c.get(ctx, fmt.Sprintf("me/player/recently-played?limit=%d", min(limit, 50)), &r)
	if err == nil {
		for _, it := range r.Items {
			c.remember(it.Track)
		}
	}
	return r.Items, err
}

// Zaman aralıkları: son 4 hafta, son 6 ay, tüm zamanlar.
const (
	ShortTerm  = "short_term"
	MediumTerm = "medium_term"
	LongTerm   = "long_term"
)

func (c *Client) TopTracks(ctx context.Context, timeRange string, limit int) ([]Track, error) {
	var p Paging[Track]
	err := c.get(ctx, fmt.Sprintf("me/top/tracks?time_range=%s&limit=%d", timeRange, limit), &p)
	if err == nil {
		c.remember(p.Items...)
	}
	return p.Items, err
}

func (c *Client) TopArtists(ctx context.Context, timeRange string, limit int) ([]FullArtist, error) {
	var p Paging[FullArtist]
	err := c.get(ctx, fmt.Sprintf("me/top/artists?time_range=%s&limit=%d", timeRange, limit), &p)
	return p.Items, err
}
