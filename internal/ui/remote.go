package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/soloist"
	"github.com/Ehtisam/sotto/internal/spotify"
)

// Kumanda modu: Soloist kapalıyken çalma durumu Web API'den sorulur ve
// komutlar o an etkin olan cihaza (telefon, Spotify uygulaması…) gider.

const pollEvery = 1500 * time.Millisecond

type remoteState struct {
	active   bool   // Web API'den gelen bir durum gösteriliyor
	device   string // çalan cihazın adı
	polling  bool
	lastPoll time.Time
	lastItem string // sırayı yalnızca şarkı değişince yeniden sormak için

	// player: son Web API durumu; cihazın kısıtları (ses, karıştırma, tekrar) buradan okunur.
	player *spotify.PlayerState
}

type pollMsg struct {
	st  *spotify.PlayerState
	at  time.Time
	err error
}

type queuePollMsg struct{ tracks []spotify.Track }

// pollSoonMsg: bir komuttan sonra durumu hemen yenile.
type pollSoonMsg struct{}

// shouldPoll: müzik Soloist'ten çalmıyorsa durum Web API'den izlenir.
func (m Model) shouldPoll() bool { return m.spState == spReady && !m.soloistActive() }

func (m Model) maybePoll() (Model, tea.Cmd) {
	if !m.shouldPoll() {
		m.remote.active = false
		return m, nil
	}
	if m.remote.polling || m.now.Sub(m.remote.lastPoll) < pollEvery {
		return m, nil
	}
	return m.poll()
}

func (m Model) poll() (Model, tea.Cmd) {
	m.remote.polling, m.remote.lastPoll = true, m.now
	sp := m.sp
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		st, err := sp.Player(ctx)
		return pollMsg{st: st, at: time.Now(), err: err}
	}
}

func (m Model) pollQueue() tea.Cmd {
	sp := m.sp
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		q, _ := sp.Queue(ctx)
		return queuePollMsg{q}
	}
}

func (m Model) updateRemote(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case pollSoonMsg:
		if !m.shouldPoll() {
			return m, nil
		}
		// Spotify'ın komutu işlemesine kısa bir süre tanı.
		m.remote.lastPoll = m.now.Add(-pollEvery + 400*time.Millisecond)
		return m, nil

	case pollMsg:
		m.remote.polling = false
		if !m.shouldPoll() {
			return m, nil // bu arada müzik Soloist'e geçti
		}
		if msg.err != nil {
			return m.spotifyFailed(msg.err), nil
		}
		if msg.st == nil {
			m.remote.active, m.remote.device, m.remote.player = false, "", nil
			m.state, m.queue = soloist.PlaybackState{}, soloist.QueueChanged{}
			return m, nil
		}
		m.remote.active, m.remote.device = true, msg.st.Device.Name
		m.remote.player = msg.st
		if msg.st.Device.ID != "" && msg.st.Device.Name != m.cfg.Soloist.DeviceName {
			m.devs.switchTo = msg.st.Device.ID // t tuşu buraya geri döner
		}
		m.state = m.stateFromAPI(msg.st, msg.at)
		var cmd tea.Cmd
		if uri := m.state.Item.URIOrEmpty(); uri != m.remote.lastItem {
			m.remote.lastItem = uri
			m.qs.polling = true
			cmd = m.pollQueue()
		}
		return m, cmd

	case queuePollMsg:
		m.qs.polling, m.qs.fetched = false, m.now
		q := soloist.QueueChanged{}
		for _, t := range msg.tracks {
			q.Upcoming = append(q.Upcoming, soloist.QueueEntry{UID: t.URI, Source: "queue", Item: entityFromTrack(&t)})
		}
		m.queue = q
		for _, t := range msg.tracks {
			m.rt.known[t.URI] = t
		}
	}
	return m, nil
}

// stateFromAPI, Web API'nin yanıtını Soloist'in durum biçimine çevirir;
// böylece ekranın geri kalanı ikisini ayırt etmeden çizer.
func (m Model) stateFromAPI(p *spotify.PlayerState, at time.Time) soloist.PlaybackState {
	st := soloist.PlaybackState{
		Status:   soloist.StatusPaused,
		Item:     entityFromTrack(p.Item),
		IsActive: false,
		Options:  soloist.PlaybackOptions{Shuffle: p.ShuffleState, Repeat: p.RepeatState},
		Position: soloist.Position{PositionMS: p.ProgressMS, TimestampMS: at.UnixMilli()},
	}
	if p.IsPlaying {
		st.Status = soloist.StatusPlaying
		st.Position.Speed = 1
	}
	if p.Device.Volume != nil {
		st.Volume = *p.Device.Volume
	}
	if p.Context != nil && p.Context.URI != "" {
		e := &soloist.Entity{URI: p.Context.URI, EntityType: p.Context.Type}
		e.Decorations.Identity.Name = m.contextName(p.Context.URI, p.Context.Type)
		st.Context = e
	}
	return st
}

// contextName, bağlamın adını kitaplıktan bulur (Web API onu vermiyor).
func (m Model) contextName(uri, kind string) string {
	if kind == "collection" {
		return m.likedName()
	}
	if name, ok := m.rt.contextNames[uri]; ok {
		return name
	}
	for _, e := range m.lib.entries {
		if e.uri == uri {
			return e.name
		}
	}
	return ""
}

func entityFromTrack(t *spotify.Track) *soloist.Entity {
	if t == nil {
		return nil
	}
	e := &soloist.Entity{URI: t.URI, EntityType: "track"}
	if t.Type == "episode" {
		e.EntityType = "episode"
	}
	e.Decorations.Identity.Name = t.Name
	e.Decorations.Playback.DurationMS = int64(t.DurationMS)
	if t.Explicit {
		e.Decorations.Playback.ContentRatings = []string{"explicit"}
	}
	for _, a := range t.Artists {
		c := struct {
			Entity soloist.Entity `json:"entity"`
		}{}
		c.Entity.URI = a.URI
		c.Entity.Decorations.Identity.Name = a.Name
		e.Decorations.Creators = append(e.Decorations.Creators, c)
	}
	if u := pickImage(t.Album.Images); u != "" {
		e.Decorations.VisualIdentity.Cover = []soloist.Image{{URL: u, Size: "default"}}
	}
	if t.Album.Name != "" {
		e.Decorations.Parent = &struct {
			Entity soloist.Entity `json:"entity"`
		}{}
		e.Decorations.Parent.Entity.Decorations.Identity.Name = t.Album.Name
		e.Decorations.Parent.Entity.URI = t.Album.URI
	}
	return e
}
