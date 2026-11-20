package spotify

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type Image struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type Artist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URI  string `json:"uri"`
}

type Album struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	URI         string   `json:"uri"`
	Images      []Image  `json:"images"`
	ReleaseDate string   `json:"release_date"`
	Artists     []Artist `json:"artists"`
}

type Track struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	URI        string   `json:"uri"`
	DurationMS int      `json:"duration_ms"`
	Explicit   bool     `json:"explicit"`
	IsLocal    bool     `json:"is_local"`
	Artists    []Artist `json:"artists"`
	Album      Album    `json:"album"`
	Type       string   `json:"type"` // track ya da episode

	// Yalnızca podcast bölümlerinde.
	ReleaseDate string  `json:"release_date"`
	Images      []Image `json:"images"`

	// AddedAt: listeye ya da beğenilenlere eklenme zamanı (sotto doldurur).
	AddedAt string `json:"added_at,omitempty"`
}

func (t Track) ArtistNames() string {
	names := make([]string, 0, len(t.Artists))
	for _, a := range t.Artists {
		names = append(names, a.Name)
	}
	return strings.Join(names, ", ")
}

type User struct {
	ID          string  `json:"id"`
	DisplayName string  `json:"display_name"`
	Product     string  `json:"product"`
	Images      []Image `json:"images"`
	Followers   struct {
		Total int `json:"total"`
	} `json:"followers"`
}

type countRef struct {
	Total int `json:"total"`
}

type Playlist struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	URI           string `json:"uri"`
	Description   string `json:"description"`
	Collaborative bool   `json:"collaborative"`
	Public        *bool  `json:"public"`
	Owner         struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"owner"`
	Images []Image `json:"images"`
	// Spotify 2026'da "tracks" alanını "items" olarak yeniden adlandırdı; ikisi de okunur.
	TracksRef *countRef `json:"tracks"`
	ItemsRef  *countRef `json:"items"`
}

// Total, listedeki şarkı sayısı.
func (p Playlist) Total() int {
	if p.ItemsRef != nil {
		return p.ItemsRef.Total
	}
	if p.TracksRef != nil {
		return p.TracksRef.Total
	}
	return 0
}

// Paging, Web API'nin sayfalı yanıtı.
type Paging[T any] struct {
	Items  []T    `json:"items"`
	Total  int    `json:"total"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
	Next   string `json:"next"`
}

// PlaylistItem, listedeki bir giriş. Yeni yanıtlarda şarkı "item",
// eskilerde "track" alanında gelir.
type PlaylistItem struct {
	AddedAt string `json:"added_at"`
	Item    *Track `json:"item"`
	Track   *Track `json:"track"`
}

func (p PlaylistItem) T() *Track {
	if p.Item != nil {
		return p.Item
	}
	return p.Track
}

type SavedTrack struct {
	AddedAt string `json:"added_at"`
	Track   Track  `json:"track"`
}

type Device struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	IsActive bool   `json:"is_active"`
	Volume   *int   `json:"volume_percent"`
	// SupportsVolume: false ise sesi uzaktan değiştirilemez.
	SupportsVolume *bool `json:"supports_volume"`
}

// IDFromURI, "spotify:track:ID" biçiminden ID'yi alır.
func IDFromURI(uri string) string {
	if i := strings.LastIndexByte(uri, ':'); i >= 0 {
		return uri[i+1:]
	}
	return uri
}

// ---- uçlar ----

func (c *Client) Me(ctx context.Context) (User, error) {
	var u User
	err := c.get(ctx, "me", &u)
	return u, err
}

// Track, şarkı bilgisini getirir; sonuçlar bellekte saklanır.
func (c *Client) Track(ctx context.Context, id string) (Track, error) {
	c.cacheMu.Lock()
	t, ok := c.tracks[id]
	c.cacheMu.Unlock()
	if ok {
		return t, nil
	}
	if err := c.get(ctx, "tracks/"+url.PathEscape(id), &t); err != nil {
		return t, err
	}
	c.remember(t)
	return t, nil
}

func (c *Client) remember(ts ...Track) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	for _, t := range ts {
		if t.ID != "" {
			c.tracks[t.ID] = t
		}
	}
}

func (c *Client) MyPlaylists(ctx context.Context, offset, limit int) (Paging[Playlist], error) {
	var p Paging[Playlist]
	err := c.get(ctx, fmt.Sprintf("me/playlists?offset=%d&limit=%d", offset, limit), &p)
	return p, err
}

// PlaylistItems, listenin şarkıları. Önce yeni /items ucunu, olmazsa eski /tracks ucunu dener.
func (c *Client) PlaylistItems(ctx context.Context, id string, offset, limit int) (Paging[PlaylistItem], error) {
	var p Paging[PlaylistItem]
	q := fmt.Sprintf("?offset=%d&limit=%d", offset, limit)
	err := c.get(ctx, "playlists/"+url.PathEscape(id)+"/items"+q, &p)
	if IsStatus(err, http.StatusNotFound) {
		err = c.get(ctx, "playlists/"+url.PathEscape(id)+"/tracks"+q, &p)
	}
	if err == nil {
		for _, it := range p.Items {
			if t := it.T(); t != nil {
				c.remember(*t)
			}
		}
	}
	return p, err
}

func (c *Client) LikedTracks(ctx context.Context, offset, limit int) (Paging[SavedTrack], error) {
	var p Paging[SavedTrack]
	err := c.get(ctx, fmt.Sprintf("me/tracks?offset=%d&limit=%d", offset, limit), &p)
	if err == nil {
		for _, it := range p.Items {
			c.remember(it.Track)
		}
	}
	return p, err
}

func (c *Client) Devices(ctx context.Context) ([]Device, error) {
	var r struct {
		Devices []Device `json:"devices"`
	}
	err := c.get(ctx, "me/player/devices", &r)
	return r.Devices, err
}

// PlayContext, bir listeyi ya da albümü verilen şarkıdan başlatır.
// deviceID boşsa etkin cihazda çalar.
func (c *Client) PlayContext(ctx context.Context, deviceID, contextURI, startURI string) error {
	body := map[string]any{"context_uri": contextURI}
	if startURI != "" {
		body["offset"] = map[string]string{"uri": startURI}
	}
	return c.do(ctx, http.MethodPut, "me/player/play"+deviceQuery(deviceID), body, nil)
}

// PlayTracks, verilen şarkıları sırayla çalar (beğenilenler gibi bağlamı olmayanlar için).
func (c *Client) PlayTracks(ctx context.Context, deviceID string, uris []string, startURI string) error {
	body := map[string]any{"uris": uris}
	if startURI != "" {
		body["offset"] = map[string]string{"uri": startURI}
	}
	return c.do(ctx, http.MethodPut, "me/player/play"+deviceQuery(deviceID), body, nil)
}

func deviceQuery(id string) string {
	if id == "" {
		return ""
	}
	return "?device_id=" + url.QueryEscape(id)
}

// PlayFrom, bir bağlamı verilen şarkıdan ve konumdan başlatır (kaldığın
// yerden devam). Bağlam yoksa yalnızca şarkıyı çalar.
func (c *Client) PlayFrom(ctx context.Context, deviceID, contextURI, trackURI string, positionMS int64) error {
	body := map[string]any{"position_ms": positionMS}
	if contextURI != "" {
		body["context_uri"] = contextURI
		if trackURI != "" {
			body["offset"] = map[string]string{"uri": trackURI}
		}
	} else {
		body["uris"] = []string{trackURI}
	}
	return c.do(ctx, http.MethodPut, "me/player/play"+deviceQuery(deviceID), body, nil)
}

// PlayContextAt, bir bağlamı verilen sıradaki şarkıdan başlatır (karıştırarak
// çalarken rastgele bir yerden başlamak için).
func (c *Client) PlayContextAt(ctx context.Context, deviceID, contextURI string, position int) error {
	body := map[string]any{"context_uri": contextURI}
	if position > 0 {
		body["offset"] = map[string]int{"position": position}
	}
	return c.do(ctx, http.MethodPut, "me/player/play"+deviceQuery(deviceID), body, nil)
}
