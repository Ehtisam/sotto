package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/spotify"
)

// Kitaplıktaki albümler, takip edilen sanatçılar ve podcast'ler. Listelerden
// ayrı tutulur (klasörler ve rootlist yalnızca listeleri içerir); "Hepsi"
// süzgecinde listelerin ardından, kendi süzgeçlerinde tek başına görünür.

const kindArtist = "artist"

type savedLibMsg struct {
	entries []libEntry
	err     error
}

func (m Model) loadSaved() tea.Cmd {
	sp, rt := m.sp, m.rt
	return func() tea.Msg {
		ctx, cancel := apiCtx(rt, 60*time.Second)
		defer cancel()
		var msg savedLibMsg
		fail := func(err error) {
			if msg.err == nil {
				msg.err = err
			}
		}
		for offset := 0; offset < 2000; offset += 50 {
			p, err := sp.LibraryAlbums(ctx, offset, 50)
			if err != nil {
				fail(err)
				break
			}
			for _, s := range p.Items {
				if s.Album.ID != "" {
					msg.entries = append(msg.entries, savedAlbumEntry(s.Album))
				}
			}
			if p.Next == "" || len(p.Items) == 0 {
				break
			}
		}
		artists, err := sp.LibraryArtists(ctx, 2000)
		if err != nil {
			fail(err)
		}
		for _, a := range artists {
			msg.entries = append(msg.entries, libEntry{kind: kindArtist, key: a.ID, uri: a.URI, name: a.Name, image: pickImage(a.Images)})
		}
		for offset := 0; offset < 1000; offset += 50 {
			p, err := sp.LibraryShows(ctx, offset, 50)
			if err != nil {
				fail(err)
				break
			}
			for _, s := range p.Items {
				sh := s.Show
				if sh.ID == "" {
					continue
				}
				msg.entries = append(msg.entries, libEntry{kind: kindShow, key: sh.ID, uri: sh.URI, name: sh.Name,
					owner: F("search.episodecount", sh.TotalEpisodes), desc: cleanDesc(sh.Description),
					image: pickImage(sh.Images), total: sh.TotalEpisodes})
			}
			if p.Next == "" || len(p.Items) == 0 {
				break
			}
		}
		return msg
	}
}

func savedAlbumEntry(a spotify.SimpleAlbum) libEntry {
	year := a.ReleaseDate
	if len(year) > 4 {
		year = year[:4]
	}
	return libEntry{kind: kindAlbum, key: a.ID, uri: a.URI, name: a.Name, owner: artistNames(a.Artists), desc: year,
		image: pickImage(a.Images), total: a.TotalTracks, album: a.Album}
}

func (m Model) updateSavedLib(msg savedLibMsg) (Model, tea.Cmd) {
	m.lib.savedLoading = false
	if len(msg.entries) > 0 || msg.err == nil {
		m.lib.saved, m.lib.savedLoaded = msg.entries, true
	}
	if msg.err != nil {
		m.lib.savedErr = msg.err
		if len(msg.entries) == 0 {
			m = m.spotifyFailed(msg.err)
		}
	}
	m.clampLib()
	return m, nil
}

// savedKinds: süzgece göre gösterilecek kayıtlı tür ("" hiçbiri).
func savedKind(f libFilter) string {
	switch f {
	case libAlbums:
		return kindAlbum
	case libArtists:
		return kindArtist
	case libPodcasts:
		return kindShow
	}
	return ""
}

// savedOf, verilen türdeki kayıtlı öğeler.
func (m Model) savedOf(kind string) []libEntry {
	var out []libEntry
	for _, e := range m.lib.saved {
		if e.kind == kind {
			out = append(out, e)
		}
	}
	return out
}

// savedInfo, kayıtlı öğenin satırının sağı.
func savedInfo(e libEntry) string {
	switch e.kind {
	case kindAlbum:
		return joinDot(e.owner, e.desc)
	case kindArtist:
		return T("kind.artist")
	case kindShow:
		return joinDot(T("kind.show"), e.owner)
	}
	return ""
}
