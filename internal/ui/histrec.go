package ui

import (
	"time"

	"github.com/Ehtisam/sotto/internal/history"
)

// Dinleme kaydı: saat her tıkladığında çalan şarkının dinlenen süresi
// biriktirilir (yalnız çalarken); şarkı değişince ya da sotto kapanınca
// yeterince dinlendiyse (history.Counts) geçmiş dosyasına yazılır. Müzik
// hangi cihazda çalarsa çalsın sotto'nun gördüğü her şey sayılır.

type histRec struct {
	play     history.Play
	duration time.Duration
	played   time.Duration
	lastTick time.Time
}

// maxTickGap: iki tık arası bundan uzunsa (bilgisayar uyudu) sayılmaz.
const maxTickGap = 3 * time.Second

// histTick, saat tıkında çağrılır.
func (m Model) histTick() {
	h := &m.rt.hist
	it := m.state.Item
	uri := ""
	if it != nil && (it.EntityType == "track" || it.EntityType == "episode" || it.EntityType == "") {
		uri = it.URI
	}
	if uri != h.play.URI {
		m.histFlush()
		if uri != "" {
			name, artists, album := m.describe(it)
			p := history.Play{At: m.now, URI: uri, Title: name, Artist: artists, Album: album, Device: m.histDevice()}
			if ctx := m.state.Context; ctx != nil {
				p.Context = ctx.URI
			}
			*h = histRec{play: p, duration: it.Duration(), lastTick: m.now}
		}
		return
	}
	if uri == "" {
		return
	}
	// Adlar sonradan (Web API'den) gelir: boşsa tamamla.
	if h.play.Title == "" || h.play.Artist == "" {
		h.play.Title, h.play.Artist, h.play.Album = m.describe(it)
	}
	if gap := m.now.Sub(h.lastTick); m.playing() && gap > 0 && gap < maxTickGap {
		h.played += gap
	}
	h.lastTick = m.now
}

func (m Model) histDevice() string {
	if m.remote.active {
		return m.remote.device
	}
	return m.auth.DeviceName
}

// histFlush, süren dinlemeyi (sayılıyorsa) dosyaya yazar ve sıfırlar.
func (m Model) histFlush() {
	h := &m.rt.hist
	if h.play.URI != "" && h.play.Title != "" && history.Counts(h.played, h.duration) {
		h.play.MS = h.played.Milliseconds()
		if t, ok := m.rt.known[h.play.URI]; ok && len(t.Artists) > 0 {
			h.play.ArtistURI = t.Artists[0].URI
		}
		if err := history.Append(m.cfg.LocalDir, h.play); err != nil {
			m.rt.logs.add(logError, "history: "+err.Error(), m.now)
		}
	}
	*h = histRec{}
}
