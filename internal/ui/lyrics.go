package ui

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/bridge"
	"github.com/Ehtisam/sotto/internal/lyrics"
	"github.com/Ehtisam/sotto/internal/spotify"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// Şarkı sözleri: Şimdi çalıyor sayfasında, kapağın yanında. Önce Spotify'ın
// kendi sözleri (✱), yoksa LRCLIB. l tuşu sözler ile sıradakiler arasında geçer.

type lyricsState struct {
	showQueue bool      // l: sözler yerine sıradakiler
	login     string    // Spotify sözleri için bir kerelik giriş adresi
	retryAt   time.Time // girişten sonra yeniden dene
}

type lyricsMsg struct {
	uri    string
	lyrics *lyrics.Lyrics // nil: bulunamadı
	login  string
}

// fetchLyrics, çalan şarkının sözleri henüz istenmediyse ister.
func (m Model) fetchLyrics() tea.Cmd {
	it := m.state.Item
	if it == nil || it.EntityType != "track" || it.URI == "" || m.rt.lyricsAsked[it.URI] {
		return nil
	}
	name, artist, album := m.describe(it)
	if name == "" || artist == "" {
		return nil // adlar Web API'den gelince yeniden denenir
	}
	m.rt.lyricsAsked[it.URI] = true
	track := lyrics.Track{ID: spotify.IDFromURI(it.URI), Name: name, Artist: artist, Album: album, Duration: it.Duration()}
	uri := it.URI
	br := m.rt.bridge
	if !m.cfg.Extras.Enabled {
		br = nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		msg := lyricsMsg{uri: uri}
		if br != nil {
			l, login := fromSpotify(ctx, br, track.ID)
			if l != nil {
				msg.lyrics = l
				return msg
			}
			msg.login = login
		}
		if l, err := lyrics.FromLRCLIB(ctx, track); err == nil {
			msg.lyrics = l
		}
		return msg
	}
}

// fromSpotify, köprüden jeton alıp Spotify'ın sözlerini ister. Jeton
// reddedilirse bir kez yenisini dener. Giriş gerekiyorsa adresini döner.
func fromSpotify(ctx context.Context, br *bridge.Bridge, id string) (*lyrics.Lyrics, string) {
	for try := 0; try < 2; try++ {
		tok, err := br.Token(ctx)
		var need *bridge.LoginNeeded
		if errors.As(err, &need) {
			return nil, need.URL
		}
		if err != nil {
			return nil, ""
		}
		l, err := lyrics.FromSpotify(ctx, tok, id)
		if errors.Is(err, lyrics.ErrUnauthorized) {
			br.Invalidate()
			continue
		}
		return l, ""
	}
	return nil, ""
}

func (m Model) updateLyrics(msg lyricsMsg) (Model, tea.Cmd) {
	m.rt.lyrics[msg.uri] = msg.lyrics
	m.lyr.login = msg.login
	return m, nil
}

// lyricsTick, girişten sonra sözleri yeniden ister.
func (m Model) lyricsTick() Model {
	if !m.lyr.retryAt.IsZero() && m.now.After(m.lyr.retryAt) {
		m.lyr.retryAt = time.Time{}
		if it := m.state.Item; it != nil {
			delete(m.rt.lyricsAsked, it.URI)
		}
	}
	return m
}

// currentLyrics, çalan şarkının sözleri ve henüz yüklenip yüklenmediği.
func (m Model) currentLyrics() (l *lyrics.Lyrics, known bool) {
	it := m.state.Item
	if it == nil {
		return nil, true
	}
	l, known = m.rt.lyrics[it.URI]
	return l, known
}

// lyricsRows, sözleri h satıra sığdırır: söylenen satır üçte bir yükseklikte durur.
func (m Model) lyricsRows(w, h int) []string {
	l, known := m.currentLyrics()
	switch {
	case m.lyr.login != "" && l == nil:
		return []string{theme.Dim.Render(truncate(T("lyr.login"), w)), theme.Faint.Render(truncate(T("lyr.login.how"), w))}
	case !known:
		return []string{theme.Faint.Render(T("lyr.loading"))}
	case l == nil:
		return []string{theme.Faint.Render(T("lyr.none"))}
	}
	body := max(h-2, 1) // son iki satır: boşluk ve kaynak
	cur := l.At(m.position())
	start := 0
	if l.Synced {
		start = max(0, cur-body/3)
		start = min(start, max(len(l.Lines)-body, 0))
	}
	var rows []string
	for i := start; i < len(l.Lines) && len(rows) < body; i++ {
		text := truncate(l.Lines[i].Text, w)
		switch {
		case !l.Synced:
			rows = append(rows, theme.Plain.Render(text))
		case i == cur:
			rows = append(rows, theme.Hi.Bold(true).Render(text))
		case i < cur:
			rows = append(rows, theme.Faint.Render(text))
		default:
			rows = append(rows, theme.Dim.Render(text))
		}
	}
	return append(rows, "", theme.Faint.Italic(true).Render(truncate(T("lyr.source")+l.Source, w)))
}

// openLyricsLogin, Spotify sözleri için tarayıcıda girişi açar.
func (m Model) openLyricsLogin() (Model, tea.Cmd) {
	if m.lyr.login == "" {
		return m, nil
	}
	url := m.lyr.login
	m.lyr.retryAt = m.now.Add(15 * time.Second)
	return m.note(T("lyr.opened")), func() tea.Msg {
		if err := openBrowser(url); err != nil {
			return flashMsg(err.Error())
		}
		return nil
	}
}
