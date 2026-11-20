package ui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/i18n"
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
	setAutostart
	setVolume
	setExtras
	setAccount
	setConfigFile
)

// settingNames, çeviri anahtarlarında kullanılan adlar (set.name.X, set.help.X).
var settingNames = map[settingID]string{
	setTheme: "theme", setAccent: "accent", setCovers: "covers", setLanguage: "language",
	setAutostart: "autostart", setVolume: "volume", setExtras: "extras",
	setAccount: "account", setConfigFile: "config",
}

type settingsState struct {
	cursor int // settingRows içinde, yalnızca ayar satırları sayılır
}

// settingRow, sayfanın bir satırı: bölüm başlığı (id < 0) ya da ayar.
type settingRow struct {
	id      settingID
	section string // başlık satırları için çeviri anahtarı
}

var settingRows = []settingRow{
	{id: -1, section: "set.look"},
	{id: setTheme}, {id: setAccent}, {id: setCovers}, {id: setLanguage},
	{id: -1, section: "set.speaker"},
	{id: setAutostart}, {id: setVolume},
	{id: -1, section: "set.extras"},
	{id: setExtras},
	{id: -1, section: "set.account"},
	{id: setAccount}, {id: setConfigFile},
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

func (m Model) settingSelected() settingID {
	ids := settingIDs()
	return ids[max(0, min(m.set.cursor, len(ids)-1))]
}

// ---- değerler ----

func themeIndex(name string) int {
	for i, p := range theme.Presets {
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
		return T("theme." + theme.Presets[themeIndex(c.Look.Theme)].Name)
	case setAccent:
		if theme.AccentHex(c.Look.Accent) == "" {
			return T("accent.theme")
		}
		return T("accent." + c.Look.Accent)
	case setCovers:
		if c.Look.CoversOn() {
			return T("set.covers.pixel")
		}
		return T("set.off")
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
	case setExtras:
		return onOff(c.Extras.Enabled)
	case setAccount, setConfigFile:
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
		c.Look.Theme = theme.Presets[cycle(themeIndex(c.Look.Theme), d, len(theme.Presets))].Name
		theme.Apply(c.Look.Theme, c.Look.Accent)
	case setAccent:
		names := accentNames()
		i := 0
		for j, n := range names {
			if n == c.Look.Accent {
				i = j
			}
		}
		c.Look.Accent = names[cycle(i, d, len(names))]
		theme.Apply(c.Look.Theme, c.Look.Accent)
	case setCovers:
		if c.Look.CoversOn() {
			c.Look.Covers = "off"
		} else {
			c.Look.Covers = "pixel"
		}
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
	case setAutostart:
		c.Soloist.Autostart = !c.Soloist.Autostart
	case setVolume:
		c.Soloist.InitialVolume = max(0, min(100, c.Soloist.InitialVolume+5*d))
	case setExtras:
		c.Extras.Enabled = !c.Extras.Enabled
		if !c.Extras.Enabled {
			m.prof.view = nil // ✱ bilgisi artık gösterilmez
		}
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
	return m.note(note), nil
}

func (m Model) settingsKey(k string) (Model, tea.Cmd, bool) {
	n := len(settingIDs())
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
		if id == setAccount || id == setConfigFile {
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

func (m Model) settingsView(w int) []string {
	rows := []string{theme.Title.Render(T("page.settings")), theme.Dim.Render(T("set.sub")), ""}
	active := m.focus == focusContent
	sel := m.settingSelected()
	for _, r := range settingRows {
		if r.id < 0 {
			if len(rows) > 3 {
				rows = append(rows, "")
			}
			rows = append(rows, theme.Dim.Italic(true).Render(T(r.section)))
			continue
		}
		value := m.settingValue(r.id)
		selected := active && r.id == sel
		if selected && r.id != setAccount && r.id != setConfigFile {
			value = "‹ " + value + " ›"
		}
		line := "  " + row(T("set.name."+settingNames[r.id]), value, w-8, selected)
		if sw := m.settingSwatch(r.id); sw != "" {
			line += "  " + sw
		}
		rows = append(rows, line)
	}
	// Seçili ayarın kısa açıklaması.
	if key := "set.help." + settingNames[sel]; T(key) != key {
		rows = append(rows, "", theme.Faint.Render(truncate(T(key), w)))
	}
	return rows
}
