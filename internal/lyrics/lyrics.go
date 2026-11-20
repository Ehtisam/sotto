// Package lyrics, şarkı sözlerini getirir: önce Spotify'ın kendi sözleri
// (✱, masaüstü jetonuyla), bulunamazsa LRCLIB (lrclib.net).
package lyrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ErrNotFound: iki kaynakta da söz yok.
var ErrNotFound = errors.New("no lyrics found")

// ErrUnauthorized: Spotify jetonu reddetti; köprü yeni jeton almalı.
var ErrUnauthorized = errors.New("spotify rejected the token")

type Line struct {
	Start time.Duration
	Text  string
}

type Lyrics struct {
	Synced bool
	Lines  []Line
	Source string // "Spotify · Musixmatch", "LRCLIB"
}

// At, verilen konumda söylenen satırın sırası (-1: henüz başlamadı).
func (l *Lyrics) At(pos time.Duration) int {
	if l == nil || !l.Synced {
		return -1
	}
	i := sort.Search(len(l.Lines), func(i int) bool { return l.Lines[i].Start > pos })
	return i - 1
}

// Track, söz aranacak şarkı.
type Track struct {
	ID       string // Spotify şarkı ID'si
	Name     string
	Artist   string
	Album    string
	Duration time.Duration
}

var client = &http.Client{Timeout: 15 * time.Second}

// FromSpotify, Spotify'ın iç söz bağlantısından getirir (✱).
func FromSpotify(ctx context.Context, token, trackID string) (*Lyrics, error) {
	u := "https://spclient.wg.spotify.com/color-lyrics/v2/track/" + url.PathEscape(trackID) + "?format=json&vocalRemoval=false&market=from_token"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("app-platform", "WebPlayer")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound, http.StatusNoContent:
		return nil, ErrNotFound
	case http.StatusUnauthorized:
		return nil, ErrUnauthorized
	default:
		return nil, fmt.Errorf("spotify lyrics: %s", resp.Status)
	}
	var body struct {
		Lyrics struct {
			SyncType string `json:"syncType"`
			Provider string `json:"providerDisplayName"`
			Lines    []struct {
				Start string `json:"startTimeMs"`
				Words string `json:"words"`
			} `json:"lines"`
		} `json:"lyrics"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	l := &Lyrics{Synced: body.Lyrics.SyncType == "LINE_SYNCED", Source: "Spotify"}
	if body.Lyrics.Provider != "" {
		l.Source += " · " + body.Lyrics.Provider
	}
	for _, ln := range body.Lyrics.Lines {
		ms, _ := strconv.ParseInt(ln.Start, 10, 64)
		l.Lines = append(l.Lines, Line{Start: time.Duration(ms) * time.Millisecond, Text: clean(ln.Words)})
	}
	if len(l.Lines) == 0 {
		return nil, ErrNotFound
	}
	return l, nil
}

type lrclibRecord struct {
	Synced   string  `json:"syncedLyrics"`
	Plain    string  `json:"plainLyrics"`
	Duration float64 `json:"duration"`
}

// FromLRCLIB, lrclib.net'ten getirir. Önce tam eşleşme, olmazsa arama dener.
func FromLRCLIB(ctx context.Context, t Track) (*Lyrics, error) {
	q := url.Values{"track_name": {t.Name}, "artist_name": {t.Artist}}
	if t.Album != "" {
		q.Set("album_name", t.Album)
	}
	if t.Duration > 0 {
		q.Set("duration", strconv.Itoa(int(t.Duration.Seconds())))
	}
	var rec lrclibRecord
	err := lrclib(ctx, "https://lrclib.net/api/get?"+q.Encode(), &rec)
	if errors.Is(err, ErrNotFound) {
		var list []lrclibRecord
		s := url.Values{"track_name": {t.Name}, "artist_name": {t.Artist}}
		if err = lrclib(ctx, "https://lrclib.net/api/search?"+s.Encode(), &list); err != nil {
			return nil, err
		}
		best := -1
		for i, r := range list {
			// Süresi birkaç saniye içinde tutan ve tercihen eş zamanlı olan kaydı seç.
			if t.Duration > 0 && absf(r.Duration-t.Duration.Seconds()) > 4 {
				continue
			}
			if best == -1 || (r.Synced != "" && list[best].Synced == "") {
				best = i
			}
		}
		if best == -1 {
			return nil, ErrNotFound
		}
		rec = list[best]
	} else if err != nil {
		return nil, err
	}
	if rec.Synced != "" {
		if l := ParseLRC(rec.Synced); len(l.Lines) > 0 {
			l.Source = "LRCLIB"
			return l, nil
		}
	}
	if strings.TrimSpace(rec.Plain) == "" {
		return nil, ErrNotFound
	}
	l := &Lyrics{Source: "LRCLIB"}
	for _, s := range strings.Split(rec.Plain, "\n") {
		l.Lines = append(l.Lines, Line{Text: clean(s)})
	}
	return l, nil
}

func lrclib(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "sotto (https://github.com/Ehtisam/sotto)")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("lrclib: %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

var lrcRE = regexp.MustCompile(`^\[(\d+):(\d+(?:\.\d+)?)\]\s?(.*)$`)

// ParseLRC, "[mm:ss.xx] metin" satırlarını çözer.
func ParseLRC(s string) *Lyrics {
	l := &Lyrics{Synced: true}
	for _, raw := range strings.Split(s, "\n") {
		m := lrcRE.FindStringSubmatch(strings.TrimSpace(raw))
		if m == nil {
			continue
		}
		min, _ := strconv.Atoi(m[1])
		sec, _ := strconv.ParseFloat(m[2], 64)
		start := time.Duration(min)*time.Minute + time.Duration(sec*float64(time.Second))
		l.Lines = append(l.Lines, Line{Start: start, Text: clean(m[3])})
	}
	sort.SliceStable(l.Lines, func(i, j int) bool { return l.Lines[i].Start < l.Lines[j].Start })
	return l
}

// clean, boş ara satırları "♪" olarak gösterir.
func clean(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "♪"
	}
	return s
}

func absf(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
