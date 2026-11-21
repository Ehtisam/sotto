package spclient

import (
	"context"
	"fmt"
	"math/big"
	"net/url"
	"strings"
)

// Keşif ✱: sanatçı sayfası, biyografi, künye, takipçiler, arkadaş etkinliği.
// Hepsi masaüstü uygulamasının kullandığı iç uçlardan, bridge jetonuyla.

// ArtistTrack, sanatçının popüler şarkılarından biri.
type ArtistTrack struct {
	URI      string
	Name     string
	AlbumURI string
	Plays    string // Spotify'ın yazdığı gibi ("33,324,983")
}

// Card, sanatçı sayfasındaki bir albüm, liste ya da sanatçı kartı.
type Card struct {
	URI   string
	Name  string
	Sub   string // yıl ya da açıklama
	Image string
}

// ArtistView, Spotify uygulamasındaki sanatçı sayfasının özeti.
type ArtistView struct {
	URI       string
	Name      string
	Image     string
	Verified  bool
	Listeners string // "1M monthly listeners" (dil Accept-Language'a göre)

	Popular   []ArtistTrack
	Releases  []Card // popüler çıkanlar
	Featuring []Card // "This Is …", radyo gibi listeler
	Related   []Card // hayranlar bunları da seviyor
	AppearsOn []Card
}

// hubNode, artistview'ın (glue hub) bir öğesi; yalnızca kullanılan alanlar.
type hubNode struct {
	ID   string `json:"id"`
	Text struct {
		Title       string `json:"title"`
		Subtitle    string `json:"subtitle"`
		Description string `json:"description"`
		Accessory   string `json:"accessory"`
	} `json:"text"`
	Images struct {
		Main struct {
			URI string `json:"uri"`
		} `json:"main"`
	} `json:"images"`
	Metadata struct {
		URI       string `json:"uri"`
		SectionID string `json:"sectionId"`
		AlbumURI  string `json:"album_uri"`
	} `json:"metadata"`
	Custom struct {
		Verified bool `json:"verified"`
	} `json:"custom"`
	Children []hubNode `json:"children"`
}

// Artist, sanatçı sayfasını getirir.
func Artist(ctx context.Context, token, id string) (ArtistView, error) {
	var r struct {
		Header hubNode   `json:"header"`
		Body   []hubNode `json:"body"`
	}
	if err := getJSON(ctx, token, "/artistview/v1/artist/"+url.PathEscape(id)+"?format=json", &r); err != nil {
		return ArtistView{}, err
	}
	return parseArtist(id, r.Header, r.Body), nil
}

func parseArtist(id string, h hubNode, body []hubNode) ArtistView {
	v := ArtistView{URI: "spotify:artist:" + id, Name: h.Text.Title, Image: h.Images.Main.URI,
		Verified: h.Custom.Verified, Listeners: h.Text.Accessory}
	card := func(n hubNode) Card {
		sub := n.Text.Subtitle
		if sub == "" {
			sub = n.Text.Description
		}
		return Card{URI: n.Metadata.URI, Name: n.Text.Title, Sub: sub, Image: n.Images.Main.URI}
	}
	// Bölüm, öğenin kimliğinden anlaşılır ("artist-entity-view-related_card0").
	section := func(n hubNode) string {
		s := n.Metadata.SectionID
		if s == "" {
			s = n.ID
		}
		s = strings.TrimPrefix(s, "artist-entity-view-")
		if i := strings.IndexByte(s, '_'); i >= 0 {
			s = s[:i]
		}
		return s
	}
	add := func(n hubNode) {
		uri := n.Metadata.URI
		if uri == "" || n.Text.Title == "" {
			return
		}
		switch sec := section(n); {
		case strings.HasPrefix(sec, "top-tracks") && strings.HasPrefix(uri, "spotify:track:"):
			v.Popular = append(v.Popular, ArtistTrack{URI: uri, Name: n.Text.Title, AlbumURI: n.Metadata.AlbumURI, Plays: n.Text.Subtitle})
		case sec == "releases" && !strings.HasSuffix(uri, ":releases"):
			v.Releases = append(v.Releases, card(n))
		case sec == "you-might-also-like":
			v.Featuring = append(v.Featuring, card(n))
		case sec == "related" && strings.HasPrefix(uri, "spotify:artist:"):
			v.Related = append(v.Related, card(n))
		case strings.HasPrefix(sec, "appears-on"):
			v.AppearsOn = append(v.AppearsOn, card(n))
		}
	}
	for _, n := range body {
		add(n)
		for _, c := range n.Children {
			add(c)
		}
	}
	return v
}

// ArtistBio, sanatçının biyografisi (düz metin; Spotify'ın bağlantıları adıyla kalır).
func ArtistBio(ctx context.Context, token, id string) (string, error) {
	var r struct {
		Biography []struct {
			Text string `json:"text"`
		} `json:"biography"`
	}
	gid, err := GID(id)
	if err != nil {
		return "", err
	}
	if err := getJSON(ctx, token, "/metadata/4/artist/"+gid+"?market=from_token", &r); err != nil {
		return "", err
	}
	if len(r.Biography) == 0 {
		return "", nil
	}
	return r.Biography[0].Text, nil
}

// GID, base62 Spotify kimliğini iç uçların kullandığı 32 haneli onaltılıya çevirir.
func GID(id string) (string, error) {
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	n := new(big.Int)
	for _, c := range id {
		i := strings.IndexRune(alphabet, c)
		if i < 0 {
			return "", fmt.Errorf("spclient: bad id %q", id)
		}
		n.Mul(n, big.NewInt(62))
		n.Add(n, big.NewInt(int64(i)))
	}
	if n.BitLen() > 128 {
		return "", fmt.Errorf("spclient: bad id %q", id)
	}
	return fmt.Sprintf("%032x", n), nil
}

// CreditArtist, künyedeki bir kişi.
type CreditArtist struct {
	URI      string   `json:"uri"` // boş olabilir (Spotify'da sanatçı sayfası yoksa)
	Name     string   `json:"name"`
	Subroles []string `json:"subroles"`
}

// CreditRole, künyenin bir bölümü (Performers, Writers, Producers).
type CreditRole struct {
	Title   string         `json:"roleTitle"`
	Artists []CreditArtist `json:"artists"`
}

// Credits, bir şarkının künyesi.
type Credits struct {
	Roles   []CreditRole `json:"roleCredits"`
	Sources []string     `json:"sourceNames"` // plak şirketi ya da dağıtıcı
}

// TrackCredits, şarkının künyesini getirir.
func TrackCredits(ctx context.Context, token, id string) (Credits, error) {
	var c Credits
	err := getJSON(ctx, token, "/track-credits-view/v0/experimental/"+url.PathEscape(id)+"/credits", &c)
	return c, err
}

// UserCard, takipçi ya da takip edilen listesindeki bir kişi ya da sanatçı.
type UserCard struct {
	URI       string `json:"uri"`
	Name      string `json:"name"`
	Image     string `json:"image_url"`
	Followers int    `json:"followers_count"`
	// Followed: bu kişi kullanıcıyı takip ediyor (takipçiler listesinde hep
	// true); Following: kullanıcı bu kişiyi takip ediyor.
	Followed  bool `json:"is_followed"`
	Following bool `json:"is_following"`
}

// Followers, bir kullanıcının takipçileri.
func Followers(ctx context.Context, token, userID string) ([]UserCard, error) {
	return userCards(ctx, token, userID, "followers")
}

// Following, bir kullanıcının takip ettiği kişiler ve sanatçılar.
func Following(ctx context.Context, token, userID string) ([]UserCard, error) {
	return userCards(ctx, token, userID, "following")
}

func userCards(ctx context.Context, token, userID, which string) ([]UserCard, error) {
	var r struct {
		Profiles []UserCard `json:"profiles"`
	}
	err := getJSON(ctx, token, "/user-profile-view/v3/profile/"+url.PathEscape(userID)+"/"+which+"?market=from_token", &r)
	return r.Profiles, err
}

// FriendTrack, bir arkadaşın son dinlediği.
type FriendTrack struct {
	URI    string `json:"uri"`
	Name   string `json:"name"`
	Image  string `json:"imageUrl"`
	Album  Ref    `json:"album"`
	Artist Ref    `json:"artist"`
	Ctx    Ref    `json:"context"`
}

// Ref, adı ve adresi olan bir şey.
type Ref struct {
	URI  string `json:"uri"`
	Name string `json:"name"`
}

// Friend, arkadaş etkinliğindeki bir satır.
type Friend struct {
	Timestamp int64       `json:"timestamp"` // ms
	User      Ref         `json:"user"`
	Track     FriendTrack `json:"track"`
}

// Friends, arkadaş etkinliği: takip ettiğin ve dinlediğini paylaşan kişiler.
func Friends(ctx context.Context, token string) ([]Friend, error) {
	var r struct {
		Friends []Friend `json:"friends"`
	}
	err := getJSON(ctx, token, "/presence-view/v1/buddylist", &r)
	return r.Friends, err
}
