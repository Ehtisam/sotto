// Package ui, sotto'nun Bubble Tea arayüzü.
package ui

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/bridge"
	"github.com/Ehtisam/sotto/internal/config"
	"github.com/Ehtisam/sotto/internal/cover"
	"github.com/Ehtisam/sotto/internal/i18n"
	"github.com/Ehtisam/sotto/internal/lyrics"
	"github.com/Ehtisam/sotto/internal/soloist"
	"github.com/Ehtisam/sotto/internal/spotify"
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
	{"I", "page.now", ""},
	{"II", "page.home", "soon.home"},
	{"III", "page.search", "soon.search"},
	{"IV", "page.library", ""},
	{"V", "page.queue", "soon.queue"},
	{"VI", "page.devices", "soon.devices"},
	{"VII", "page.profile", "soon.profile"},
	{"VIII", "page.settings", "soon.settings"},
}

// hasContent: içine girilip gezinilebilen sayfalar.
func (p page) hasContent() bool {
	return p == pageHome || p == pageLibrary || p == pageDevices || p == pageSearch
}

type focusArea int

const (
	focusMenu focusArea = iota
	focusContent
)

type engineState int

const (
	engineOff engineState = iota
	engineStarting
	engineReady
	engineFailed
)

type spotifyState int

const (
	spNoClient  spotifyState = iota // ayarlarda Client ID yok
	spLoggedOut                     // giriş yapılmamış
	spLoggingIn                     // tarayıcıda onay bekleniyor
	spReady
)

// shared, modelin kopyaları arasında paylaşılan kaynaklar. Haritalar
// yalnızca Update içinde değiştirilir.
type shared struct {
	proc   *soloist.Process
	client *soloist.Client

	known   map[string]spotify.Track // URI -> şarkı bilgisi (Soloist'in eksik bıraktıkları için)
	pending map[string]bool

	covers     *cover.Store
	coverAsked map[string]bool

	bridge      *bridge.Bridge
	lyrics      map[string]*lyrics.Lyrics // URI -> sözler (nil: bulunamadı)
	lyricsAsked map[string]bool
}

// Model, sotto'nun bütün arayüz durumu.
type Model struct {
	cfg config.Config
	rt  *shared
	sp  *spotify.Client

	width, height int
	page          page
	focus         focusArea
	now           time.Time

	engine    engineState
	engineErr error

	auth     soloist.AuthState
	haveAuth bool

	// Çalma durumu. Soloist bağlıyken ondan, değilken Web API'den (kumanda) gelir.
	state  soloist.PlaybackState
	queue  soloist.QueueChanged
	remote remoteState

	spState spotifyState
	spErr   error
	me      spotify.User

	lib    library
	search searchState
	home   homeState
	devs   deviceList
	help   bool // ? ile açılan kısayol listesi
	lyr    lyricsState

	flash     string
	flashAt   time.Time
	flashInfo bool // hata değil, bilgi notu
}

// New, ayarlarla yeni bir model kurar.
func New(cfg config.Config) Model {
	m := Model{
		cfg: cfg,
		rt: &shared{
			known: map[string]spotify.Track{}, pending: map[string]bool{},
			covers: cover.NewStore(filepath.Join(cfg.LocalDir, "covers")), coverAsked: map[string]bool{},
			bridge: bridge.New(filepath.Join(cfg.LocalDir, "bridge")),
			lyrics: map[string]*lyrics.Lyrics{}, lyricsAsked: map[string]bool{},
		},
		sp:  spotify.NewClient(cfg.Spotify.ClientID, cfg.TokenFile()),
		now: time.Now(),
	}
	if cfg.Soloist.Autostart {
		m.engine = engineStarting
	} else {
		m.engine = engineOff
	}
	switch {
	case !m.sp.HasClientID():
		m.spState = spNoClient
	case m.sp.HasLogin():
		m.spState = spReady
		m.lib.loading = true // Init kitaplığı ve ana sayfayı yüklemeye başlar
		m.home.loading = true
	default:
		m.spState = spLoggedOut
	}
	return m
}

// Shutdown, sotto'nun başlattığı Soloist'i ve bağlantıyı kapatır.
func (m Model) Shutdown() {
	m.rt.bridge.Close()
	if m.rt.client != nil {
		m.rt.client.Close()
	}
	if m.rt.proc != nil {
		m.rt.proc.Stop() // başka bir pencerenin Soloist'iyse dokunmaz
	}
}

// soloistLive: Soloist bağlı ve hesaba girmiş; çalma durumu ondan gelir.
func (m Model) soloistLive() bool {
	return m.rt.client != nil && m.engine == engineReady && m.auth.LoggedIn
}

// soloistActive: müzik şu an Soloist'ten çalıyor; komutlar ona gider.
func (m Model) soloistActive() bool { return m.soloistLive() && m.auth.IsActive }

// ---- mesajlar ----

type tickMsg time.Time

type engineUpMsg struct {
	proc   *soloist.Process
	client *soloist.Client
}

type engineErrMsg struct{ err error }

// Olaylar hangi bağlantıdan geldiğini taşır; kapatılmış bir bağlantının
// geç kalan olayları yok sayılır.
type engineEventMsg struct {
	c  *soloist.Client
	ev any
}

type engineDownMsg struct {
	c   *soloist.Client
	err error
}

type flashMsg string

type loginDoneMsg struct{ err error }

type meMsg struct {
	user spotify.User
	err  error
}

type enrichMsg struct{ tracks []spotify.Track }

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
			return engineDownMsg{c: c}
		}
		if d, ok := ev.(soloist.Disconnected); ok {
			return engineDownMsg{c, d.Err}
		}
		return engineEventMsg{c, ev}
	}
}

// do, Soloist'e bir komut gönderir; hata olursa alt satırda kısa bir not gösterilir.
func (m Model) do(f func(context.Context, *soloist.Client) error) tea.Cmd {
	c := m.rt.client
	if c == nil {
		return func() tea.Msg { return flashMsg(i18n.T("flash.nosoloist")) }
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

// control, bir çalma komutunu doğru yere gönderir: müzik Soloist'ten
// çalıyorsa ona, başka bir cihazda çalıyorsa Web API ile o cihaza.
func (m Model) control(viaSoloist func(context.Context, *soloist.Client) error, viaAPI func(context.Context, *spotify.Client) error) tea.Cmd {
	if m.soloistActive() {
		return m.do(viaSoloist)
	}
	if m.spState != spReady {
		if m.soloistLive() {
			return m.do(viaSoloist)
		}
		return func() tea.Msg { return flashMsg(i18n.T("flash.noaccount")) }
	}
	sp := m.sp
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := viaAPI(ctx, sp); err != nil {
			if spotify.IsStatus(err, 404) {
				return flashMsg(i18n.T("flash.nodevice"))
			}
			return flashMsg(err.Error())
		}
		return pollSoonMsg{}
	}
}

func (m Model) login() tea.Cmd {
	sp := m.sp
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		return loginDoneMsg{sp.Login(ctx, openBrowser)}
	}
}

func (m Model) loadMe() tea.Cmd {
	sp := m.sp
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		u, err := sp.Me(ctx)
		return meMsg{u, err}
	}
}

func openBrowser(u string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	case "darwin":
		return exec.Command("open", u).Start()
	default:
		return exec.Command("xdg-open", u).Start()
	}
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{tick()}
	if m.cfg.Soloist.Autostart {
		cmds = append(cmds, startSoloist(m.cfg))
	}
	if m.spState == spReady {
		cmds = append(cmds, m.loadMe(), m.loadLibrary(), m.loadHome())
	}
	return tea.Batch(cmds...)
}

// ---- güncelleme ----

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.lib.clamp(m.listHeight())
		return m, nil

	case tickMsg:
		m.now = time.Time(msg)
		if m.flash != "" && m.now.Sub(m.flashAt) > 5*time.Second {
			m.flash = ""
		}
		var cmd, dcmd tea.Cmd
		m = m.lyricsTick()
		m, cmd = m.maybePoll()
		if m.page == pageDevices && !m.devs.loading && m.now.Sub(m.devs.fetched) > 5*time.Second {
			if dcmd = m.loadDevices(); dcmd != nil {
				m.devs.loading = true
			}
		}
		return m, tea.Batch(tick(), cmd, dcmd, m.fetchCovers(), m.fetchLyrics())

	case engineUpMsg:
		m.rt.proc, m.rt.client = msg.proc, msg.client
		m.engine, m.engineErr = engineReady, nil
		return m, listen(msg.client)

	case engineErrMsg:
		m.engine, m.engineErr = engineFailed, msg.err
		return m, nil

	case engineDownMsg:
		if msg.c != m.rt.client {
			return m, nil // bilerek kapatılan eski bağlantı
		}
		m.engine, m.engineErr = engineFailed, msg.err
		if m.engineErr == nil {
			m.engineErr = errDisconnected
		}
		m.rt.client, m.rt.proc = nil, nil
		m.auth, m.haveAuth = soloist.AuthState{}, false
		return m, nil

	case engineEventMsg:
		if msg.c != m.rt.client {
			return m, nil
		}
		cmd := m.apply(msg.ev)
		return m, tea.Batch(listen(m.rt.client), cmd, m.enrich())

	case flashMsg:
		return m.say(string(msg)), nil

	case loginDoneMsg:
		if msg.err != nil {
			m.spState, m.spErr = spLoggedOut, msg.err
			return m.say(msg.err.Error()), nil
		}
		m.spState, m.spErr = spReady, nil
		m.lib.loading, m.home.loading = true, true
		return m, tea.Batch(m.loadMe(), m.loadLibrary(), m.loadHome(), m.enrich())

	case meMsg:
		if msg.err != nil {
			return m.spotifyFailed(msg.err), nil
		}
		m.me = msg.user
		return m, nil

	case enrichMsg:
		for _, t := range msg.tracks {
			m.rt.known[t.URI] = t
		}
		return m, nil

	case pollMsg, queuePollMsg, pollSoonMsg:
		return m.updateRemote(msg)

	case playlistsMsg, tracksPageMsg, playedMsg:
		return m.updateLibrary(msg)

	case devicesMsg, transferredMsg:
		return m.updateDevices(msg)

	case searchFireMsg, searchResultMsg:
		return m.updateSearch(msg)

	case homeMsg:
		return m.updateHome(msg)

	case lyricsMsg:
		return m.updateLyrics(msg)

	case coverMsg:
		return m, nil // kapak artık bellekte; bir sonraki çizimde görünür

	case tea.KeyPressMsg:
		return m.key(msg)
	}
	return m, nil
}

// say, alt satırda kısa bir hata notu gösterir.
func (m Model) say(s string) Model {
	m.flash, m.flashAt, m.flashInfo = s, m.now, false
	return m
}

// note, alt satırda kısa bir bilgi notu gösterir.
func (m Model) note(s string) Model {
	m.flash, m.flashAt, m.flashInfo = s, m.now, true
	return m
}

// spotifyFailed, girişin geçersiz olduğu anlaşılırsa yeniden giriş ister.
func (m Model) spotifyFailed(err error) Model {
	if errors.Is(err, spotify.ErrNotLoggedIn) {
		m.spState = spLoggedOut
	}
	m.spErr = err
	return m.say(err.Error())
}

type disconnectedErr struct{}

func (disconnectedErr) Error() string { return "lost the connection to soloist" }

var errDisconnected error = disconnectedErr{}

// apply, Soloist'ten gelen bir olayı duruma işler. Müzik başka bir
// cihazdayken çalma olaylarını yok sayar; o durumda durum Web API'den gelir.
func (m *Model) apply(ev any) tea.Cmd {
	switch ev.(type) {
	case soloist.PlaybackState, soloist.TrackChanged, soloist.StatusChanged, soloist.VolumeChanged,
		soloist.ContextChanged, soloist.OptionsChanged, soloist.PositionSync, soloist.QueueChanged:
		if m.shouldPoll() {
			return nil
		}
	}
	switch e := ev.(type) {
	case soloist.AuthState:
		wasLoggedIn := m.auth.LoggedIn
		m.auth, m.haveAuth = e, true
		m.state.IsActive = e.IsActive
		if e.LoggedIn && !wasLoggedIn {
			cmds := []tea.Cmd{m.do(func(ctx context.Context, c *soloist.Client) error {
				if err := c.RequestState(ctx); err != nil {
					return err
				}
				return c.RequestQueue(ctx, 20)
			})}
			// Soloist kullanıcı istediği için açıldıysa müziği ona al.
			if m.devs.pending != "" && m.spState == spReady {
				cmds = append(cmds, m.transferByName(m.devs.pending))
				m.devs.pending = ""
			}
			return tea.Batch(cmds...)
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
		became := e.IsActive && !m.auth.IsActive
		m.auth.IsActive, m.state.IsActive = e.IsActive, e.IsActive
		if e.DeviceName != "" {
			m.auth.DeviceName = e.DeviceName
		}
		if became { // müzik Soloist'e geçti: durumu ondan tazele
			m.remote.active = false
			return m.do(func(ctx context.Context, c *soloist.Client) error {
				if err := c.RequestState(ctx); err != nil {
					return err
				}
				return c.RequestQueue(ctx, 20)
			})
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

// enrich, Soloist'in adsız ya da sanatçısız gönderdiği şarkıları Web API'den tamamlar.
func (m Model) enrich() tea.Cmd {
	if m.spState != spReady {
		return nil
	}
	var ids []string
	want := func(e *soloist.Entity) {
		if e == nil || e.EntityType != "track" || e.URI == "" || len(ids) >= 12 {
			return
		}
		if e.Name() != "" && e.Creators() != "" {
			return
		}
		if _, ok := m.rt.known[e.URI]; ok || m.rt.pending[e.URI] {
			return
		}
		m.rt.pending[e.URI] = true
		ids = append(ids, spotify.IDFromURI(e.URI))
	}
	want(m.state.Item)
	for _, q := range m.queue.Upcoming {
		want(q.Item)
	}
	if len(ids) == 0 {
		return nil
	}
	sp := m.sp
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var out []spotify.Track
		for _, id := range ids {
			if t, err := sp.Track(ctx, id); err == nil {
				out = append(out, t)
			}
		}
		return enrichMsg{out}
	}
}

// describe, bir öğenin adını, sanatçısını ve albümünü verir; Soloist'in
// eksik bıraktıklarını Web API'den gelen bilgiyle doldurur.
func (m Model) describe(e *soloist.Entity) (name, artists, album string) {
	if e == nil {
		return "", "", ""
	}
	name, artists, album = e.Name(), e.Creators(), e.ParentName()
	if t, ok := m.rt.known[e.URI]; ok {
		if name == "" {
			name = t.Name
		}
		if artists == "" {
			artists = t.ArtistNames()
		}
		if album == "" {
			album = t.Album.Name
		}
	}
	return name, artists, album
}

func (m Model) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()

	// Kısayol listesi açıkken: esc ya da ? kapatır.
	if m.help {
		switch k {
		case "?", "esc", "q":
			m.help = false
			return m, nil
		}
	}

	// Ara sayfasında yazarken harfler kısayol değil, aranacak metindir.
	if m.focus == focusContent && !m.help && m.page == pageSearch {
		if mm, cmd, ok := m.searchKey(msg); ok {
			return mm, cmd
		}
	}

	// İçerikteyken gezinme tuşları önce sayfaya gider.
	if m.focus == focusContent && !m.help && m.page != pageSearch {
		if mm, cmd, ok := m.contentKey(k); ok {
			return mm, cmd
		}
	}

	playing := m.state.Status == soloist.StatusPlaying || m.state.Status == soloist.StatusBuffering
	switch k {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.help = true
		return m, nil
	case "/":
		m.page, m.focus = pageSearch, focusContent
		m.search.open = nil
		return m.enterSearch(), nil

	case "up", "k":
		m.page = (m.page + page(len(pages)) - 1) % page(len(pages))
	case "down", "j":
		m.page = (m.page + 1) % page(len(pages))
	case "1", "2", "3", "4", "5", "6", "7", "8":
		m.page, m.focus = page(k[0]-'1'), focusMenu
	case "enter", "right", "tab":
		if m.page.hasContent() {
			m.focus = focusContent
			if m.page == pageSearch {
				m = m.enterSearch()
			}
			cmd := m.enterPage()
			return m, cmd
		}

	case "space", " ":
		if playing {
			return m, m.control(
				func(ctx context.Context, c *soloist.Client) error { return c.Pause(ctx) },
				func(ctx context.Context, s *spotify.Client) error { return s.Pause(ctx) })
		}
		return m, m.control(
			func(ctx context.Context, c *soloist.Client) error { return c.Play(ctx, "") },
			func(ctx context.Context, s *spotify.Client) error { return s.Resume(ctx) })
	case "n":
		return m, m.control(
			func(ctx context.Context, c *soloist.Client) error { return c.Next(ctx) },
			func(ctx context.Context, s *spotify.Client) error { return s.Next(ctx) })
	case "p":
		return m, m.control(
			func(ctx context.Context, c *soloist.Client) error { return c.Previous(ctx) },
			func(ctx context.Context, s *spotify.Client) error { return s.Previous(ctx) })
	case ",", "<", ".", ">":
		step := 10 * time.Second
		if k == "," || k == "<" {
			step = -step
		}
		ms := (m.position() + step).Milliseconds()
		return m, m.control(
			func(ctx context.Context, c *soloist.Client) error { return c.Seek(ctx, ms) },
			func(ctx context.Context, s *spotify.Client) error { return s.Seek(ctx, ms) })
	case "+", "=", "-":
		v := m.state.Volume + 5
		if k == "-" {
			v = m.state.Volume - 5
		}
		return m, m.control(
			func(ctx context.Context, c *soloist.Client) error { return c.SetVolume(ctx, v) },
			func(ctx context.Context, s *spotify.Client) error { return s.SetVolume(ctx, v) })
	case "s":
		on := !m.state.Options.Shuffle
		return m, m.control(
			func(ctx context.Context, c *soloist.Client) error { return c.SetShuffle(ctx, on) },
			func(ctx context.Context, s *spotify.Client) error { return s.SetShuffle(ctx, on) })
	case "r":
		next := map[string]string{"off": "context", "context": "track", "track": "off"}[m.state.Options.Repeat]
		if next == "" {
			next = "context"
		}
		return m, m.control(
			func(ctx context.Context, c *soloist.Client) error { return c.SetRepeat(ctx, next) },
			func(ctx context.Context, s *spotify.Client) error { return s.SetRepeat(ctx, next) })
	case "t", "a":
		return m.switchSpeaker()
	case "o":
		return m.toggleSoloist()
	case "l":
		// Sözler Şimdi çalıyor sayfasında: başka yerdeyken oraya götürür,
		// oradayken sözler ile sıradakiler arasında geçer.
		if m.page != pageNow || m.help {
			m.page, m.focus, m.help = pageNow, focusMenu, false
			m.lyr.showQueue = false
		} else {
			m.lyr.showQueue = !m.lyr.showQueue
		}
		if l, known := m.currentLyrics(); known && l == nil && m.state.Item != nil && m.lyr.login == "" {
			return m.note(T("lyr.none")), nil
		}
	case "L":
		return m.openLyricsLogin()
	}
	return m, nil
}

// contentKey, içerik odaktayken sayfanın kendi tuşları. ok=false ise tuş genel kısayollara düşer.
func (m Model) contentKey(k string) (Model, tea.Cmd, bool) {
	if k == "tab" {
		m.focus = focusMenu
		return m, nil, true
	}
	switch m.page {
	case pageHome:
		return m.homeKey(k)
	case pageLibrary:
		return m.libraryKey(k)
	case pageDevices:
		return m.devicesKey(k)
	}
	return m, nil, false
}

// enterPage, bir sayfaya girilince gerekenleri başlatır.
func (m *Model) enterPage() tea.Cmd {
	if m.page == pageLibrary && m.spState == spReady && !m.lib.loaded && !m.lib.loading {
		m.lib.loading = true
		return m.loadLibrary()
	}
	if m.page == pageHome {
		return m.maybeLoadHome()
	}
	if m.page == pageDevices && !m.devs.loading {
		if cmd := m.loadDevices(); cmd != nil {
			m.devs.loading = true
			return cmd
		}
	}
	return nil
}

// position, çalan şarkının şu anki tahmini konumu.
func (m Model) position() time.Duration {
	pos := m.state.Position.At(m.now)
	if d := m.state.Item.Duration(); d > 0 && pos > d {
		pos = d
	}
	return pos
}

// listHeight, içerik sayfalarında liste satırlarına kalan yükseklik.
func (m Model) listHeight() int { return max(m.height-7-4, 1) }
