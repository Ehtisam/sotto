package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Ehtisam/sotto/internal/spotify"
)

// Kinds: "sotto play --type …" ile çalınabilen türler.
var Kinds = []string{"track", "album", "artist", "playlist", "show", "episode"}

// Find, sözcüklerle arar ve sözcüklere gerçekten uyan ilk sonucu verir
// (Spotify'ın araması her şeye bir şey bulduğu için sonuç denetlenir).
// label, insanlar için "Ad — Sanatçı".
func Find(ctx context.Context, sp *spotify.Client, words, kind string) (uri, label string, err error) {
	words = strings.TrimSpace(words)
	if words == "" {
		return "", "", errors.New("what should be played? e.g. sotto play daft punk one more time")
	}
	if kind == "" {
		kind = "track"
	}
	ok := false
	for _, k := range Kinds {
		ok = ok || k == kind
	}
	if !ok {
		return "", "", fmt.Errorf("--type takes %s", strings.Join(Kinds, ", "))
	}
	r, err := sp.Search(ctx, words, []string{kind}, 0)
	if err != nil {
		return "", "", err
	}
	type cand struct{ uri, name, by string }
	var cs []cand
	for _, t := range r.Tracks.Items {
		if t != nil {
			cs = append(cs, cand{t.URI, t.Name, t.ArtistNames()})
		}
	}
	for _, a := range r.Albums.Items {
		if a != nil {
			cs = append(cs, cand{a.URI, a.Name, artistNames(a.Artists)})
		}
	}
	for _, a := range r.Artists.Items {
		if a != nil {
			cs = append(cs, cand{a.URI, a.Name, ""})
		}
	}
	for _, p := range r.Playlists.Items {
		if p != nil {
			cs = append(cs, cand{p.URI, p.Name, p.Owner.DisplayName})
		}
	}
	for _, s := range r.Shows.Items {
		if s != nil {
			cs = append(cs, cand{s.URI, s.Name, ""})
		}
	}
	for _, e := range r.Episodes.Items {
		if e != nil {
			cs = append(cs, cand{e.URI, e.Name, ""})
		}
	}
	labelOf := func(c cand) string {
		if c.by != "" {
			return c.name + " — " + c.by
		}
		return c.name
	}
	for _, c := range cs {
		if c.uri != "" && Matches(words, c.name, c.by) {
			return c.uri, labelOf(c), nil
		}
	}
	if len(cs) == 0 {
		return "", "", fmt.Errorf("nothing found for %q", words)
	}
	var near []string
	for _, c := range cs[:min(3, len(cs))] {
		near = append(near, labelOf(c))
	}
	return "", "", fmt.Errorf("nothing matched %q; closest: %s", words, strings.Join(near, "; "))
}

// Target, "play" ve "queue" argümanlarını çözer: tek bir bağlantı/adres ya da
// aranacak sözcükler. label yalnızca aramada dolar.
func Target(ctx context.Context, sp *spotify.Client, args []string) (uri, label string, err error) {
	f := parse(args)
	if len(f.pos) == 1 {
		if u, err := ToURI(f.pos[0]); err == nil {
			return u, "", nil
		}
	}
	return Find(ctx, sp, f.rest(0), f.vals["type"])
}

// Playing, çalmaya başlayınca gösterilen metin ("" : bağlantıyla çalındı).
func Playing(label string) string {
	if label == "" {
		return ""
	}
	return "Playing " + label + "."
}
