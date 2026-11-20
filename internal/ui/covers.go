package ui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Ehtisam/sotto/internal/spotify"
)

// Kapaklar: çalan şarkınınki sol sütunda ve Şimdi çalıyor sayfasında,
// açık listeninki liste başlığında. Listelerin satırlarında kapak yok.

type coverMsg struct {
	url string
	err error
}

// fetchCovers, ekranda gereken ama henüz indirilmemiş kapakları ister.
func (m Model) fetchCovers() tea.Cmd {
	var urls []string
	for _, u := range []string{m.nowCoverURL(), m.openListCoverURL()} {
		if u == "" || m.rt.coverAsked[u] || m.rt.covers.Has(u) {
			continue
		}
		m.rt.coverAsked[u] = true
		urls = append(urls, u)
	}
	if len(urls) == 0 {
		return nil
	}
	store := m.rt.covers
	cmds := make([]tea.Cmd, 0, len(urls))
	for _, u := range urls {
		cmds = append(cmds, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			return coverMsg{u, store.Fetch(ctx, u)}
		})
	}
	return tea.Batch(cmds...)
}

// nowCoverURL, çalan şarkının kapağı: önce Soloist'in verdiği, yoksa Web API'nin.
func (m Model) nowCoverURL() string {
	it := m.state.Item
	if it == nil {
		return ""
	}
	sizes := map[string]string{}
	for _, c := range it.Decorations.VisualIdentity.Cover {
		sizes[c.Size] = c.URL
	}
	for _, s := range []string{"default", "large", "xlarge", "small"} {
		if u := sizes[s]; u != "" {
			return u
		}
	}
	if t, ok := m.rt.known[it.URI]; ok {
		return pickImage(t.Album.Images)
	}
	return ""
}

func (m Model) openListCoverURL() string {
	switch {
	case m.page == pageLibrary && m.lib.open != nil:
		return m.lib.open.entry.image
	case m.page == pageSearch && m.search.open != nil:
		return m.search.open.entry.image
	case m.page == pageHome && m.home.open != nil:
		return m.home.open.entry.image
	}
	return ""
}

// pickImage, ~300 piksele en yakın kapağı seçer: terminalde daha büyüğü fark etmez.
func pickImage(imgs []spotify.Image) string {
	best, bestDiff := "", 1<<30
	for _, im := range imgs {
		d := im.Width - 300
		if d < 0 {
			d = -d
		}
		if im.Width == 0 { // boyutu bilinmiyorsa son çare
			d = 1 << 20
		}
		if d < bestDiff {
			best, bestDiff = im.URL, d
		}
	}
	return best
}

// coverBox, kapağı ya da kapak yüklenene kadar aynı boyutta boş bir alan.
func (m Model) coverBox(url string, cols, rows int) string {
	if s := m.rt.covers.Render(url, cols, rows); s != "" {
		return s
	}
	line := strings.Repeat(" ", cols)
	return lipgloss.NewStyle().Width(cols).Height(rows).Render(strings.TrimRight(strings.Repeat(line+"\n", rows), "\n"))
}
