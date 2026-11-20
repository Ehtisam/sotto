// Package ui, sotto'nun Bubble Tea arayüzü.
package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/config"
	"github.com/Ehtisam/sotto/internal/soloist"
)

type page int

const (
	pageNow page = iota
	pageHome
	pageSearch
	pageLibrary
	pageQueue
	pageDevices
	pageProfile
	pageSettings
)

var pages = []struct{ numeral, name, soon string }{
	{"I", "Şimdi çalıyor", ""},
	{"II", "Ana sayfa", "kaldığın yerden devam, son çalınanlar, en çok dinlenenler"},
	{"III", "Ara", "şarkı, albüm, sanatçı, çalma listesi, podcast"},
	{"IV", "Kitaplık", "listeler, beğenilenler, albümler, sanatçılar, podcast'ler"},
	{"V", "Sıra", "sıradakiler ve son çalınanlar"},
	{"VI", "Cihazlar", "Soloist, Spotify uygulaması, telefon"},
	{"VII", "Profil", "fotoğraf, takipçiler, en çok dinlenenler"},
	{"VIII", "Ayarlar", "renkler, temalar, kısayollar, hoparlör"},
}

type engineState int

const (
	engineOff engineState = iota
	engineStarting
	engineReady
	engineFailed
)

// runtime, modelin kopyaları arasında paylaşılan, kapatılması gereken kaynaklar.
type runtime struct {
	proc   *soloist.Process
	client *soloist.Client
}

// Model, sotto'nun bütün arayüz durumu.
type Model struct {
	cfg config.Config
	rt  *runtime

	width, height int
	page          page
	now           time.Time

	engine    engineState
	engineErr error

	auth     soloist.AuthState
	haveAuth bool
	state    soloist.PlaybackState
	queue    soloist.QueueChanged

	flash   string
	flashAt time.Time
}

// New, ayarlarla yeni bir model kurar.
func New(cfg config.Config) Model {
	m := Model{cfg: cfg, rt: &runtime{}, now: time.Now()}
	if cfg.Soloist.Enabled {
		m.engine = engineStarting
	}
	return m
}

// Shutdown, Soloist'i ve bağlantıyı kapatır. sotto çıkarken çağrılır.
func (m Model) Shutdown() {
	if m.rt.client != nil {
		m.rt.client.Close()
	}
	if m.rt.proc != nil {
		m.rt.proc.Stop()
	}
}

// ---- mesajlar ----

type tickMsg time.Time

type engineUpMsg struct {
	proc   *soloist.Process
	client *soloist.Client
}

type engineErrMsg struct{ err error }

type engineEventMsg struct{ ev any }

type engineDownMsg struct{ err error }

type flashMsg string

func tick() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func startSoloist(cfg config.Config) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		p, err := soloist.Start(ctx, soloist.Options{
			Path:          cfg.SoloistPath(),
			APIKey:        cfg.SoloistAPIKey(),
			DeviceName:    cfg.Soloist.DeviceName,
			DataDir:       cfg.SoloistDataDir(),
			CacheDir:      cfg.SoloistCacheDir(),
			LogPath:       cfg.SoloistLog(),
			InitialVolume: cfg.Soloist.InitialVolume,
		})
		if err != nil {
			return engineErrMsg{err}
		}
		c, err := soloist.Dial(ctx, p.Addr)
		if err != nil {
			p.Stop()
			return engineErrMsg{err}
		}
		return engineUpMsg{p, c}
	}
}

func listen(c *soloist.Client) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-c.Events()
		if !ok {
			return engineDownMsg{}
		}
		if d, ok := ev.(soloist.Disconnected); ok {
			return engineDownMsg{d.Err}
		}
		return engineEventMsg{ev}
	}
}

// do, Soloist'e bir komut gönderir; hata olursa alt satırda kısa bir not gösterilir.
func (m Model) do(f func(context.Context, *soloist.Client) error) tea.Cmd {
	c := m.rt.client
	if c == nil {
		return func() tea.Msg { return flashMsg("Soloist bağlı değil") }
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := f(ctx, c); err != nil {
			return flashMsg(err.Error())
		}
		return nil
	}
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{tick()}
	if m.cfg.Soloist.Enabled {
		cmds = append(cmds, startSoloist(m.cfg))
	}
	return tea.Batch(cmds...)
}

// ---- güncelleme ----

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tickMsg:
		m.now = time.Time(msg)
		if m.flash != "" && m.now.Sub(m.flashAt) > 4*time.Second {
			m.flash = ""
		}
		return m, tick()

	case engineUpMsg:
		m.rt.proc, m.rt.client = msg.proc, msg.client
		m.engine, m.engineErr = engineReady, nil
		return m, listen(msg.client)

	case engineErrMsg:
		m.engine, m.engineErr = engineFailed, msg.err
		return m, nil

	case engineDownMsg:
		m.engine, m.engineErr = engineFailed, msg.err
		if m.engineErr == nil {
			m.engineErr = errDisconnected
		}
		return m, nil

	case engineEventMsg:
		cmd := m.apply(msg.ev)
		return m, tea.Batch(listen(m.rt.client), cmd)

	case flashMsg:
		m.flash, m.flashAt = string(msg), m.now
		return m, nil

	case tea.KeyPressMsg:
		return m.key(msg)
	}
	return m, nil
}

type disconnectedErr struct{}

func (disconnectedErr) Error() string { return "Soloist ile bağlantı koptu" }

var errDisconnected error = disconnectedErr{}

// apply, Soloist'ten gelen bir olayı duruma işler.
func (m *Model) apply(ev any) tea.Cmd {
	switch e := ev.(type) {
	case soloist.AuthState:
		wasLoggedIn := m.auth.LoggedIn
		m.auth, m.haveAuth = e, true
		m.state.IsActive = e.IsActive
		if e.LoggedIn && !wasLoggedIn {
			return m.do(func(ctx context.Context, c *soloist.Client) error {
				if err := c.RequestState(ctx); err != nil {
					return err
				}
				return c.RequestQueue(ctx, 20)
			})
		}
	case soloist.PlaybackState:
		m.state = e
	case soloist.TrackChanged:
		m.state.Item = e.Item
	case soloist.StatusChanged:
		m.state.Status = e.Status
	case soloist.VolumeChanged:
		m.state.Volume = e.Volume
	case soloist.DeviceChanged:
		m.auth.IsActive, m.state.IsActive = e.IsActive, e.IsActive
		if e.DeviceName != "" {
			m.auth.DeviceName = e.DeviceName
		}
	case soloist.ContextChanged:
		m.state.Context = e.Context
	case soloist.OptionsChanged:
		m.state.Options = e.Options
	case soloist.PositionSync:
		m.state.Position = e.Position
	case soloist.QueueChanged:
		m.queue = e
	case soloist.APIError:
		m.flash, m.flashAt = e.Message, m.now
	}
	return nil
}

func (m Model) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch k := msg.String(); k {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "up", "k":
		m.page = (m.page + page(len(pages)) - 1) % page(len(pages))
	case "down", "j":
		m.page = (m.page + 1) % page(len(pages))
	case "1", "2", "3", "4", "5", "6", "7", "8":
		m.page = page(k[0] - '1')

	case "space", " ":
		if m.state.Status == soloist.StatusPlaying || m.state.Status == soloist.StatusBuffering {
			return m, m.do(func(ctx context.Context, c *soloist.Client) error { return c.Pause(ctx) })
		}
		return m, m.do(func(ctx context.Context, c *soloist.Client) error { return c.Play(ctx, "") })
	case "n":
		return m, m.do(func(ctx context.Context, c *soloist.Client) error { return c.Next(ctx) })
	case "p":
		return m, m.do(func(ctx context.Context, c *soloist.Client) error { return c.Previous(ctx) })
	case "left", "right":
		step := 10 * time.Second
		if k == "left" {
			step = -step
		}
		target := m.position() + step
		return m, m.do(func(ctx context.Context, c *soloist.Client) error { return c.Seek(ctx, target.Milliseconds()) })
	case "+", "=":
		v := m.state.Volume + 5
		return m, m.do(func(ctx context.Context, c *soloist.Client) error { return c.SetVolume(ctx, v) })
	case "-":
		v := m.state.Volume - 5
		return m, m.do(func(ctx context.Context, c *soloist.Client) error { return c.SetVolume(ctx, v) })
	case "s":
		on := !m.state.Options.Shuffle
		return m, m.do(func(ctx context.Context, c *soloist.Client) error { return c.SetShuffle(ctx, on) })
	case "r":
		next := map[string]string{"off": "context", "context": "track", "track": "off"}[m.state.Options.Repeat]
		if next == "" {
			next = "context"
		}
		return m, m.do(func(ctx context.Context, c *soloist.Client) error { return c.SetRepeat(ctx, next) })
	case "a":
		return m, m.do(func(ctx context.Context, c *soloist.Client) error { return c.Activate(ctx) })
	}
	return m, nil
}

// position, çalan şarkının şu anki tahmini konumu.
func (m Model) position() time.Duration {
	pos := m.state.Position.At(m.now)
	if d := m.state.Item.Duration(); d > 0 && pos > d {
		pos = d
	}
	return pos
}
