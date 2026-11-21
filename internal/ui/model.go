// Package ui, sotto'nun Bubble Tea arayüzü.
package ui

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/bridge"
	"github.com/Ehtisam/sotto/internal/cli"
	"github.com/Ehtisam/sotto/internal/config"
	"github.com/Ehtisam/sotto/internal/cover"
	"github.com/Ehtisam/sotto/internal/desktop"
	"github.com/Ehtisam/sotto/internal/discord"
	"github.com/Ehtisam/sotto/internal/i18n"
	"github.com/Ehtisam/sotto/internal/keys"
	"github.com/Ehtisam/sotto/internal/lyrics"
	"github.com/Ehtisam/sotto/internal/soloist"
	"github.com/Ehtisam/sotto/internal/spotify"
	"github.com/Ehtisam/sotto/internal/ui/theme"
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
	{"V", "page.queue", ""},
	{"VI", "page.devices", "soon.devices"},
	{"VII", "page.profile", ""},
	{"VIII", "page.settings", ""},
}

// hasContent: içine girilip gezinilebilen sayfalar.
func (p page) hasContent() bool {
	return p == pageHome || p == pageLibrary || p == pageDevices || p == pageSearch || p == pageQueue || p == pageProfile || p == pageSettings
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

	gfx gfxShared // gerçek kapak (Sixel) yerleşimi

	// contextNames: adını Spotify'ın vermediği bağlamlar (radyo, Spotify'ın listeleri).
	contextNames map[string]string
	out          *Output // terminal çıktısı (testlerde nil)

	ev events // bildirim, olay komutları, medya kartı, kaldığın yer (events.go)

	// freshUntil: R'ye basıldı; bu ana kadar başlayan yüklemeler önbelleği atlar.
	freshUntil time.Time

	sorts map[string]sortPref // liste başına sıralama (sort.go)

	logs    *logBook          // günlük (log.go)
	bar     barZone           // ilerleme çubuğunun ekrandaki yeri (mouse.go)
	btns    []btnZone         // şerit düğmelerinin yeri (strip_buttons; mouse.go)
	viz     vizState          // ses görselleştirmesi (viz.go)
	hist    histRec           // dinleme kaydı (histrec.go)
	smart   smartState        // akıllı karıştırma (smart.go)
	discord *discord.Presence // Discord'da "şu an dinliyor" (discord.go)
	// privUntil: özel oturumun bittiği an (private.go; sıfır: kapalı).
	privUntil time.Time
}

// apiCtx, sayfa yüklemeleri için zaman aşımlı bağlam; R'den hemen sonraysa
// Spotify'ın önbelleği atlanır (spotify.Fresh).
func apiCtx(rt *shared, d time.Duration) (context.Context, context.CancelFunc) {
	ctx := context.Background()
	if rt != nil && time.Now().Before(rt.freshUntil) {
		ctx = spotify.Fresh(ctx)
	}
	return context.WithTimeout(ctx, d)
}

// refreshNow: R tuşu; ardından başlayan yüklemeler Spotify'a yeniden sorar.
func (m Model) refreshNow() { m.rt.freshUntil = time.Now().Add(3 * time.Second) }

// Model, sotto'nun bütün arayüz durumu.
type Model struct {
	cfg config.Config
	rt  *shared
	sp  *spotify.Client

	width, height int
	gfx           gfxState // terminalin resim desteği
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

	lib        library
	search     searchState
	home       homeState
	devs       deviceList
	qs         queueState
	prof       profileState
	set        settingsState
	help       bool // ? ile açılan kısayol listesi
	helpScroll int
	helpFind   finder // ? listesinde / ile süzgeç (help.go)
	vizFull    bool   // tam ekran görselleştirme (V, vizfull.go)
	wiz        wizardState
	actions    actionMenu
	lyr        lyricsState
	sleep      sleepState
	shiftAt    time.Time  // shift basılı (shift.go); sıfır: değil
	form       plForm     // liste formu (plform.go)
	sortm      sortMenu   // sıralama menüsü (sort.go)
	pick       devPicker  // hızlı cihaz seçici (devpick.go)
	extra      extraState // ek sayfalar: en çok dinlenenler, son çalınanlar, günlük (extra.go)

	km      keys.Map // tuş haritası (keymap.go)
	seq     string   // yarım kalan tuş dizisi ("g")
	seqAt   time.Time
	count   string // yarım kalan sayı ("12")
	countAt time.Time

	flash     string
	flashAt   time.Time
	flashInfo bool // hata değil, bilgi notu
}

// New, ayarlarla yeni bir model kurar.
func New(cfg config.Config) Model {
	themeErrs := theme.LoadUser(cfg.ConfigDir) // themes klasöründeki temalar
	applyLook(cfg)
	m := Model{
		cfg: cfg,
		rt: &shared{
			known: map[string]spotify.Track{}, pending: map[string]bool{},
			covers: cover.NewStore(filepath.Join(cfg.LocalDir, "covers")), coverAsked: map[string]bool{},
			bridge: bridge.New(filepath.Join(cfg.LocalDir, "bridge")),
			lyrics: map[string]*lyrics.Lyrics{}, lyricsAsked: map[string]bool{},
			contextNames: map[string]string{},
			privUntil:    loadPrivate(cfg.LocalDir),
			smart:        smartState{added: map[string]bool{}},
		},
		sp:  spotify.NewClient(cfg.Spotify.ClientID, cfg.TokenFile()),
		now: time.Now(),
	}
	m.search.recent = loadRecent(cfg.LocalDir)
	m.rt.sorts = loadSorts(cfg.LocalDir)
	m.rt.logs = &logBook{}
	if cfg.LocalDir != "" {
		m.rt.logs.path = logFile(cfg.LocalDir)
	}
	m.km = keys.Load(cfg.ConfigDir)
	for _, e := range themeErrs {
		m = m.say(e.Error())
	}
	if len(m.km.Errors) > 0 {
		m = m.say("keymap.toml: " + strings.Join(m.km.Errors, "; "))
	}
	if s, ok := m.rt.sorts[libSortKey]; ok {
		m.lib.sortBy, m.lib.desc = s.By, s.Desc
	}
	m.rt.ev.focused = true
	m.rt.ev.last = loadLast(cfg.LocalDir)
	desktop.SetDataDir(cfg.LocalDir)
	soloist.CleanOld(cfg.SoloistPath()) // önceki güncellemeden kalan eski sürüm
	if cfg.Soloist.Autostart {
		m.engine = engineStarting
	} else {
		m.engine = engineOff
	}
	switch {
	case !m.sp.HasClientID():
		m.spState = spNoClient
		m.wiz.on = true // ilk açılış: kurulum sihirbazı
	case m.sp.HasLogin():
		m.spState = spReady
		m.lib.loading = true // Init kitaplığı ve ana sayfayı yüklemeye başlar
		m.home.loading = true
	default:
		m.spState = spLoggedOut
	}
	return m
}

// WithOutput, terminal çıktısını modele tanıtır (resim şeritleri için; main
// aynı Output'u tea.WithOutput'a da verir).
func (m Model) WithOutput(o *Output) Model {
	m.rt.out = o
	return m
}

// Shutdown, sotto'nun başlattığı Soloist'i ve bağlantıyı kapatır.
func (m Model) Shutdown() {
	m.histFlush()
	if p := m.rt.discord; p != nil {
		m.rt.discord = nil
		p.Close()
	}
	saveLast(m.cfg.LocalDir, m.rt.ev.last)
	if m.rt.ev.media != nil {
		m.rt.ev.media.Close()
		m.rt.ev.media = nil
	}
	m.closeViz()
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
			Prefs: &soloist.Prefs{
				CrossfadeS: cfg.Soloist.Crossfade,
				Quality:    cfg.Soloist.Quality,
				Normalize:  cfg.Soloist.Normalize,
			},
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
	sp, device := m.sp, m.remoteDeviceName()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := viaAPI(ctx, sp); err != nil {
			if spotify.IsStatus(err, 404) {
				return flashMsg(i18n.T("flash.nodevice"))
			}
			if spotify.IsStatus(err, 403) { // cihazın kısıtı (telefon sesi, kapalı tekrar…)
				if strings.Contains(err.Error(), "volume") {
					return flashMsg(i18n.F("flash.novolume", device))
				}
				if strings.Contains(err.Error(), "Restriction violated") {
					return flashMsg(i18n.F("flash.disallowed", device))
				}
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

// openBrowser, adresi varsayılan tarayıcıda açar (testler değiştirir).
var openBrowser = func(u string) error {
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
	cmds := []tea.Cmd{tick(), tea.Raw(gfxQuery)}
	m.syncDiscord() // ayar açıksa Discord'a bağlanmayı dener (arka planda)
	if m.cfg.Soloist.Autostart {
		cmds = append(cmds, startSoloist(m.cfg))
	}
	if m.spState == spReady {
		cmds = append(cmds, m.loadMe(), m.loadLibrary(), m.loadHome())
	}
	if m.cfg.Desktop.MediaControls {
		cmds = append(cmds, openMedia())
	}
	if !m.wiz.on {
		cmds = append(cmds, m.readSoloistBuild()) // süre uyarısı ve Ayarlar için
	}
	return tea.Batch(cmds...)
}

// ---- güncelleme ----

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if mm, cmd, ok := m.updateGfx(msg); ok {
		return mm, cmd
	}
	before := m.snap()
	out, cmd := m.update(msg)
	mm := out.(Model)
	if mm.extra.page != extraNone && mm.extra.host != mm.page { // başka sayfaya geçildi
		mm.extra = extraState{rng: mm.extra.rng}
	}
	return mm, tea.Batch(cmd, mm.scheduleGfx(), mm.observe(before))
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if mm, ok := m.shiftEvent(msg); ok {
		return mm, nil
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.rt.gfx.drawn = nil // Bubble Tea boyut değişince ekranı baştan çizer
		m.clampLib()
		if m.extraOn() {
			m.rebuildExtra() // sarılmış yazılar yeni genişliğe
		}
		return m, m.afterResize()

	case tickMsg:
		m.now = time.Time(msg)
		if m.flash != "" && m.now.Sub(m.flashAt) > 5*time.Second {
			m.flash = ""
		}
		if m.devs.switching != "" && m.now.Sub(m.devs.switchAt) > switchTimeout {
			m.devs.switching = ""
		}
		var cmd, dcmd, qcmd, scmd, vcmd tea.Cmd
		m.histTick()
		smcmd := m.smartTick()
		m, vcmd = m.vizTick()
		m = m.lyricsTick()
		if m.extraOn() && m.extra.page == extraLog {
			m.rebuildExtra()
		}
		m, scmd = m.sleepTick()
		m, cmd = m.maybePoll()
		m, qcmd = m.maybeRefreshQueue()
		if m.page == pageDevices && !m.devs.loading && m.now.Sub(m.devs.fetched) > 5*time.Second {
			if dcmd = m.loadDevices(); dcmd != nil {
				m.devs.loading = true
			}
		}
		return m, tea.Batch(tick(), cmd, dcmd, qcmd, scmd, vcmd, smcmd, m.fetchCovers(), m.fetchLyrics())

	case engineUpMsg:
		m.rt.proc, m.rt.client = msg.proc, msg.client
		m.engine, m.engineErr = engineReady, nil
		return m, listen(msg.client)

	case engineErrMsg:
		m.engine, m.engineErr = engineFailed, msg.err
		m.devs.switching, m.devs.pending = "", ""
		return m, m.readSoloistBuild()

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
		switch m.page { // profil bilgisi için kullanıcı kimliği gerekiyordu
		case pageProfile:
			return m, m.maybeLoadProfile()
		case pageLibrary:
			return m, m.maybeLoadProfileView()
		}
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

	case profileTopMsg, profileCountsMsg, profileViewMsg:
		return m.updateProfile(msg)

	case lyricsMsg:
		return m.updateLyrics(msg)

	case savedStateMsg, actionDoneMsg:
		return m.updateActions(msg)

	case smartRecsMsg, smartAddedMsg:
		return m.updateSmart(msg)
	case privateMsg:
		return m.updatePrivate(msg)
	case radioMsg:
		return m.updateRadio(msg)

	case queueEditMsg:
		return m.updateQueueEdit(msg)

	case plSavedMsg:
		return m.updatePlaylistForm(msg)

	case plDeletedMsg:
		return m.updatePlaylistDeleted(msg)

	case rootlistMsg:
		return m.updateRootlist(msg)

	case plMovedMsg:
		return m.updateListMove(msg)

	case wizDownloadedMsg, wizBridgeMsg, wizBridgeRetryMsg:
		return m.updateWizard(msg)

	case tea.PasteMsg:
		if m.wiz.on {
			return m.wizardPaste(msg.Content), nil
		}
		// Yapıştırılan Spotify bağlantısı açılır (formda değilken).
		if !m.form.on && spotifyLink(msg.Content) != "" {
			return m.openLink(msg.Content)
		}
		if m.page == pageSearch && m.focus == focusContent && m.search.typing && m.search.open == nil && !m.form.on {
			return m.searchPaste(msg.Content)
		}
		if mm, ok := m.findPaste(msg.Content); ok && !m.form.on {
			return mm, mm.loadAllForFind()
		}

	case vizStartedMsg, vizTickMsg:
		return m.updateViz(msg)

	case extraTopMsg, extraRecentMsg:
		return m.updateExtra(msg)

	case openArtistMsg, artistViewMsg, artistAlbumsMsg:
		return m.updateArtist(msg)
	case infoMsg:
		return m.updateInfo(msg)
	case newsMsg:
		return m.updateNews(msg)
	case peopleMsg, userMsg, openUserMsg:
		return m.updatePeople(msg)
	case openPeopleMsg:
		return m.openPeople(msg.user, msg.name, msg.section)
	case savedLibMsg:
		return m.updateSavedLib(msg)

	case seqTimeoutMsg, countTimeoutMsg:
		return m.updateKeyTimeout(msg)

	case clipboardMsg, linkEntryMsg:
		return m.updateQuick(msg)

	case seekEntryMsg:
		return m.updateSeekEntry(msg)

	case IPCMsg:
		return m.updateIPC(msg)

	case ipcFoundMsg:
		return m, m.playFound(msg.uri, msg.label, msg.reply)

	case soloistBuildMsg, soloistUpdatedMsg, applyPrefsMsg:
		return m.updateSettingsMsg(msg)

	case mediaOpenedMsg, mediaEventMsg:
		return m.updateMedia(msg)

	case tea.FocusMsg:
		m.rt.ev.focused = true
		return m, nil

	case tea.BlurMsg:
		m.rt.ev.focused = false
		return m, nil

	case resumedMsg:
		return m, pollSoonMsgCmd()

	case coverMsg:
		return m, nil // kapak artık bellekte; bir sonraki çizimde görünür

	case tea.KeyPressMsg:
		return m.key(msg)

	case tea.MouseClickMsg, tea.MouseWheelMsg:
		return m.mouse(msg.(tea.MouseMsg))
	}
	return m, nil
}

// say, alt satırda kısa bir hata notu gösterir.
func (m Model) say(s string) Model {
	m.rt.logs.add(logError, s, m.now)
	m.flash, m.flashAt, m.flashInfo = s, m.now, false
	return m
}

// note, alt satırda kısa bir bilgi notu gösterir.
func (m Model) note(s string) Model {
	m.rt.logs.add(logInfo, s, m.now)
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
			// Soloist kullanıcı istediği için açıldıysa müziği ona al;
			// açılıştaysa ve hiçbir yerde bir şey çalmıyorsa kaldığın yeri yükle.
			if m.devs.pending != "" && m.spState == spReady {
				cmds = append(cmds, m.transferByNamePlay(m.devs.pending, !m.devs.pendingPaused))
				m.devs.pending, m.devs.pendingPaused = "", false
			} else {
				cmds = append(cmds, m.maybeResume())
			}
			return tea.Batch(cmds...)
		}
	case soloist.PlaybackState:
		m.state = e
		m.state.Context = m.nameContext(e.Context)
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
		m.state.Context = m.nameContext(e.Context)
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

// keyCanon, haritadan geçmiş tuşu (k) işler (keymap.go: key). Yazı yazılan
// yerlerde k basılan tuşun kendisidir.
func (m Model) keyCanon(msg tea.KeyPressMsg, k string) (tea.Model, tea.Cmd) {
	// Kurulum sihirbazı açıkken bütün tuşlar ona gider.
	if m.wiz.on {
		if k == "ctrl+c" {
			return m, m.quit()
		}
		return m.wizardKey(msg)
	}

	// Liste formu açıkken tuşlar ona gider.
	if m.form.on {
		if k == "ctrl+c" {
			return m, m.quit()
		}
		return m.formKey(msg)
	}

	// Eylem menüsü açıkken tuşlar ona gider.
	if m.actions.open {
		if k == "ctrl+c" {
			return m, m.quit()
		}
		return m.actionsKey(k)
	}

	// Kısayol listesi açıkken: esc ya da ? kapatır (esc önce süzgeci temizler).
	if m.help {
		if mm, cmd, ok := m.helpFindKey(msg); ok {
			return mm, cmd
		}
		switch k {
		case "esc":
			if m.helpFind.active() {
				m.helpFind, m.helpScroll = finder{}, 0
				return m, nil
			}
			m.help = false
			return m, nil
		case "?", "q":
			m.help = false
			return m, nil
		}
		if mm, ok := m.helpKey(k); ok {
			return mm, nil
		}
	}

	// Tam ekran görselleştirme açıkken: stil ve kapatma tuşları; gerisi genel.
	if m.vizFull && !m.help {
		if mm, cmd, ok := m.vizFullKey(k); ok {
			return mm, cmd
		}
	}

	// Cihaz seçici açıkken tuşlar ona gider.
	if m.pick.open {
		if k == "ctrl+c" {
			return m, m.quit()
		}
		return m.devPickKey(k)
	}

	// Sıralama menüsü açıkken tuşlar ona gider.
	if m.sortm.open {
		if k == "ctrl+c" {
			return m, m.quit()
		}
		return m.sortKey(k)
	}

	// Sayfa içi arama: yazarken harfler aranacak metindir; / ve ctrl+f
	// başlatır, esc önce süzgeci temizler, shift+O sıralama menüsünü açar.
	if m.focus == focusContent && !m.help {
		if mm, cmd, ok := m.findKey(msg); ok {
			return mm, cmd
		}
		switch k {
		case "/", "ctrl+f":
			if mm, cmd, ok := m.startFind(); ok {
				return mm, cmd
			}
		case "O":
			if mm, ok := m.openSort(); ok {
				return mm, nil
			}
		case "esc":
			if mm, ok := m.clearFind(); ok {
				return mm, nil
			}
		}
	}

	// Ek sayfa (en çok dinlenenler, son çalınanlar, günlük) sayfanın üstünde.
	extraOn := m.extraOn()
	if extraOn && m.focus == focusContent && !m.help {
		if mm, cmd, ok := m.extraKey(k); ok {
			return mm, cmd
		}
	}

	// Ara sayfasında yazarken harfler kısayol değil, aranacak metindir.
	if m.focus == focusContent && !m.help && m.page == pageSearch && !extraOn {
		if mm, cmd, ok := m.searchKey(msg, k); ok {
			return mm, cmd
		}
	}

	// İçerikteyken gezinme tuşları önce sayfaya gider.
	if m.focus == focusContent && !m.help && m.page != pageSearch && !extraOn {
		if mm, cmd, ok := m.contentKey(k); ok {
			return mm, cmd
		}
	}

	switch k {
	case "q", "ctrl+c":
		return m, m.quit()
	case "?":
		m.help, m.helpScroll, m.helpFind = true, 0, finder{}
		return m, nil
	case "i":
		return m.openInfo()
	case "/":
		m.page, m.focus = pageSearch, focusContent
		m.search.open = nil
		return m.enterSearch(), nil

	case "up", "k", "down", "j":
		// Menüde gezerken de sayfanın verisini iste: önizlemesi hemen dolsun.
		if k == "up" || k == "k" {
			m.page = (m.page + page(len(pages)) - 1) % page(len(pages))
		} else {
			m.page = (m.page + 1) % page(len(pages))
		}
		cmd := m.loadPage()
		return m, cmd
	case "1", "2", "3", "4", "5", "6", "7", "8":
		// Rakam sayfaya götürür ve içine girer; içi olmayan sayfada menüde kalır.
		m.page, m.focus, m.help = page(k[0]-'1'), focusMenu, false
		if m.page.hasContent() {
			m.focus = focusContent
			if m.page == pageSearch {
				m.search.open = nil
				m = m.enterSearch()
			}
			cmd := m.enterPage()
			return m, cmd
		}
	case "enter", "right", "tab":
		if m.page == pageNow && m.lyricsPickable() {
			// Şimdi çalıyor: sözlere gir, söylenen satır seçili başlar.
			m.focus = focusContent
			mm, cmd, _ := m.nowKey("enter")
			return mm, cmd
		}
		if m.page.hasContent() {
			m.focus = focusContent
			if m.page == pageSearch {
				m = m.enterSearch()
			}
			cmd := m.enterPage()
			return m, cmd
		}

	case "space", " ":
		return m, m.playPause()
	case "n":
		// Kitaplık'ta n yeni liste açar (menüde, listede, açık listede); başka
		// yerde sonraki şarkı.
		if m.page == pageLibrary && m.spState == spReady {
			return m.openPlaylistForm(nil, ""), nil
		}
		return m, m.next()
	case "p":
		return m, m.previous()
	case ",", "<", ".", ">":
		step := 10 * time.Second
		if k == "," || k == "<" {
			step = -step
		}
		return m, m.seekTo(m.position() + step)
	case "+", "=", "-":
		v := m.state.Volume + 5
		if k == "-" {
			v = m.state.Volume - 5
		}
		return m, m.setVolume(v)
	case "s":
		return m.cycleShuffle()
	case "r":
		next, _ := cli.NextRepeat("", m.state.Options.Repeat)
		return m, m.setRepeat(next)
	case "t":
		return m.switchSpeaker()
	case "d":
		return m.openDevPicker()
	case "V":
		return m.toggleVizFull()
	case "a":
		return m.openActions()
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
	case "z":
		return m.cycleSleep(), nil
	case "m":
		return m.toggleMute("")
	case "0":
		return m, m.seekTo(0)
	case "S":
		return m.shufflePlay()
	case "h":
		return m.toggleLike()
	case "u":
		return m.queueSelected()
	case "v":
		return m.openClipboard()
	case "c":
		return m.findPlaying()
	case "N":
		// Yeni liste: her sayfadan, menüdeyken de.
		if m.spState != spReady {
			return m.say(T("flash.noaccount")), nil
		}
		return m.openPlaylistForm(nil, ""), nil
	}
	return m, nil
}

// contentKey, içerik odaktayken sayfanın kendi tuşları. ok=false ise tuş genel kısayollara düşer.
func (m Model) contentKey(k string) (Model, tea.Cmd, bool) {
	mm, cmd, ok := m.pageKey(k)
	if !ok && k == "tab" {
		mm.focus = focusMenu
		return mm, nil, true
	}
	return mm, cmd, ok
}

// pageKey, içerik odaktayken tuşu açık sayfaya verir.
func (m Model) pageKey(k string) (Model, tea.Cmd, bool) {
	switch m.page {
	case pageNow:
		return m.nowKey(k)
	case pageHome:
		return m.homeKey(k)
	case pageLibrary:
		return m.libraryKey(k)
	case pageDevices:
		return m.devicesKey(k)
	case pageQueue:
		return m.queueKey(k)
	case pageProfile:
		return m.profileKey(k)
	case pageSettings:
		return m.settingsKey(k)
	}
	return m, nil, false
}

// loadPage, sayfanın verisi eksik ya da eskiyse ister (menüde üstüne gelince
// de çağrılır; imleçlere dokunmaz).
func (m *Model) loadPage() tea.Cmd {
	switch m.page {
	case pageQueue:
		mm, cmd := m.refreshQueue()
		*m = mm
		return tea.Batch(cmd, m.maybeLoadHome(), m.enrich())
	case pageProfile:
		return m.maybeLoadProfile()
	}
	return m.enterPage()
}

// enterPage, bir sayfaya girilince gerekenleri başlatır.
func (m *Model) enterPage() tea.Cmd {
	if m.page == pageLibrary {
		view := m.maybeLoadProfileView() // "profilinde" işaretleri için
		if m.spState == spReady && !m.lib.loaded && !m.lib.loading {
			m.lib.loading = true
			return tea.Batch(m.loadLibrary(), view)
		}
		return view
	}
	if m.page == pageHome {
		return m.maybeLoadHome()
	}
	if m.page == pageProfile {
		return m.maybeLoadProfile()
	}
	if m.page == pageQueue {
		m.qs.cursor, m.qs.scroll = 0, 0
		mm, cmd := m.refreshQueue()
		*m = mm
		return tea.Batch(cmd, m.maybeLoadHome(), m.enrich())
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
func (m Model) listHeight() int { return max(m.layout().bodyH-4, 1) }
