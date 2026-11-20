package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/cli"
	"github.com/Ehtisam/sotto/internal/desktop"
	"github.com/Ehtisam/sotto/internal/soloist"
	"github.com/Ehtisam/sotto/internal/spotify"
)

// Dış dünya: şarkı ya da çalma durumu değişince bildirim, olay komutları,
// Windows'un medya kartı ve "kaldığın yer" kaydı. Değişiklikler her
// Update'ten sonra önceki durumla karşılaştırılarak bulunur; hangi yoldan
// gelirse gelsin (Soloist, kumanda yoklaması) aynı yerden geçer.

// events, modelin kopyaları arasında paylaşılan olay durumu (shared içinde).
type events struct {
	announced  string    // duyurulan son şarkı
	seenURI    string    // adı beklenen şarkı
	seenAt     time.Time // seenURI ilk görüldüğünde
	started    bool      // ilk durum geldi (açılıştaki şarkı duyurulmaz)
	lastStatus string    // playing, paused, stopped
	focused    bool      // sotto'nun penceresi önde

	media *desktop.Media

	last      lastPlay  // kaldığın yer
	lastSaved time.Time // dosyaya en son yazıldığında

	resumeTried bool
}

// lastPlay, kaldığın yer (%LOCALAPPDATA%\sotto\last.json).
type lastPlay struct {
	Context    string    `json:"context,omitempty"`
	Track      string    `json:"track"`
	PositionMS int64     `json:"position_ms"`
	Title      string    `json:"title,omitempty"`
	Artist     string    `json:"artist,omitempty"`
	Saved      time.Time `json:"saved"`
}

func lastFile(dir string) string { return filepath.Join(dir, "last.json") }

func loadLast(dir string) lastPlay {
	var l lastPlay
	if b, err := os.ReadFile(lastFile(dir)); err == nil {
		_ = json.Unmarshal(b, &l)
	}
	return l
}

func saveLast(dir string, l lastPlay) {
	if l.Track == "" || dir == "" {
		return
	}
	if b, err := json.MarshalIndent(l, "", "  "); err == nil {
		_ = os.MkdirAll(dir, 0o700)
		_ = os.WriteFile(lastFile(dir), b, 0o600)
	}
}

type playSnap struct {
	uri    string
	status string
	pos    soloist.Position
}

func (m Model) snap() playSnap {
	return playSnap{uri: m.state.Item.URIOrEmpty(), status: simpleStatus(m.state.Status, m.state.Item), pos: m.state.Position}
}

// simpleStatus: playing, paused ya da stopped.
func simpleStatus(s string, item *soloist.Entity) string {
	switch {
	case item == nil:
		return "stopped"
	case s == soloist.StatusPlaying || s == soloist.StatusBuffering:
		return "playing"
	case s == soloist.StatusPaused:
		return "paused"
	}
	return "stopped"
}

// observe, bu güncellemede çalma durumunda ne değiştiğine bakar.
func (m Model) observe(old playSnap) tea.Cmd {
	ev := &m.rt.ev
	cur := m.snap()
	var cmds []tea.Cmd

	if cur.uri != "" {
		name, artist, _ := m.describe(m.state.Item)
		ev.last.Track, ev.last.Context = cur.uri, m.state.Context.URIOrEmpty()
		ev.last.PositionMS = m.position().Milliseconds()
		ev.last.Title, ev.last.Artist = name, artist
		if cur.uri != ev.announced {
			if ev.seenURI != cur.uri {
				ev.seenURI, ev.seenAt = cur.uri, m.now
			}
			// Soloist bazen adı sonradan veriyor; kısa bir süre bekle.
			if name != "" || m.now.Sub(ev.seenAt) > 3*time.Second {
				ev.announced = cur.uri
				if name != "" {
					m.rt.logs.add(logEvent, "♪ "+name+" — "+artist, m.now)
				}
				cmds = append(cmds, m.mediaTrack(), m.mediaPosition())
				if ev.started {
					cmds = append(cmds, m.notifyTrack(), m.hook("track_change", m.cfg.Hooks.OnTrackChange))
				}
				ev.started = true
				ev.last.Saved = m.now
				ev.lastSaved = m.now
				l, dir := ev.last, m.cfg.LocalDir
				cmds = append(cmds, func() tea.Msg { saveLast(dir, l); return nil })
			}
		} else if cur.status == "playing" && m.now.Sub(ev.lastSaved) > 15*time.Second {
			ev.lastSaved = m.now
			ev.last.Saved = m.now
			l, dir := ev.last, m.cfg.LocalDir
			cmds = append(cmds, func() tea.Msg { saveLast(dir, l); return nil })
		}
	}

	if cur.status != ev.lastStatus {
		prev := ev.lastStatus
		ev.lastStatus = cur.status
		cmds = append(cmds, m.mediaStatus(cur.status), m.mediaPosition())
		if prev != "" && ev.started {
			cmds = append(cmds, m.hook("status_change", m.cfg.Hooks.OnStatusChange))
		}
	} else if cur.uri != "" && cur.uri == old.uri {
		// Sarıldıysa kartın zaman çizelgesini düzelt.
		if d := old.pos.At(m.now) - cur.pos.At(m.now); math.Abs(d.Seconds()) > 2 {
			cmds = append(cmds, m.mediaPosition())
		}
	}
	return tea.Batch(cmds...)
}

// ---- bildirim ----

// trackFields, bildirim biçiminde ve olay komutlarında kullanılan alanlar.
func (m Model) trackFields() map[string]string {
	name, artist, album := m.describe(m.state.Item)
	device := m.remote.device
	if m.soloistActive() {
		device = m.auth.DeviceName
	}
	return map[string]string{
		"title": name, "artist": artist, "album": album,
		"context": m.state.Context.Name(), "device": device,
	}
}

// fillTemplate, "{title}" gibi alanları doldurur; boş kalan alanların
// ayraçlarını temizler ("Şarkı · " değil "Şarkı").
func fillTemplate(tpl string, f map[string]string) string {
	for k, v := range f {
		tpl = strings.ReplaceAll(tpl, "{"+k+"}", v)
	}
	tpl = strings.TrimSpace(tpl)
	for _, sep := range []string{"·", "—", "-", "|", "•", ","} {
		for strings.Contains(tpl, sep+"  "+sep) || strings.Contains(tpl, sep+" "+sep) {
			tpl = strings.ReplaceAll(tpl, sep+"  "+sep, sep)
			tpl = strings.ReplaceAll(tpl, sep+" "+sep, sep)
		}
		tpl = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(tpl, sep), sep))
	}
	return strings.TrimSpace(tpl)
}

func (m Model) notifyTrack() tea.Cmd {
	c := m.cfg.Desktop
	switch {
	case (c.Notify != "always" && c.Notify != "unfocused") || m.state.Item == nil:
		return nil
	case c.Notify == "unfocused" && m.rt.ev.focused:
		return nil
	case simpleStatus(m.state.Status, m.state.Item) != "playing":
		return nil
	}
	f := m.trackFields()
	title, body := fillTemplate(c.NotifyTitle, f), fillTemplate(c.NotifyBody, f)
	if title == "" {
		title, body = body, ""
	}
	if title == "" {
		return nil
	}
	store, coverURL := m.rt.covers, m.notifyCoverURL()
	return func() tea.Msg {
		n := desktop.Note{Title: title, Body: body}
		if coverURL != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			n.Image, _ = store.File(ctx, coverURL)
			cancel()
		}
		_ = notify(n)
		return nil
	}
}

// notifyCoverURL: kapaklar kapalı olsa da bildirimde ve medya kartında kapak gösterilir.
func (m Model) notifyCoverURL() string {
	c := m.cfg
	c.Look.Covers = "image"
	mm := m
	mm.cfg = c
	return mm.nowCoverURL()
}

// Testler değiştirir.
var (
	notify    = desktop.Notify
	runHookFn = runHook
)

// SyncStartMenu, Başlat menüsü kısayolunu ayara göre ekler (yoksa ya da
// başka bir exe'yi gösteriyorsa) ya da siler. Windows dışında bir şey yapmaz.
func SyncStartMenu(on bool) error {
	if !on {
		return removeShortcut()
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return err
	}
	desktop.CleanupOld()
	changed := desktop.RefreshIcons() // simge değiştiyse kısayol yeniden yazılır
	if !changed && desktop.ShortcutOK(exe) {
		return nil
	}
	if err := installShortcut(exe); err != nil && !errors.Is(err, desktop.ErrUnsupported) {
		return err
	}
	return nil
}

// Testler değiştirir (test programını Başlat menüsüne koymasın).
var (
	installShortcut = desktop.InstallShortcut
	removeShortcut  = desktop.RemoveShortcut
)

// ---- olay komutları ----

func (m Model) hook(event, command string) tea.Cmd {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil
	}
	f := m.trackFields()
	env := append(os.Environ(),
		"SOTTO_EVENT="+event,
		"SOTTO_TITLE="+f["title"], "SOTTO_ARTIST="+f["artist"], "SOTTO_ALBUM="+f["album"],
		"SOTTO_CONTEXT="+f["context"], "SOTTO_DEVICE="+f["device"],
		"SOTTO_URI="+m.state.Item.URIOrEmpty(),
		"SOTTO_STATUS="+simpleStatus(m.state.Status, m.state.Item),
		fmt.Sprintf("SOTTO_DURATION_MS=%d", m.state.Item.Duration().Milliseconds()),
		"SOTTO_COVER_URL="+m.notifyCoverURL(),
	)
	logPath := filepath.Join(m.cfg.LocalDir, "logs", "hooks.log")
	return func() tea.Msg {
		runHookFn(command, env, logPath)
		return nil
	}
}

// runHook, komutu kabukta çalıştırır (en çok bir dakika) ve çıktısını
// logs\hooks.log'a yazar.
func runHook(command string, env []string, logPath string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := shellCommand(ctx, command)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	_ = os.MkdirAll(filepath.Dir(logPath), 0o700)
	if f, ferr := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); ferr == nil {
		status := "ok"
		if err != nil {
			status = err.Error()
		}
		fmt.Fprintf(f, "%s  %s  [%s]\n%s", time.Now().Format(time.DateTime), command, status, out)
		f.Close()
	}
}

// ---- medya kartı ve medya tuşları ----

type mediaOpenedMsg struct {
	media *desktop.Media
	err   error
}

type mediaEventMsg struct {
	media *desktop.Media
	ev    desktop.Event
}

func openMedia() tea.Cmd {
	return func() tea.Msg {
		md, err := desktop.OpenMedia()
		return mediaOpenedMsg{md, err}
	}
}

func listenMedia(md *desktop.Media) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-md.Events()
		if !ok {
			return nil
		}
		return mediaEventMsg{md, ev}
	}
}

func (m Model) updateMedia(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case mediaOpenedMsg:
		if msg.err != nil || !m.cfg.Desktop.MediaControls {
			if msg.media != nil {
				msg.media.Close()
			}
			return m, nil
		}
		m.rt.ev.media = msg.media
		// Kartı şimdiki durumla doldur.
		return m, tea.Batch(listenMedia(msg.media), m.mediaTrack(), m.mediaStatus(simpleStatus(m.state.Status, m.state.Item)), m.mediaPosition())

	case mediaEventMsg:
		if msg.media != m.rt.ev.media {
			return m, nil // kapatılmış kart
		}
		var cmd tea.Cmd
		switch {
		case msg.ev.IsSeek:
			cmd = m.seekTo(msg.ev.Seek)
		case msg.ev.Button == desktop.Play:
			cmd = m.resumeOrPlay()
		case msg.ev.Button == desktop.Pause || msg.ev.Button == desktop.Stop:
			cmd = m.pause()
		case msg.ev.Button == desktop.Toggle:
			cmd = m.playPause()
		case msg.ev.Button == desktop.Next:
			cmd = m.next()
		case msg.ev.Button == desktop.Previous:
			cmd = m.previous()
		}
		return m, tea.Batch(cmd, listenMedia(msg.media))
	}
	return m, nil
}

// setMediaControls, medya kartını açar ya da kapatır (Ayarlar).
func (m Model) setMediaControls(on bool) tea.Cmd {
	if !on {
		md := m.rt.ev.media
		m.rt.ev.media = nil
		if md != nil {
			return func() tea.Msg { md.Close(); return nil }
		}
		return nil
	}
	if m.rt.ev.media != nil {
		return nil
	}
	return openMedia()
}

func (m Model) mediaTrack() tea.Cmd {
	md := m.rt.ev.media
	if md == nil || m.state.Item == nil {
		return nil
	}
	name, artist, album := m.describe(m.state.Item)
	t := desktop.Track{Title: name, Artist: artist, Album: album, CoverURL: m.notifyCoverURL(), Duration: m.state.Item.Duration()}
	return func() tea.Msg { _ = md.SetTrack(t); return nil }
}

func (m Model) mediaStatus(s string) tea.Cmd {
	md := m.rt.ev.media
	if md == nil {
		return nil
	}
	st := map[string]desktop.Status{"playing": desktop.Playing, "paused": desktop.Paused, "stopped": desktop.Stopped}[s]
	return func() tea.Msg { _ = md.SetStatus(st); return nil }
}

func (m Model) mediaPosition() tea.Cmd {
	md := m.rt.ev.media
	if md == nil || m.state.Item == nil {
		return nil
	}
	pos, dur := m.position(), m.state.Item.Duration()
	return func() tea.Msg { _ = md.SetPosition(pos, dur); return nil }
}

// ---- çalma komutları (tuşlar, medya kartı ve komut satırı aynılarını kullanır) ----

func (m Model) playing() bool {
	return m.state.Status == soloist.StatusPlaying || m.state.Status == soloist.StatusBuffering
}

func (m Model) pause() tea.Cmd {
	return m.control(
		func(ctx context.Context, c *soloist.Client) error { return c.Pause(ctx) },
		func(ctx context.Context, s *spotify.Client) error { return s.Pause(ctx) })
}

func (m Model) resume() tea.Cmd {
	return m.control(
		func(ctx context.Context, c *soloist.Client) error { return c.Play(ctx, "") },
		func(ctx context.Context, s *spotify.Client) error { return s.Resume(ctx) })
}

// resumeOrPlay: çalan bir şey yüklüyse sürdürür; hiçbir şey yoksa kaldığın yerden başlatır.
func (m Model) resumeOrPlay() tea.Cmd {
	if m.state.Item == nil {
		if cmd := m.playLast(); cmd != nil {
			return cmd
		}
	}
	return m.resume()
}

func (m Model) playPause() tea.Cmd {
	if m.playing() {
		return m.pause()
	}
	return m.resumeOrPlay()
}

// remoteDeviceName: komutun gittiği cihazın adı (iletilerde).
func (m Model) remoteDeviceName() string {
	if m.remote.device != "" {
		return m.remote.device
	}
	return T("dev.this")
}

// refused: müzik başka bir cihazdaysa ve o cihaz bu komuta şu an izin
// vermiyorsa (Spotify bildiriyor) komut gönderilmez, nedeni söylenir.
func (m Model) refused(action string) tea.Cmd {
	if m.soloistActive() || !m.remote.active || !cli.Disallowed(m.remote.player, action, m.state.Options.Repeat) {
		return nil
	}
	key := "flash.disallowed"
	if action == "volume" {
		key = "flash.novolume"
	}
	text := F(key, m.remoteDeviceName())
	return func() tea.Msg { return flashMsg(text) }
}

func (m Model) setShuffle(on bool) tea.Cmd {
	if c := m.refused("shuffle"); c != nil {
		return c
	}
	return m.control(
		func(ctx context.Context, c *soloist.Client) error { return c.SetShuffle(ctx, on) },
		func(ctx context.Context, s *spotify.Client) error { return s.SetShuffle(ctx, on) })
}

func (m Model) setRepeat(mode string) tea.Cmd {
	if c := m.refused("repeat:" + mode); c != nil {
		return c
	}
	return m.control(
		func(ctx context.Context, c *soloist.Client) error { return c.SetRepeat(ctx, mode) },
		func(ctx context.Context, s *spotify.Client) error { return s.SetRepeat(ctx, mode) })
}

func (m Model) next() tea.Cmd {
	if c := m.refused("next"); c != nil {
		return c
	}
	return m.control(
		func(ctx context.Context, c *soloist.Client) error { return c.Next(ctx) },
		func(ctx context.Context, s *spotify.Client) error { return s.Next(ctx) })
}

func (m Model) previous() tea.Cmd {
	if c := m.refused("prev"); c != nil {
		return c
	}
	return m.control(
		func(ctx context.Context, c *soloist.Client) error { return c.Previous(ctx) },
		func(ctx context.Context, s *spotify.Client) error { return s.Previous(ctx) })
}

func (m Model) seekTo(pos time.Duration) tea.Cmd {
	if c := m.refused("seek"); c != nil {
		return c
	}
	ms := max(pos, 0).Milliseconds()
	return m.control(
		func(ctx context.Context, c *soloist.Client) error { return c.Seek(ctx, ms) },
		func(ctx context.Context, s *spotify.Client) error { return s.Seek(ctx, ms) })
}

func (m Model) setVolume(v int) tea.Cmd {
	if c := m.refused("volume"); c != nil {
		return c
	}
	v = max(0, min(100, v))
	return m.control(
		func(ctx context.Context, c *soloist.Client) error { return c.SetVolume(ctx, v) },
		func(ctx context.Context, s *spotify.Client) error { return s.SetVolume(ctx, v) })
}

// ---- kaldığın yerden devam ----

type resumedMsg struct{ err error }

// maybeResume: Soloist açılışta hazır olunca, hiçbir yerde bir şey çalmıyorsa
// müziği ona duraklatılmış olarak aktarır; Spotify son durumu (şarkı, konum,
// bağlam) geri yükler.
func (m Model) maybeResume() tea.Cmd {
	ev := &m.rt.ev
	if ev.resumeTried || !m.cfg.Soloist.Resume || m.spState != spReady || m.devs.pending != "" {
		return nil
	}
	ev.resumeTried = true
	sp, name := m.sp, m.cfg.Soloist.DeviceName
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if st, err := sp.Player(ctx); err != nil || st != nil {
			return resumedMsg{err} // başka bir yerde bir şey var; dokunma
		}
		for try := 0; try < 6; try++ {
			if ds, err := sp.Devices(ctx); err == nil {
				for _, d := range ds {
					if d.Name == name {
						return resumedMsg{sp.Transfer(ctx, d.ID, false)}
					}
				}
			}
			time.Sleep(time.Second)
		}
		return resumedMsg{errNotListed}
	}
}

// playLast, kaydedilen yerden çalmaya başlar (hiçbir şey yüklü değilken
// boşluk ya da medya tuşu). Soloist açıksa onda, değilse etkin cihazda.
func (m Model) playLast() tea.Cmd {
	l := m.rt.ev.last
	if l.Track == "" {
		l = loadLast(m.cfg.LocalDir)
	}
	if l.Track == "" || m.spState != spReady {
		return nil
	}
	sp, name, live := m.sp, m.cfg.Soloist.DeviceName, m.soloistLive()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		dev := ""
		if live {
			if ds, err := sp.Devices(ctx); err == nil {
				for _, d := range ds {
					if d.Name == name {
						dev = d.ID
					}
				}
			}
		}
		err := sp.PlayFrom(ctx, dev, l.Context, l.Track, l.PositionMS)
		if err != nil && l.Context != "" { // bağlam çalınamıyorsa (ör. beğenilenler) yalnızca şarkı
			err = sp.PlayFrom(ctx, dev, "", l.Track, l.PositionMS)
		}
		if err != nil {
			if spotify.IsStatus(err, 404) {
				return flashMsg(T("flash.nodevice"))
			}
			return flashMsg(err.Error())
		}
		return pollSoonMsg{}
	}
}

func pollSoonMsgCmd() tea.Cmd { return func() tea.Msg { return pollSoonMsg{} } }
