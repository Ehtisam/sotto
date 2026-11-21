package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/cli"
	"github.com/Ehtisam/sotto/internal/i18n"
	"github.com/Ehtisam/sotto/internal/keys"
	"github.com/Ehtisam/sotto/internal/setup"
	"github.com/Ehtisam/sotto/internal/soloist"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// VIII · Ayarlar: görünüm, hoparlör, ✱ özellikleri ve hesap kısayolları.
// ← → ya da enter değeri değiştirir; değişiklik hemen uygulanır ve
// config.toml'a yazılır.

type settingID int

const (
	setTheme settingID = iota
	setAccent
	setCovers
	setLanguage
	setLineNumbers
	setBorder
	setIcons
	setProgress
	setStrip
	setMenuPos
	setLayout
	setButtons
	setMouse
	setVisualizer
	setKeymap
	setThemes
	setAutostart
	setResume
	setCrossfade
	setQuality
	setNormalize
	setVolume
	setSoloist
	setSoloistAuto
	setNotify
	setMedia
	setStartMenu
	setDiscord
	setExtras
	setPrivate
	setAccount
	setConfigFile
)

// settingNames, çeviri anahtarlarında kullanılan adlar (set.name.X, set.help.X).
var settingNames = map[settingID]string{
	setTheme: "theme", setAccent: "accent", setCovers: "covers", setLanguage: "language",
	setLineNumbers: "linenumbers", setBorder: "border", setIcons: "icons", setProgress: "progress", setStrip: "strip", setMenuPos: "menupos", setLayout: "layout", setButtons: "buttons",
	setMouse: "mouse", setVisualizer: "visualizer", setKeymap: "keymap", setThemes: "themes",
	setAutostart: "autostart", setResume: "resume", setCrossfade: "crossfade", setQuality: "quality", setNormalize: "normalize", setVolume: "volume", setSoloist: "soloist", setSoloistAuto: "soloistauto",
	setNotify: "notify", setMedia: "media", setStartMenu: "startmenu", setDiscord: "discord", setExtras: "extras",
	setPrivate: "private", setAccount: "account", setConfigFile: "config",
}

type settingsState struct {
	cursor int // settingRows içinde, yalnızca ayar satırları sayılır

	build    *soloist.Build  // kurulu Soloist sürümü (nil: bilinmiyor)
	buildErr error           // okunamadı / kurulu değil
	updating *setup.Progress // Soloist indiriliyor
	warned   bool            // süre uyarısı bu açılışta bir kez
	applySeq int             // Soloist ses ayarlarının son değişikliği
	find     finder          // / ile ayarlar içinde arama
}

// applyPrefsMsg: ses ayarı değişti, bekleme bitti; seq hâlâ sonuncuysa Soloist yeniden açılır.
type applyPrefsMsg struct{ seq int }

// prefsDelay: ses ayarı son değişiklikten bu kadar sonra uygulanır.
var prefsDelay = 1500 * time.Millisecond

// settingRow, sayfanın bir satırı: bölüm başlığı (id < 0) ya da ayar.
type settingRow struct {
	id      settingID
	section string // başlık satırları için çeviri anahtarı
}

var settingRows = []settingRow{
	{id: -1, section: "set.look"},
	{id: setTheme}, {id: setAccent}, {id: setCovers}, {id: setLanguage},
	{id: setLineNumbers}, {id: setBorder}, {id: setIcons}, {id: setProgress}, {id: setStrip}, {id: setMenuPos}, {id: setLayout}, {id: setMouse}, {id: setButtons}, {id: setVisualizer},
	{id: setKeymap}, {id: setThemes},
	{id: -1, section: "set.speaker"},
	{id: setAutostart}, {id: setResume}, {id: setCrossfade}, {id: setQuality}, {id: setNormalize},
	{id: setVolume}, {id: setSoloist}, {id: setSoloistAuto},
	{id: -1, section: "set.desktop"},
	{id: setNotify}, {id: setMedia}, {id: setStartMenu}, {id: setDiscord},
	{id: -1, section: "set.extras"},
	{id: setExtras},
	{id: -1, section: "set.account"},
	{id: setPrivate}, {id: setAccount}, {id: setConfigFile},
}

// settingIDs, seçilebilir satırlar sırasıyla.
func settingIDs() []settingID {
	var out []settingID
	for _, r := range settingRows {
		if r.id >= 0 {
			out = append(out, r.id)
		}
	}
	return out
}

// shownSettingIDs: süzgeç açıksa yalnız uyan ayarlar (sayfadaki sırayla).
func (m Model) shownSettingIDs() []settingID {
	all := settingIDs()
	q := m.set.find.query
	if q == "" {
		return all
	}
	section := map[settingID]string{}
	cur := ""
	for _, r := range settingRows {
		if r.id < 0 {
			cur = T(r.section)
		} else {
			section[r.id] = cur
		}
	}
	text := func(i int) string {
		id := all[i]
		name := settingNames[id]
		return T("set.name."+name) + " " + section[id] + " " + m.settingValue(id) + " " + T("set.help."+name)
	}
	// Bulanık eşleşme uzun açıklamalarda her şeyi yakalıyor: burada sorgunun
	// her sözcüğü metinde geçmeli.
	words := strings.Fields(cli.Fold(q))
	var out []settingID
	for i, id := range all {
		t := cli.Fold(text(i))
		ok := true
		for _, w := range words {
			if !strings.Contains(t, w) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, id)
		}
	}
	return out
}

// settingSelected, seçili ayar; süzgeç hiçbir şey bulmadıysa -1.
func (m Model) settingSelected() settingID {
	ids := m.shownSettingIDs()
	if len(ids) == 0 {
		return -1
	}
	return ids[max(0, min(m.set.cursor, len(ids)-1))]
}

// ---- değerler ----

func themeIndex(name string) int {
	for i, p := range theme.All() {
		if p.Name == name {
			return i
		}
	}
	return 0
}

// accentNames: "" temanın kendi vurgusu, sonra hazır renkler.
func accentNames() []string {
	out := []string{""}
	for _, a := range theme.Accents {
		out = append(out, a.Name)
	}
	return out
}

func cycle(i, d, n int) int { return ((i+d)%n + n) % n }

// settingValue, satırın sağında görünen değer.
func (m Model) settingValue(id settingID) string {
	c := m.cfg
	onOff := func(b bool) string {
		if b {
			return T("set.on")
		}
		return T("set.off")
	}
	switch id {
	case setTheme:
		p := theme.All()[themeIndex(c.Look.Theme)]
		if p.User {
			return p.Name
		}
		return T("theme." + p.Name)
	case setAccent:
		if theme.AccentHex(c.Look.Accent) == "" {
			return T("accent.theme")
		}
		return T("accent." + c.Look.Accent)
	case setCovers:
		switch {
		case !c.Look.CoversOn():
			return T("set.off")
		case c.Look.Covers == "pixel":
			return T("set.covers.pixel")
		case m.gfx.sixel:
			return T("set.covers.image")
		}
		return T("set.covers.nosixel")
	case setLanguage:
		if i18n.Lang() == "tr" {
			return "Türkçe"
		}
		return "English"
	case setAutostart:
		if c.Soloist.Autostart {
			return T("set.autostart.on")
		}
		return T("set.autostart.off")
	case setVolume:
		return fmt.Sprintf("%d", c.Soloist.InitialVolume)
	case setResume:
		return T("set.onstart." + orDefault(c.Soloist.OnStart, "paused"))
	case setCrossfade:
		if c.Soloist.Crossfade <= 0 {
			return T("set.off")
		}
		return F("set.crossfade.s", c.Soloist.Crossfade)
	case setQuality:
		return T("set.quality." + orDefault(c.Soloist.Quality, "very_high"))
	case setNormalize:
		return onOff(c.Soloist.Normalize)
	case setSoloist:
		return m.soloistValue()
	case setSoloistAuto:
		return onOff(c.Soloist.AutoUpdate)
	case setNotify:
		switch c.Desktop.Notify {
		case "always":
			return T("set.notify.always")
		case "unfocused":
			return T("set.notify.unfocused")
		}
		return T("set.off")
	case setMedia:
		return onOff(c.Desktop.MediaControls)
	case setStartMenu:
		return onOff(c.Desktop.StartMenu)
	case setDiscord:
		if c.Desktop.Discord && strings.TrimSpace(c.Desktop.DiscordAppID) == "" {
			return T("set.discord.noid")
		}
		return onOff(c.Desktop.Discord)
	case setExtras:
		return onOff(c.Extras.Enabled)
	case setLineNumbers:
		return T("set.ln." + orDefault(c.Look.LineNumbers, "absolute"))
	case setBorder:
		return T("set.border." + orDefault(c.Look.Border, "line"))
	case setIcons:
		ic := orDefault(c.Look.Icons, "unicode")
		return T("set.icons."+ic) + "  " + theme.Icon.Pause + " " + theme.Icon.Note + " " + theme.Icon.Shuffle + " " + theme.Icon.Heart
	case setProgress:
		return T("set.progress."+orDefault(c.Look.Progress, "line")) + "  " + theme.Bar(5, 9)
	case setStrip:
		return T("set.strip." + orDefault(c.Look.Strip, "bottom"))
	case setMenuPos:
		return T("set.menupos." + orDefault(c.Look.Menu, "left"))
	case setLayout:
		return T("set.layout." + orDefault(c.Look.Layout, "plain"))
	case setMouse:
		return onOff(c.Look.Mouse)
	case setButtons:
		return onOff(c.Look.Buttons)
	case setVisualizer:
		v := T("set.viz." + orDefault(c.Look.Visualizer, "off"))
		if m.rt.viz.err != nil {
			v += " · " + m.rt.viz.err.Error()
		}
		return v
	case setPrivate:
		if m.privateOn() {
			return F("priv.until", m.rt.privUntil.Format("15:04"))
		}
		return T("set.off")
	case setAccount, setConfigFile, setKeymap, setThemes:
		return T("set.open")
	}
	return ""
}

// settingSwatch, renk ayarlarının yanındaki önizleme kareleri.
func (m Model) settingSwatch(id settingID) string {
	switch id {
	case setTheme:
		p := theme.PaletteOf(m.cfg.Look.Theme)
		return theme.Swatch(p.Accent, p.Gold, p.Text)
	case setAccent:
		p := theme.Resolve(m.cfg.Look.Theme, m.cfg.Look.Accent)
		return theme.Swatch(p.Accent, p.AccentHi, p.Selected)
	}
	return ""
}

// change, seçili ayarı d yönünde değiştirir (d: -1 ya da 1), uygular ve kaydeder.
func (m Model) change(id settingID, d int) (Model, tea.Cmd) {
	c := &m.cfg
	switch id {
	case setTheme:
		all := theme.All()
		c.Look.Theme = all[cycle(themeIndex(c.Look.Theme), d, len(all))].Name
		applyLook(*c)
	case setAccent:
		names := accentNames()
		i := 0
		for j, n := range names {
			if n == c.Look.Accent {
				i = j
			}
		}
		c.Look.Accent = names[cycle(i, d, len(names))]
		applyLook(*c)
	case setCovers:
		modes := []string{"image", "pixel", "off"}
		i := 0
		for j, mode := range modes {
			if mode == c.Look.Covers {
				i = j
			}
		}
		c.Look.Covers = modes[cycle(i, d, len(modes))]
	case setLanguage:
		if i18n.Lang() == "tr" {
			c.Language = "en"
		} else {
			c.Language = "tr"
		}
		i18n.Set(c.Language)
		for i := range m.lib.entries { // yüklenirken çevrilmiş ad
			if m.lib.entries[i].kind == kindLiked {
				m.lib.entries[i].name = T("lib.liked")
			}
		}
	case setLineNumbers:
		c.Look.LineNumbers = cycleOf([]string{"absolute", "relative", "off"}, orDefault(c.Look.LineNumbers, "absolute"), d)
	case setBorder:
		c.Look.Border = cycleOf(theme.BorderKinds, orDefault(c.Look.Border, "line"), d)
		applyLook(*c)
	case setIcons:
		c.Look.Icons = cycleOf(theme.IconSets, orDefault(c.Look.Icons, "unicode"), d)
		applyLook(*c)
	case setProgress:
		c.Look.Progress = cycleOf(theme.ProgressKinds, orDefault(c.Look.Progress, "line"), d)
		applyLook(*c)
	case setStrip:
		c.Look.Strip = cycleOf([]string{"bottom", "top"}, orDefault(c.Look.Strip, "bottom"), d)
		m.rt.gfx.drawn = nil // kapaklar yer değiştirdi
	case setMenuPos:
		c.Look.Menu = cycleOf([]string{"left", "right", "hidden"}, orDefault(c.Look.Menu, "left"), d)
	case setLayout:
		c.Look.Layout = cycleOf([]string{"plain", "boxed"}, orDefault(c.Look.Layout, "plain"), d)
	case setMouse:
		c.Look.Mouse = !c.Look.Mouse
	case setButtons:
		c.Look.Buttons = !c.Look.Buttons
	case setVisualizer:
		c.Look.Visualizer = cycleOf([]string{"now", "strip", "off"}, orDefault(c.Look.Visualizer, "off"), d)
		m.rt.viz.tried, m.rt.viz.err = time.Time{}, nil // hemen yeniden denensin
	case setKeymap:
		if err := openBrowser(keys.File(c.ConfigDir)); err != nil {
			return m.say(err.Error()), nil
		}
		return m.note(T("set.keymap.opened")), nil
	case setThemes:
		dir := theme.UserDir(c.ConfigDir)
		_ = os.MkdirAll(dir, 0o700)
		if err := openBrowser(dir); err != nil {
			return m.say(err.Error()), nil
		}
		return m.note(T("set.themes.opened")), nil
	case setAutostart:
		c.Soloist.Autostart = !c.Soloist.Autostart
	case setVolume:
		c.Soloist.InitialVolume = max(0, min(100, c.Soloist.InitialVolume+5*d))
	case setResume:
		c.Soloist.OnStart = cycleOf([]string{"paused", "play", "off"}, orDefault(c.Soloist.OnStart, "paused"), d)
	case setCrossfade, setQuality, setNormalize:
		switch id {
		case setCrossfade:
			i := 0
			for j, s := range soloist.CrossfadeSteps {
				if s <= c.Soloist.Crossfade {
					i = j
				}
			}
			c.Soloist.Crossfade = soloist.CrossfadeSteps[cycle(i, d, len(soloist.CrossfadeSteps))]
		case setQuality:
			c.Soloist.Quality = cycleOf(soloist.Qualities, orDefault(c.Soloist.Quality, "very_high"), d)
		case setNormalize:
			c.Soloist.Normalize = !c.Soloist.Normalize
		}
		if err := c.Save(); err != nil {
			return m.say(err.Error()), nil
		}
		// Soloist ayarı yalnız açılırken okur. Art arda basışlarda bir kez
		// yeniden başlatılsın diye kısa bir bekleme.
		m.set.applySeq++
		seq := m.set.applySeq
		if m.rt.proc == nil || m.rt.proc.Attached {
			return m.note(T("set.applied.next")), nil
		}
		return m.note(T("set.saved")), tea.Tick(prefsDelay, func(time.Time) tea.Msg { return applyPrefsMsg{seq} })
	case setSoloist:
		return m.updateSoloist(false)
	case setSoloistAuto:
		c.Soloist.AutoUpdate = !c.Soloist.AutoUpdate
	case setNotify:
		modes := []string{"unfocused", "always", "off"}
		i := 0
		for j, mode := range modes {
			if mode == c.Desktop.Notify {
				i = j
			}
		}
		c.Desktop.Notify = modes[cycle(i, d, len(modes))]
	case setDiscord:
		c.Desktop.Discord = !c.Desktop.Discord
		if err := c.Save(); err != nil {
			return m.say(err.Error()), nil
		}
		m.syncDiscord()
		if c.Desktop.Discord && strings.TrimSpace(c.Desktop.DiscordAppID) == "" {
			return m.say(T("set.discord.howid")), nil
		}
		return m.note(T("set.saved")), nil
	case setStartMenu:
		c.Desktop.StartMenu = !c.Desktop.StartMenu
		if err := c.Save(); err != nil {
			return m.say(err.Error()), nil
		}
		on := c.Desktop.StartMenu
		return m.note(T("set.saved")), func() tea.Msg {
			if err := SyncStartMenu(on); err != nil {
				return flashMsg(err.Error())
			}
			return nil
		}
	case setMedia:
		c.Desktop.MediaControls = !c.Desktop.MediaControls
		if err := c.Save(); err != nil {
			return m.say(err.Error()), nil
		}
		return m.note(T("set.saved")), m.setMediaControls(c.Desktop.MediaControls)
	case setExtras:
		c.Extras.Enabled = !c.Extras.Enabled
		if !c.Extras.Enabled {
			m.prof.view = nil // ✱ bilgisi artık gösterilmez
		}
	case setPrivate:
		return m.togglePrivate()
	case setAccount:
		if err := openBrowser("https://www.spotify.com/account/"); err != nil {
			return m.say(err.Error()), nil
		}
		return m.note(T("set.opened")), nil
	case setConfigFile:
		if err := openBrowser(c.ConfigFile()); err != nil {
			return m.say(err.Error()), nil
		}
		return m.note(T("set.opened")), nil
	}
	if err := c.Save(); err != nil {
		return m.say(err.Error()), nil
	}
	note := T("set.saved")
	if id == setAutostart || id == setVolume {
		note = T("set.saved.next") // bir sonraki açılışta geçerli
	}
	// Renkler ya da yerleşim değişince ekran baştan boyanır ve kapak resimlerinin
	// üstü de boyanır: resimleri (boyut değişimindeki gibi) yeniden gönder.
	switch id {
	case setTheme, setAccent, setCovers, setBorder, setIcons, setProgress, setStrip, setLineNumbers, setLanguage, setMenuPos, setLayout:
		m.rt.gfx.drawn = nil
		return m.note(note), m.afterResize()
	}
	return m.note(note), nil
}

func (m Model) settingsKey(k string) (Model, tea.Cmd, bool) {
	n := len(m.shownSettingIDs())
	if n == 0 && k != "esc" && k != "backspace" {
		return m, nil, k == "up" || k == "k" || k == "down" || k == "j" || k == "enter" || k == "left" || k == "right"
	}
	switch k {
	case "up", "k":
		m.set.cursor = max(0, m.set.cursor-1)
	case "down", "j":
		m.set.cursor = min(n-1, m.set.cursor+1)
	case "home", "g":
		m.set.cursor = 0
	case "end", "G":
		m.set.cursor = n - 1
	case "right", "enter":
		mm, cmd := m.change(m.settingSelected(), 1)
		return mm, cmd, true
	case "left":
		id := m.settingSelected()
		if id == setAccount || id == setConfigFile || id == setSoloist {
			return m, nil, true // geri yönü olmayan satırlar
		}
		mm, cmd := m.change(id, -1)
		return mm, cmd, true
	case "esc", "backspace":
		m.focus = focusMenu
	default:
		return m, nil, false
	}
	return m, nil, true
}

func (m Model) settingsView(w, h int) []string {
	rows := []string{theme.Title.Render(T("page.settings")), theme.Dim.Render(T("set.sub")), ""}
	ends := []bool{false, false, false} // satır bir ayar mı (başlık ya da boşluk değil)
	active := m.focus == focusContent
	sel := m.settingSelected()
	shown := m.shownSettingIDs()
	visible := map[settingID]bool{}
	for _, id := range shown {
		visible[id] = true
	}
	if m.set.find.active() {
		rows[2] = findBar(m.set.find, len(shown), len(settingIDs()), w)
		if len(shown) == 0 {
			return append(rows, "", theme.Faint.Render(T("find.none")))
		}
	}
	// hasShown: bölümün (i'den sonraki başlığa kadar) görünen ayarı var mı.
	hasShown := func(i int) bool {
		for _, r := range settingRows[i+1:] {
			if r.id < 0 {
				return false
			}
			if visible[r.id] {
				return true
			}
		}
		return false
	}
	for i, r := range settingRows {
		if r.id >= 0 && !visible[r.id] {
			continue
		}
		if r.id < 0 {
			if !hasShown(i) {
				continue
			}
			if len(rows) > 3 {
				rows, ends = append(rows, ""), append(ends, false)
			}
			rows, ends = append(rows, theme.Dim.Italic(true).Render(T(r.section))), append(ends, false)
			continue
		}
		value := m.settingValue(r.id)
		selected := active && r.id == sel
		if selected && r.id != setAccount && r.id != setConfigFile && r.id != setSoloist {
			value = "‹ " + value + " ›"
		}
		line := "  " + row(T("set.name."+settingNames[r.id]), value, w-8, selected)
		if sw := m.settingSwatch(r.id); sw != "" {
			line += "  " + sw
		}
		rows, ends = append(rows, line), append(ends, true)
	}
	// Seçili ayarın kısa açıklaması.
	var help []string
	if key := "set.help." + settingNames[sel]; sel >= 0 && T(key) != key {
		help = []string{"", theme.Faint.Render(truncate(T(key), w))}
	}
	// Pencereye sığmıyorsa seçili satır görünecek kadar kaydır (başlık kalır).
	if room := h - len(help) - 3; h > 0 && len(rows)-3 > room && room > 2 {
		selRow := 0
		for i, r := range rows[3:] {
			if strings.Contains(r, T("set.name."+settingNames[sel])) {
				selRow = i
				break
			}
		}
		body, isSet := rows[3:], ends[3:]
		start := max(0, min(selRow-room/2, len(body)-room))
		end := start + room
		// Kesilen yerde bölüm başlığı altı boş kalmasın: sondaki başlık ve
		// boşluklar atılır (o bölüm aşağı kayınca bütün olarak görünür).
		for end > start+1 && !isSet[end-1] {
			end--
		}
		for start < end-1 && body[start] == "" {
			start++
		}
		rows = append(rows[:3:3], body[start:end]...)
	}
	return append(rows, help...)
}

// ---- Soloist sürümü ve güncelleme ----

type soloistBuildMsg struct {
	build soloist.Build
	err   error
}

// soloistUpdatedMsg: indirme bitti. newer: indirilen daha yeniydi ve yerine kondu.
type soloistUpdatedMsg struct {
	build soloist.Build
	newer bool
	auto  bool
	err   error
}

func (m Model) readSoloistBuild() tea.Cmd {
	path := m.cfg.SoloistPath()
	return func() tea.Msg {
		if _, err := os.Stat(path); err != nil {
			return soloistBuildMsg{err: err}
		}
		b, err := readBuild(context.Background(), path)
		return soloistBuildMsg{b, err}
	}
}

// expiryWarnDays: süresine bu kadar gün kalınca uyarılır ya da (ayar açıksa) güncellenir.
const expiryWarnDays = 14

func (m Model) soloistValue() string {
	if p := m.set.updating; p != nil {
		pct := 0
		if t := p.Total.Load(); t > 0 {
			pct = int(100 * p.Done.Load() / t)
		}
		return fmt.Sprintf(T("set.soloist.dl"), pct)
	}
	if m.set.build == nil {
		if m.set.buildErr != nil {
			return T("set.soloist.none")
		}
		return "…"
	}
	b := m.set.build
	if left := b.DaysLeft(m.now); left > 0 {
		return fmt.Sprintf(T("set.soloist.left"), shortVersion(b.Version), left)
	}
	return fmt.Sprintf(T("set.soloist.expired"), shortVersion(b.Version))
}

// shortVersion: "1.3.10.87" → "1.3.10".
func shortVersion(v string) string {
	n := 0
	for i, r := range v {
		if r == '.' {
			if n++; n == 3 {
				return v[:i]
			}
		}
	}
	return v
}

func (m Model) updateSettingsMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case applyPrefsMsg:
		if msg.seq != m.set.applySeq {
			return m, nil // arkasından bir değişiklik daha geldi
		}
		return m.restartSoloist()

	case soloistBuildMsg:
		if msg.err != nil {
			m.set.build, m.set.buildErr = nil, msg.err
			return m, nil
		}
		b := msg.build
		m.set.build, m.set.buildErr = &b, nil
		left := b.DaysLeft(m.now)
		if left > expiryWarnDays || m.set.updating != nil {
			return m, nil
		}
		// Süre yaklaştı: ayar açıksa yeni sürümü arka planda indir (günde en çok
		// bir kez; süresi dolduysa hemen), değilse bir kez uyar.
		if m.cfg.Soloist.AutoUpdate {
			if left <= 0 || m.now.Sub(soloist.CheckedSince(m.cfg.SoloistPath())) > 20*time.Hour {
				m.set.warned = true
				mm, cmd := m.updateSoloist(true)
				if left <= 0 {
					return mm.note(T("flash.soloist.autoexpired")), cmd
				}
				return mm.note(fmt.Sprintf(T("flash.soloist.autoupdating"), left)), cmd
			}
			return m, nil // bugün bakıldı, yeni sürüm yoktu
		}
		if !m.set.warned {
			m.set.warned = true
			if left <= 0 {
				return m.say(T("flash.soloist.expired")), nil
			}
			return m.say(fmt.Sprintf(T("flash.soloist.expiring"), left)), nil
		}

	case soloistUpdatedMsg:
		m.set.updating = nil
		if msg.err != nil {
			return m.say(fmt.Sprintf(T("flash.soloist.updatefail"), msg.err)), nil
		}
		if !msg.newer {
			if msg.auto {
				if m.set.build != nil && m.set.build.DaysLeft(m.now) <= 0 {
					return m.say(T("flash.soloist.nonew")), nil
				}
				return m, nil // sessizce yarın yeniden bakılır
			}
			return m.note(T("set.soloist.latest")), nil
		}
		b := msg.build
		m.set.build, m.set.buildErr = &b, nil
		if m.rt.proc != nil { // çalışan Soloist eski sürümle devam eder, müzik kesilmez
			return m.note(T("set.soloist.ready")), nil
		}
		m = m.note(T("set.soloist.updated"))
		if m.engine == engineFailed && (m.cfg.Soloist.Autostart || errors.Is(m.engineErr, soloist.ErrExpired)) {
			m.engine, m.engineErr = engineStarting, nil
			return m, startSoloist(m.cfg)
		}
	}
	return m, nil
}

// updateSoloist, Spotify'ın yayımladığı en yeni Soloist'i resmî adresten
// çalışanın yanına indirir; daha yeniyse yerine koyar. Çalışan Soloist
// durdurulmaz: eski sürümle devam eder, bir sonraki başlatışta yenisi açılır.
func (m Model) updateSoloist(auto bool) (Model, tea.Cmd) {
	if m.set.updating != nil {
		return m, nil
	}
	url, err := setup.SoloistURL()
	if err != nil {
		if auto {
			return m, nil
		}
		return m.say(T("wiz.dl.unsupported")), nil
	}
	p := &setup.Progress{}
	m.set.updating = p
	exe := m.cfg.SoloistPath()
	var cur *soloist.Build
	if m.set.build != nil {
		b := *m.set.build
		cur = &b
	}
	cmd := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		soloist.MarkChecked(exe, time.Now())
		staged := soloist.StagedPath(exe)
		if err := downloadSoloist(ctx, url, setup.ExeName("soloist"), staged, p); err != nil {
			_ = os.Remove(staged)
			return soloistUpdatedMsg{auto: auto, err: err}
		}
		nb, err := readBuild(ctx, staged)
		if err != nil {
			_ = os.Remove(staged)
			return soloistUpdatedMsg{auto: auto, err: err}
		}
		if cur != nil && !nb.Built.After(cur.Built) {
			_ = os.Remove(staged)
			return soloistUpdatedMsg{build: nb, auto: auto}
		}
		if err := soloist.Swap(exe); err != nil {
			_ = os.Remove(staged)
			return soloistUpdatedMsg{auto: auto, err: err}
		}
		return soloistUpdatedMsg{build: nb, newer: true, auto: auto}
	}
	if !auto {
		m = m.note(T("set.soloist.updating"))
	}
	return m, cmd
}

// Testlerde değiştirilir.
var (
	downloadSoloist = setup.Fetch
	readBuild       = soloist.ReadBuild
)

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// cycleOf, listede cur'dan d adım ötesi.
func cycleOf(list []string, cur string, d int) string {
	i := 0
	for j, v := range list {
		if v == cur {
			i = j
		}
	}
	return list[cycle(i, d, len(list))]
}
