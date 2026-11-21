package spotify

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// Keşif: kitaplıktaki albümler, sanatçılar ve podcast'ler; sanatçının
// diskografisi; albüm, bölüm ve sanatçı ayrıntıları.

// SavedAlbum, kitaplıktaki bir albüm.
type SavedAlbum struct {
	AddedAt string      `json:"added_at"`
	Album   SimpleAlbum `json:"album"`
}

// SavedShow, kitaplıktaki bir podcast.
type SavedShow struct {
	AddedAt string `json:"added_at"`
	Show    Show   `json:"show"`
}

// LibraryAlbums, kitaplıktaki albümler (en son eklenen üstte).
func (c *Client) LibraryAlbums(ctx context.Context, offset, limit int) (Paging[SavedAlbum], error) {
	var p Paging[SavedAlbum]
	err := c.get(ctx, fmt.Sprintf("me/albums?offset=%d&limit=%d", offset, limit), &p)
	return p, err
}

// LibraryShows, kitaplıktaki podcast'ler.
func (c *Client) LibraryShows(ctx context.Context, offset, limit int) (Paging[SavedShow], error) {
	var p Paging[SavedShow]
	err := c.get(ctx, fmt.Sprintf("me/shows?offset=%d&limit=%d", offset, limit), &p)
	return p, err
}

// LibraryArtists, takip edilen bütün sanatçılar (en çok max tane).
func (c *Client) LibraryArtists(ctx context.Context, max int) ([]FullArtist, error) {
	var out []FullArtist
	after := ""
	for len(out) < max {
		var r struct {
			Artists struct {
				Items   []FullArtist `json:"items"`
				Cursors struct {
					After string `json:"after"`
				} `json:"cursors"`
			} `json:"artists"`
		}
		path := "me/following?type=artist&limit=50"
		if after != "" {
			path += "&after=" + url.QueryEscape(after)
		}
		if err := c.get(ctx, path, &r); err != nil {
			return out, err
		}
		out = append(out, r.Artists.Items...)
		after = r.Artists.Cursors.After
		if after == "" || len(r.Artists.Items) == 0 {
			break
		}
	}
	return out, nil
}

// ArtistAlbum, sanatçının diskografisindeki bir çıkış.
type ArtistAlbum struct {
	SimpleAlbum
	// AlbumGroup: album, single, compilation, appears_on.
	AlbumGroup string `json:"album_group"`
}

// Group, çıkışın diskografideki grubu: album, single, compilation ya da
// appears_on. Spotify 2026'dan beri geliştirici uygulamalarına album_group'u
// boş veriyor; o zaman sanatçı albümün sanatçıları arasında değilse
// "yer aldığı", değilse albümün türü.
func (a ArtistAlbum) Group(artistID string) string {
	if a.AlbumGroup != "" {
		return a.AlbumGroup
	}
	own := false
	for _, ar := range a.Artists {
		if ar.ID == artistID || ar.URI == "spotify:artist:"+artistID {
			own = true
		}
	}
	if !own {
		return "appears_on"
	}
	if a.AlbumType == "" {
		return "album"
	}
	return a.AlbumType
}

// ArtistAlbums, sanatçının bütün çıkışları (Spotify'ın sırasıyla: gruplar,
// her grupta en yeni üstte). Geliştirici uygulamalarında sayfa en çok 10
// olabiliyor: 50 reddedilirse 10'la sürer.
func (c *Client) ArtistAlbums(ctx context.Context, id string, max int) ([]ArtistAlbum, error) {
	var out []ArtistAlbum
	limit := 50
	for offset := 0; offset < max; {
		var p Paging[ArtistAlbum]
		err := c.get(ctx, fmt.Sprintf("artists/%s/albums?include_groups=album,single,compilation,appears_on&offset=%d&limit=%d",
			url.PathEscape(id), offset, limit), &p)
		if err != nil && limit > 10 && IsStatus(err, http.StatusBadRequest) {
			limit = 10
			continue
		}
		if err != nil {
			return out, err
		}
		out = append(out, p.Items...)
		offset += len(p.Items)
		if p.Next == "" || len(p.Items) == 0 {
			break
		}
	}
	return out, nil
}

// ArtistLatest, sanatçının en yeni albüm ve single'ları (Yenilikler için tek
// ya da iki istek). Spotify grupları sırayla, her grubu en yeniden veriyor:
// ilk sayfa hep albümse single'lar ayrıca istenir.
func (c *Client) ArtistLatest(ctx context.Context, id string) ([]ArtistAlbum, error) {
	var p Paging[ArtistAlbum]
	if err := c.get(ctx, fmt.Sprintf("artists/%s/albums?include_groups=album,single&limit=10", url.PathEscape(id)), &p); err != nil {
		return nil, err
	}
	out := p.Items
	allAlbums := len(p.Items) > 0
	for _, a := range p.Items {
		if a.Group(id) != "album" {
			allAlbums = false
		}
	}
	if allAlbums && p.Total > len(p.Items) {
		var s Paging[ArtistAlbum]
		if err := c.get(ctx, fmt.Sprintf("artists/%s/albums?include_groups=single&limit=10", url.PathEscape(id)), &s); err == nil {
			out = append(out, s.Items...)
		}
	}
	return out, nil
}

// ArtistInfo, sanatçının resmî bilgisi (ad, fotoğraf). Takipçi sayısı ve
// türler geliştirici uygulamalarına 2026'dan beri boş gelebiliyor.
type ArtistInfo struct {
	FullArtist
	Genres    []string `json:"genres"`
	Followers *struct {
		Total int `json:"total"`
	} `json:"followers"`
}

// Artist, bir sanatçının resmî bilgisi.
func (c *Client) Artist(ctx context.Context, id string) (ArtistInfo, error) {
	var a ArtistInfo
	err := c.get(ctx, "artists/"+url.PathEscape(id), &a)
	return a, err
}

// AlbumDetails, albüm bilgisi: çıkış tarihi, plak şirketi, telifler.
type AlbumDetails struct {
	SimpleAlbum
	Label      string `json:"label"`
	Copyrights []struct {
		Text string `json:"text"`
		Type string `json:"type"` // C (telif) ya da P (kayıt)
	} `json:"copyrights"`
	Tracks Paging[Track] `json:"tracks"`
}

// AlbumDetail, bir albümün ayrıntıları (ilk şarkı sayfasıyla).
func (c *Client) AlbumDetail(ctx context.Context, id string) (AlbumDetails, error) {
	var a AlbumDetails
	err := c.get(ctx, "albums/"+url.PathEscape(id), &a)
	return a, err
}

// Episode, bir podcast bölümünün ayrıntıları.
type Episode struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	URI         string   `json:"uri"`
	Description string   `json:"description"`
	DurationMS  int      `json:"duration_ms"`
	ReleaseDate string   `json:"release_date"`
	Explicit    bool     `json:"explicit"`
	Languages   []string `json:"languages"`
	Images      []Image  `json:"images"`
	Show        Show     `json:"show"`
	// ResumePoint: kullanıcının kaldığı yer (user-read-playback-position izni).
	ResumePoint *struct {
		FullyPlayed      bool `json:"fully_played"`
		ResumePositionMS int  `json:"resume_position_ms"`
	} `json:"resume_point"`
}

// EpisodeInfo, bir bölümün ayrıntıları.
func (c *Client) EpisodeInfo(ctx context.Context, id string) (Episode, error) {
	var e Episode
	err := c.get(ctx, "episodes/"+url.PathEscape(id), &e)
	return e, err
}

// ShowDetails, bir podcast'in ayrıntıları.
type ShowDetails struct {
	Show
	Languages []string `json:"languages"`
	Explicit  bool     `json:"explicit"`
	MediaType string   `json:"media_type"`
}

// ShowInfo, bir podcast'in ayrıntıları.
func (c *Client) ShowInfo(ctx context.Context, id string) (ShowDetails, error) {
	var s ShowDetails
	err := c.get(ctx, "shows/"+url.PathEscape(id), &s)
	return s, err
}
