package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/soloist"
	"github.com/Ehtisam/sotto/internal/spotify"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// VI · Cihazlar: hoparlörler. Soloist (hafif mod) ve Spotify Connect
// cihazları yan yana; enter müziği seçilene aktarır.

type deviceList struct {
	items   []spotify.Device
	loading bool
	fetched time.Time
	err     error
	cursor  int
	pending string // Soloist açılınca müziği ona aktar
	// pendingPaused: aktarırken çalmaya başlatma (yeniden başlatma öncesi duraklatılmıştı).
	pendingPaused bool
	switchTo      string // kumandaya dönerken kullanılacak son cihaz

	// switching: müzik bu cihaza geçiriliyor (tuşa basıldığı an başlar,
	// transferredMsg ya da hata gelince biter); şeritte ve başlıkta görünür.
	switching string
	switchAt  time.Time

	// volSeq: cihaz sesi için basışlar toplanır, son basıştan 300 ms sonra
	// tek istek gider (devVolumeMsg aynı sıra numarasını taşıyorsa).
	volSeq int
}

type devVolumeMsg struct {
	seq      int
	id, name string
	soloist  bool
	vol      int
}

type devVolumeDoneMsg struct {
	name string
	err  error
}

// switchTimeout: geçiş bu sürede bitmezse "geçiliyor" yazısı kalkar.
const switchTimeout = 20 * time.Second

// beginSwitch, geçişin başladığını hemen gösterir (Spotify 1–2 sn sürüyor).
func (m Model) beginSwitch(name string) Model {
	if name == "" {
		name = T("dev.other")
	}
	m.devs.switching, m.devs.switchAt = name, m.now
	return m.note(F("flash.switching", name))
}

// deviceName, cihaz kimliğinin bilinen adı.
func (m Model) deviceName(id string) string {
	for _, d := range m.devs.items {
		if d.ID == id {
			return d.Name
		}
	}
	return ""
}

// switchSpinner, geçiş sürerken dönen işaret (tick 500 ms).
func (m Model) switchSpinner() string {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	if theme.Icon.Play == ">" { // ascii simgeler
		frames = []string{"|", "/", "-", "\\"}
	}
	n := int(m.now.Sub(m.devs.switchAt) / (500 * time.Millisecond))
	return frames[(n%len(frames)+len(frames))%len(frames)]
}

// switchingText: "⠋ Soloist'e geçiliyor…" (geçiş yoksa boş).
func (m Model) switchingText() string {
	if m.devs.switching == "" {
		return ""
	}
	return m.switchSpinner() + " " + F("strip.switching", m.devs.switching)
}

type devicesMsg struct {
	items []spotify.Device
	err   error
}

type transferredMsg struct {
	name string
	err  error
}

func (m Model) loadDevices() tea.Cmd {
	if m.spState != spReady {
		return nil
	}
	sp := m.sp
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		ds, err := sp.Devices(ctx)
		return devicesMsg{ds, err}
	}
}

// deviceRows, ekranda gösterilen satırlar: Soloist kapalıysa onun için de bir satır.
type deviceRow struct {
	name, kind string
	id         string
	active     bool
	soloist    bool
	off        bool
	vol        int  // -1: bilinmiyor
	fixedVol   bool // sesi uzaktan değiştirilemez
}

func (m Model) deviceRows() []deviceRow {
	var rows []deviceRow
	own := m.cfg.Soloist.DeviceName
	seenOwn := false
	for _, d := range m.devs.items {
		r := deviceRow{name: d.Name, kind: strings.ToLower(d.Type), id: d.ID, active: d.IsActive, vol: -1}
		if d.Volume != nil {
			r.vol = *d.Volume
		}
		r.fixedVol = d.SupportsVolume != nil && !*d.SupportsVolume
		if d.Name == own {
			r.soloist, seenOwn = true, true
		}
		rows = append(rows, r)
	}
	if !seenOwn {
		rows = append([]deviceRow{{name: own, soloist: true, off: !m.soloistLive(), vol: -1}}, rows...)
	}
	return rows
}

// transfer, müziği verilen cihaza aktarır. Telefon gibi bazı cihazlar
// aktarmadan sonra duraklatılmış kalıyor; müzik çalıyorduysa orada da
// devam etmesi için bir kez daha "çal" denir.
func (m Model) transfer(id, name string) tea.Cmd {
	sp := m.sp
	wasPlaying := m.state.Status == soloist.StatusPlaying || m.state.Status == soloist.StatusBuffering
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := sp.Transfer(ctx, id, true); err != nil {
			return transferredMsg{name, err}
		}
		if !wasPlaying {
			return transferredMsg{name, nil}
		}
		for try := 0; try < 4; try++ {
			time.Sleep(700 * time.Millisecond)
			st, err := sp.Player(ctx)
			if err == nil && st != nil && st.Device.ID == id && st.IsPlaying {
				break
			}
			_ = sp.ResumeOn(ctx, id)
		}
		return transferredMsg{name, nil}
	}
}

// transferByName, cihaz listesini tazeleyip adı verilen cihaza aktarır
// (Soloist yeni açıldığında kimliği henüz bilinmiyor).
func (m Model) transferByName(name string) tea.Cmd { return m.transferByNamePlay(name, true) }

// transferByNamePlay: play false ise müzik duraklatılmış olarak gelir.
// Soloist yeniden açıldıktan hemen sonra Spotify listede eski kaydı tutabiliyor;
// ona aktarma 500/404 döner. Bu yüzden yeni kayıt gelene dek yeniden denenir.
func (m Model) transferByNamePlay(name string, play bool) tea.Cmd {
	sp := m.sp
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		var last error = errNotListed
		for try := 0; try < 8; try++ {
			if ds, err := sp.Devices(ctx); err == nil {
				for _, d := range ds {
					if d.Name == name {
						last = sp.Transfer(ctx, d.ID, play)
						if last == nil || !transferRetryable(last) {
							return transferredMsg{name, last}
						}
						break
					}
				}
			}
			select {
			case <-ctx.Done():
				return transferredMsg{name, last}
			case <-time.After(time.Second):
			}
		}
		return transferredMsg{name, last}
	}
}

// transferRetryable: cihaz henüz hazır değil (Spotify'ın geçici hataları).
func transferRetryable(err error) bool {
	for _, code := range []int{404, 500, 502, 503} {
		if spotify.IsStatus(err, code) {
			return true
		}
	}
	return false
}

type notListedErr struct{}

func (notListedErr) Error() string { return "device is not in spotify's list yet" }

var errNotListed error = notListedErr{}

// startSoloistAndUse, Soloist'i açar ve hazır olunca müziği ona aktarır.
func (m Model) startSoloistAndUse() (Model, tea.Cmd) {
	m.devs.pending, m.devs.pendingPaused = m.cfg.Soloist.DeviceName, false
	m = m.beginSwitch(m.cfg.Soloist.DeviceName)
	if m.engine == engineStarting {
		return m, nil
	}
	m.engine, m.engineErr = engineStarting, nil
	return m, startSoloist(m.cfg)
}

// stopSoloist, sotto'nun açtığı Soloist'i kapatır; müzik kumandaya döner.
func (m Model) stopSoloist() (Model, tea.Cmd) {
	if m.rt.proc != nil && m.rt.proc.Attached {
		return m.say(T("flash.attached")), nil
	}
	c, p := m.rt.client, m.rt.proc
	m.rt.client, m.rt.proc = nil, nil
	m.engine, m.engineErr = engineOff, nil
	m.auth, m.haveAuth = soloist.AuthState{}, false
	m.state, m.queue = soloist.PlaybackState{}, soloist.QueueChanged{}
	m.devs.pending, m.devs.switching = "", ""
	m = m.note(T("flash.soloist.stopped"))
	return m, func() tea.Msg {
		if c != nil {
			c.Close()
		}
		if p != nil {
			p.Stop()
		}
		return pollSoonMsg{}
	}
}

// restartSoloist, Soloist'i ayar dosyası yeniden okunsun diye kapatıp açar.
// Müzik Soloist'teyse açılınca ona geri alınır: çalıyorsa çalmaya,
// duraklatılmışsa duraklatılmış olarak (Spotify konumu saklar).
// Soloist kapalıysa ya da başka bir sotto açtıysa yalnızca not düşer.
func (m Model) restartSoloist() (Model, tea.Cmd) {
	if m.rt.proc == nil || m.rt.proc.Attached || m.engine != engineReady {
		return m.note(T("set.applied.next")), nil
	}
	active, wasPlaying := m.soloistActive(), m.playing()
	c, p, cfg := m.rt.client, m.rt.proc, m.cfg
	m.rt.client, m.rt.proc = nil, nil
	m.auth, m.haveAuth = soloist.AuthState{}, false
	m.state, m.queue = soloist.PlaybackState{}, soloist.QueueChanged{}
	m.devs.pending, m.devs.pendingPaused = "", false
	if active {
		m.devs.pending, m.devs.pendingPaused = cfg.Soloist.DeviceName, !wasPlaying
		m = m.beginSwitch(cfg.Soloist.DeviceName)
	}
	m.engine, m.engineErr = engineStarting, nil
	m = m.note(T("set.applying"))
	start := startSoloist(cfg)
	return m, func() tea.Msg {
		if c != nil {
			c.Close()
		}
		if p != nil {
			p.Stop()
		}
		return start()
	}
}

// toggleSoloist: o tuşu. Kapalıysa açar ve müziği ona alır, açıksa kapatır.
func (m Model) toggleSoloist() (Model, tea.Cmd) {
	switch m.engine {
	case engineOff, engineFailed:
		return m.startSoloistAndUse()
	case engineStarting:
		return m.note(F("flash.switching", m.cfg.Soloist.DeviceName)), nil // basıldı, sürüyor
	}
	return m.stopSoloist()
}

// switchSpeaker: t tuşu. Müzik Soloist'teyse son kumanda cihazına,
// başka yerdeyse Soloist'e geçer (gerekirse Soloist'i açar).
func (m Model) switchSpeaker() (Model, tea.Cmd) {
	if m.spState != spReady {
		return m.say(T("flash.noaccount")), nil
	}
	if m.soloistActive() {
		if m.devs.switchTo == "" {
			m.page, m.focus = pageDevices, focusContent
			return m.note(T("flash.pickdevice")), m.loadDevices()
		}
		name := m.deviceName(m.devs.switchTo)
		return m.beginSwitch(name), m.transfer(m.devs.switchTo, name)
	}
	if m.soloistLive() {
		return m.beginSwitch(m.auth.DeviceName), m.transferByName(m.auth.DeviceName)
	}
	return m.startSoloistAndUse()
}

func (m Model) updateDevices(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case devicesMsg:
		m.devs.loading, m.devs.fetched = false, m.now
		if msg.err != nil {
			m.devs.err = msg.err
			return m, nil
		}
		m.devs.items, m.devs.err = msg.items, nil
		m.devs.cursor = max(0, min(len(m.deviceRows())-1, m.devs.cursor))
		if m.pick.open {
			if !m.pick.moved {
				m.pickActive()
			}
			m.pick.cursor = max(0, min(len(m.deviceRows())-1, m.pick.cursor))
		}
	case devVolumeMsg:
		return m, m.sendDeviceVolume(msg)
	case devVolumeDoneMsg:
		switch {
		case msg.err == nil:
			return m, nil
		case spotify.IsStatus(msg.err, 403), spotify.IsStatus(msg.err, 404):
			m = m.say(F("flash.novolume", msg.name))
		default:
			m = m.say(msg.err.Error())
		}
		return m, m.loadDevices() // gerçek değer geri gelsin
	case transferredMsg:
		m.devs.switching = ""
		if msg.err != nil {
			if spotify.IsStatus(msg.err, 404) {
				return m.say(T("flash.nodevice")), nil
			}
			return m.say(msg.err.Error()), nil
		}
		if msg.name != "" {
			m = m.note(F("flash.moved", msg.name))
		}
		return m, tea.Batch(m.loadDevices(), func() tea.Msg { return pollSoonMsg{} })
	}
	return m, nil
}

func (m Model) devicesKey(k string) (Model, tea.Cmd, bool) {
	rows := m.deviceRows()
	switch k {
	case "up", "k":
		m.devs.cursor = max(0, m.devs.cursor-1)
	case "down", "j":
		m.devs.cursor = min(len(rows)-1, m.devs.cursor+1)
	case "enter", "right":
		if len(rows) == 0 {
			return m, nil, true
		}
		r := rows[m.devs.cursor]
		if r.soloist && r.off {
			mm, cmd := m.startSoloistAndUse()
			return mm, cmd, true
		}
		if r.id == "" {
			return m, nil, true
		}
		return m.beginSwitch(r.name), m.transfer(r.id, r.name), true
	case "esc", "left", "backspace":
		m.focus = focusMenu
	case "R":
		m.devs.loading = true
		return m, m.loadDevices(), true
	case "+", "=", "-":
		if len(rows) == 0 {
			return m, nil, true
		}
		d := 5
		if k == "-" {
			d = -5
		}
		mm, cmd := m.deviceVolume(rows[m.devs.cursor], d)
		return mm, cmd, true
	default:
		return m, nil, false
	}
	return m, nil, true
}

// deviceVolume, seçili cihazın sesini d kadar değiştirir: satır hemen
// güncellenir, istek basışlar durunca gider. Çalan cihazda genel sesle aynı.
func (m Model) deviceVolume(r deviceRow, d int) (Model, tea.Cmd) {
	if r.soloist && r.off {
		return m.say(T("flash.nosoloist")), nil
	}
	if r.fixedVol {
		return m.say(F("flash.novolume", r.name)), nil
	}
	if r.id == "" && !r.soloist {
		return m, nil
	}
	cur := r.vol
	if r.active || cur < 0 {
		cur = m.state.Volume
	}
	v := max(0, min(100, cur+d))
	for i := range m.devs.items {
		if m.devs.items[i].ID == r.id || (r.soloist && m.devs.items[i].Name == r.name) {
			m.devs.items[i].Volume = &v
		}
	}
	if r.active {
		m.state.Volume = v // şerit de hemen göstersin
	}
	m.devs.volSeq++
	msg := devVolumeMsg{seq: m.devs.volSeq, id: r.id, name: r.name, soloist: r.soloist && m.rt.client != nil, vol: v}
	return m, tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg { return msg })
}

// sendDeviceVolume: Soloist'e kendi API'sinden, öbürlerine Web API'den.
func (m Model) sendDeviceVolume(msg devVolumeMsg) tea.Cmd {
	if msg.seq != m.devs.volSeq {
		return nil // arkadan yeni basış geldi
	}
	if msg.soloist {
		c := m.rt.client
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return devVolumeDoneMsg{name: msg.name, err: c.SetVolume(ctx, msg.vol)}
		}
	}
	if m.spState != spReady {
		return nil
	}
	sp := m.sp
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return devVolumeDoneMsg{name: msg.name, err: sp.SetDeviceVolume(ctx, msg.id, msg.vol)}
	}
}

func (m Model) devicesView(w int) []string {
	rows := []string{theme.Title.Render(T("page.devices"))}
	if m.spState != spReady {
		return append(rows, "", theme.Dim.Render(T("flash.noaccount")))
	}
	sub := theme.Dim.Render(T("dev.sub"))
	if m.devs.err != nil {
		sub = theme.Error.Render(m.devs.err.Error())
	}
	rows = append(rows, sub, "")
	active := m.focus == focusContent
	for i, r := range m.deviceRows() {
		mark := "  "
		if r.active {
			mark = theme.Playing.Render(theme.Icon.Note + " ")
		}
		kind := r.kind
		switch {
		case r.soloist && r.off && m.engine == engineStarting:
			kind = T("dev.kind.starting")
		case r.soloist && r.off:
			kind = T("dev.kind.off")
		case r.soloist:
			kind = T("dev.kind.light")
		}
		switch {
		case r.fixedVol && !r.off:
			kind += "  " + T("dev.fixedvol") // Spotify bu cihazın sesini uzaktan değiştirtmiyor
		case r.vol >= 0 && !r.off:
			kind += "  " + fmt.Sprintf("%3d%%", r.vol)
		}
		rows = append(rows, mark+row(r.name, kind, w-2, active && i == m.devs.cursor))
	}
	rows = append(rows, "", theme.Faint.Render(T("dev.help")))
	return rows
}
