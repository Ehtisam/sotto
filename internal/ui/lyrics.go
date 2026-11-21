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

	// Satır seçme (Şimdi çalıyor'da enter ile girilir): ↑ ↓ satır seçer,
	// enter o satırın başına sarar. pickOn değilken sözler şarkıyı izler.
	pickOn  bool
	pick    int
	pickURI string // seçimin yapıldığı şarkı; şarkı değişince seçim düşer
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
	if _, ok := m.rt.lyrics[it.URI]; ok {
		return nil // zaten biliniyor
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
	pick, picking := m.lyricsPick()
	start := 0
	if l.Synced {
		anchor := cur
		if picking {
			anchor = pick
		}
		start = max(0, anchor-body/3)
		start = min(start, max(len(l.Lines)-body, 0))
	}
	var rows []string
	for i := start; i < len(l.Lines) && len(rows) < body; i++ {
		text := truncate(l.Lines[i].Text, w)
		switch {
		case !l.Synced:
			rows = append(rows, theme.Plain.Render(text))
		case picking && i == pick:
			if text == "" {
				text = "♪"
			}
			rows = append(rows, theme.SelectedRow.Render(text))
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

// ---- satıra atlama ----

// lyricsPickable: Şimdi çalıyor'da eş zamanlı sözler görünüyor.
func (m Model) lyricsPickable() bool {
	l, _ := m.currentLyrics()
	return !m.lyr.showQueue && l != nil && l.Synced && len(l.Lines) > 0
}

// lyricsPick, seçili satır ve seçimin geçerli olup olmadığı.
func (m Model) lyricsPick() (int, bool) {
	l, _ := m.currentLyrics()
	if !m.lyr.pickOn || m.focus != focusContent || m.page != pageNow || l == nil || !l.Synced ||
		m.state.Item == nil || m.lyr.pickURI != m.state.Item.URI {
		return 0, false
	}
	return max(0, min(m.lyr.pick, len(l.Lines)-1)), true
}

// nowKey: Şimdi çalıyor'da içerik odaktayken (sözlerde satır seçme).
func (m Model) nowKey(k string) (Model, tea.Cmd, bool) {
	if !m.lyricsPickable() {
		if k == "esc" || k == "backspace" {
			m.focus = focusMenu
			return m, nil, true
		}
		return m, nil, false
	}
	l, _ := m.currentLyrics()
	pick, picking := m.lyricsPick()
	if !picking {
		pick = max(0, l.At(m.position())) // seçim söylenen satırdan başlar
	}
	move := func(d int) (Model, tea.Cmd, bool) {
		m.lyr.pickOn, m.lyr.pickURI = true, m.state.Item.URI
		m.lyr.pick = max(0, min(len(l.Lines)-1, pick+d))
		return m, nil, true
	}
	switch k {
	case "up", "k":
		if !picking {
			return move(0)
		}
		return move(-1)
	case "down", "j":
		if !picking {
			return move(0)
		}
		return move(1)
	case "pgup":
		return move(-5)
	case "pgdown":
		return move(5)
	case "home":
		return move(-len(l.Lines))
	case "G", "end":
		return move(len(l.Lines))
	case "enter":
		if !picking {
			return move(0)
		}
		m.lyr.pickOn = false // atladıktan sonra sözler yine şarkıyı izler
		return m, m.seekTo(l.Lines[pick].Start), true
	case "esc", "backspace":
		if picking {
			m.lyr.pickOn = false
			return m, nil, true
		}
		m.focus = focusMenu
		return m, nil, true
	}
	return m, nil, false
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
