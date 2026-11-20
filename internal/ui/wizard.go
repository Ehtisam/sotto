package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/bridge"
	"github.com/Ehtisam/sotto/internal/setup"
	"github.com/Ehtisam/sotto/internal/spotify"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// İlk kurulum sihirbazı. Client ID yoksa kendiliğinden, `sotto setup` ile de
// istenince açılır. Adımlar:
//
//  1. Hoş geldin
//  2. Spotify uygulaması: geliştirici panelinde uygulama, Client ID, giriş
//  3. Hoparlör: hafif mod (Soloist anahtarı, resmî adresten indirme, bir kez
//     eşleşme) ya da yalnızca kumanda
//  4. Ekstralar ✱ (isteğe bağlı): go-librespot'u indirme, bir kez giriş
//  5. Bitti
//
// Her adım yapılmışsa ✓ gösterir; yeniden kurulumda hiçbir şey zorla
// yeniden yapılmaz.

type wizStep int

const (
	wizWelcome wizStep = iota
	wizApp
	wizSpeaker
	wizExtras
	wizDone
)

const (
	dashboardURL  = "https://developer.spotify.com/dashboard"
	soloistKeyURL = "https://developer.spotify.com/dashboard/soloist"
	redirectURI   = "http://127.0.0.1:8765/callback"
)

type wizardState struct {
	on   bool
	step wizStep

	input string // yazı kutusu: Client ID ya da Soloist anahtarı
	err   string

	choice  int  // seçim adımlarında imleç
	speaker int  // 0 bilinmiyor, 1 hafif mod, 2 yalnızca kumanda
	extras  int  // 0 bilinmiyor, 1 açık, 2 kapalı
	started bool // hafif mod: Soloist bu adımda başlatıldı

	dl      *setup.Progress // süren indirme
	dlWhat  string          // "soloist" ya da "librespot"
	dlError string

	bridgeLogin string // go-librespot girişi için tarayıcı adresi
	bridgeAsked bool   // giriş sayfası açıldı
	bridgeOK    bool
	bridgeBusy  bool
}

type wizDownloadedMsg struct {
	what string
	err  error
}

type wizBridgeMsg struct {
	login string
	ok    bool
	err   error
}

type wizBridgeRetryMsg struct{}

// StartSetup, sihirbazı açar (`sotto setup`).
func (m Model) StartSetup() Model {
	m.wiz = wizardState{on: true}
	return m
}

var clientIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)

func (m Model) soloistInstalled() bool {
	_, err := os.Stat(m.cfg.SoloistPath())
	return err == nil
}

// ---- tuşlar ----

func (m Model) wizardKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	k := msg.String()
	w := &m.wiz
	if k == "esc" {
		if w.step == wizWelcome || w.step == wizDone {
			w.on = false
			return m, nil
		}
		w.step--
		w.err, w.input, w.choice = "", "", 0
		return m, nil
	}

	// Yazı kutusu açıksa harfler ona gider.
	if m.wizTyping() {
		switch k {
		case "backspace":
			if r := []rune(w.input); len(r) > 0 {
				w.input = string(r[:len(r)-1])
			}
			return m, nil
		case "ctrl+u":
			w.input = ""
			return m, nil
		case "enter":
			return m.wizSubmit()
		case "ctrl+o":
			return m.wizOpenPage()
		}
		if t := msg.Key().Text; t != "" {
			w.input += cleanInput(t)
			w.err = ""
		}
		return m, nil
	}

	switch k {
	case "up", "k":
		w.choice = max(0, w.choice-1)
		return m, nil
	case "down", "j":
		w.choice = min(1, w.choice+1)
		return m, nil
	case "o":
		return m.wizOpenPage()
	case "s":
		return m.wizNext(), nil // bu adımı şimdilik atla
	case "enter":
		return m.wizEnter()
	}
	return m, nil
}

// wizardPaste, yapıştırılan metni yazı kutusuna koyar.
func (m Model) wizardPaste(text string) Model {
	if m.wizTyping() {
		m.wiz.input += cleanInput(text)
		m.wiz.err = ""
	}
	return m
}

// cleanInput, yapıştırılan anahtardaki boşlukları ve satır sonlarını atar.
func cleanInput(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || !unicode.IsPrint(r) {
			return -1
		}
		return r
	}, s)
}

// wizTyping: bu adımda yazı kutusu açık mı.
func (m Model) wizTyping() bool {
	switch m.wiz.step {
	case wizApp:
		return m.spState == spNoClient || m.spState == spLoggedOut
	case wizSpeaker:
		return m.wiz.speaker == 1 && m.cfg.SoloistAPIKey() == ""
	}
	return false
}

func (m Model) wizNext() Model {
	m.wiz.step = min(m.wiz.step+1, wizDone)
	m.wiz.err, m.wiz.input, m.wiz.choice = "", "", 0
	return m
}

// wizOpenPage, adımın ilgili sayfasını tarayıcıda açar.
func (m Model) wizOpenPage() (Model, tea.Cmd) {
	url := ""
	switch m.wiz.step {
	case wizApp:
		url = dashboardURL
	case wizSpeaker:
		url = soloistKeyURL
	case wizExtras:
		url = m.wiz.bridgeLogin
	}
	if url == "" {
		return m, nil
	}
	if err := openBrowser(url); err != nil {
		return m.say(err.Error()), nil
	}
	return m.note(T("wiz.opened")), nil
}

// wizSubmit, yazı kutusundaki değeri doğrular ve kaydeder.
func (m Model) wizSubmit() (Model, tea.Cmd) {
	w := &m.wiz
	v := strings.TrimSpace(w.input)
	switch w.step {
	case wizApp:
		if !clientIDPattern.MatchString(v) {
			w.err = T("wiz.app.bad")
			return m, nil
		}
		m.cfg.Spotify.ClientID = strings.ToLower(v)
		if err := m.cfg.Save(); err != nil {
			w.err = err.Error()
			return m, nil
		}
		m.sp = spotify.NewClient(m.cfg.Spotify.ClientID, m.cfg.TokenFile())
		w.input = ""
		m.spState = spLoggingIn
		return m, m.login()
	case wizSpeaker:
		if len(v) < 16 {
			w.err = T("wiz.key.bad")
			return m, nil
		}
		m.cfg.Soloist.APIKey = v
		if err := m.cfg.Save(); err != nil {
			w.err = err.Error()
			return m, nil
		}
		w.input = ""
		return m, nil
	}
	return m, nil
}

// wizEnter, yazı kutusu olmayan durumlarda enter.
func (m Model) wizEnter() (Model, tea.Cmd) {
	w := &m.wiz
	switch w.step {
	case wizWelcome:
		return m.wizNext(), nil
	case wizApp:
		if m.spState == spReady {
			return m.wizNext(), nil
		}
		return m, nil // tarayıcıda onay bekleniyor
	case wizSpeaker:
		switch {
		case w.speaker == 0: // seçim
			if w.choice == 0 {
				w.speaker = 1
				m.cfg.Soloist.Autostart = true
			} else {
				w.speaker = 2
				m.cfg.Soloist.Autostart = false
			}
			if err := m.cfg.Save(); err != nil {
				w.err = err.Error()
			}
			if w.speaker == 2 {
				return m.wizNext(), nil
			}
			return m, nil
		case w.dl != nil:
			return m, nil // iniyor
		case !m.soloistInstalled():
			return m.wizDownload("soloist")
		case m.soloistLive():
			return m.wizNext(), nil // eşleşti
		case m.engine == engineOff || m.engine == engineFailed:
			w.started = true
			m.engine, m.engineErr = engineStarting, nil
			return m, startSoloist(m.cfg)
		}
		return m, nil
	case wizExtras:
		switch {
		case w.extras == 0:
			if w.choice == 0 {
				w.extras = 1
				m.cfg.Extras.Enabled = true
			} else {
				w.extras = 2
				m.cfg.Extras.Enabled = false
			}
			if err := m.cfg.Save(); err != nil {
				w.err = err.Error()
			}
			if w.extras == 2 {
				return m.wizNext(), nil
			}
			return m.wizBridgeStep()
		case w.bridgeOK:
			return m.wizNext(), nil
		default:
			return m.wizBridgeStep()
		}
	case wizDone:
		w.on = false
		if m.spState == spReady {
			return m, tea.Batch(m.enterPage(), m.loadMe())
		}
		return m, nil
	}
	return m, nil
}

// ---- indirme ----

func (m Model) wizDownload(what string) (Model, tea.Cmd) {
	var url, name, dest string
	var err error
	switch what {
	case "soloist":
		url, err = setup.SoloistURL()
		name, dest = setup.ExeName("soloist"), m.cfg.SoloistPath()
	case "librespot":
		url, err = setup.LibrespotURL()
		name = setup.ExeName("go-librespot")
		dest = filepath.Join(m.cfg.LocalDir, "bridge", name)
	}
	if err != nil {
		m.wiz.dlError = T("wiz.dl.unsupported")
		return m, nil
	}
	p := &setup.Progress{}
	m.wiz.dl, m.wiz.dlWhat, m.wiz.dlError = p, what, ""
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		return wizDownloadedMsg{what, setup.Fetch(ctx, url, name, dest, p)}
	}
}

// ---- go-librespot girişi (✱) ----

// wizBridgeStep: go-librespot yoksa indirir, varsa jeton ister; giriş
// gerekiyorsa tarayıcı sayfasını açar ve arada bir yeniden dener.
func (m Model) wizBridgeStep() (Model, tea.Cmd) {
	w := &m.wiz
	if w.dl != nil || w.bridgeBusy {
		return m, nil
	}
	if !m.rt.bridge.Installed() {
		return m.wizDownload("librespot")
	}
	w.bridgeBusy = true
	br := m.rt.bridge
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, err := br.Token(ctx)
		var need *bridge.LoginNeeded
		if errors.As(err, &need) {
			return wizBridgeMsg{login: need.URL}
		}
		return wizBridgeMsg{ok: err == nil, err: err}
	}
}

func (m Model) updateWizard(msg tea.Msg) (Model, tea.Cmd) {
	w := &m.wiz
	switch msg := msg.(type) {
	case wizDownloadedMsg:
		w.dl = nil
		if msg.err != nil {
			w.dlError = msg.err.Error()
			return m, nil
		}
		if msg.what == "soloist" { // indi: hemen başlat, eşleşme beklensin
			w.started = true
			m.engine, m.engineErr = engineStarting, nil
			return m, startSoloist(m.cfg)
		}
		return m.wizBridgeStep()
	case wizBridgeMsg:
		w.bridgeBusy = false
		switch {
		case msg.ok:
			w.bridgeOK, w.bridgeLogin = true, ""
		case msg.login != "":
			w.bridgeLogin = msg.login
			if !w.bridgeAsked { // giriş sayfasını bir kez kendiliğinden aç
				w.bridgeAsked = true
				_ = openBrowser(msg.login)
			}
			return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return wizBridgeRetryMsg{} })
		case msg.err != nil:
			w.err = msg.err.Error()
		}
	case wizBridgeRetryMsg:
		if w.on && w.step == wizExtras && !w.bridgeOK {
			return m.wizBridgeStep()
		}
	}
	return m, nil
}

// ---- çizim ----

func (m Model) wizardView(w int) []string {
	z := m.wiz
	steps := []string{T("wiz.s.welcome"), T("wiz.s.app"), T("wiz.s.speaker"), T("wiz.s.extras"), T("wiz.s.done")}
	var crumbs []string
	for i, s := range steps {
		switch {
		case wizStep(i) == z.step:
			crumbs = append(crumbs, theme.Hi.Underline(true).Render(s))
		case wizStep(i) < z.step:
			crumbs = append(crumbs, theme.Dim.Render(s))
		default:
			crumbs = append(crumbs, theme.Faint.Render(s))
		}
	}
	rows := []string{theme.Title.Render(T("wiz.title")), strings.Join(crumbs, theme.Faint.Render(" · ")), ""}
	para := func(key string, args ...any) {
		for _, line := range wrapText(F(key, args...), w) {
			rows = append(rows, theme.Plain.Render(line))
		}
	}
	dim := func(s string) { rows = append(rows, theme.Dim.Render(truncate(s, w))) }
	ok := func(s string) {
		rows = append(rows, theme.GoldText.Render("✓ ")+theme.Plain.Render(truncate(s, w-2)))
	}
	cont := func() { rows = append(rows, "", theme.Dim.Render(truncate(T("wiz.continue"), w))) } // adım tamam
	numbered := func(keys ...string) {
		for i, k := range keys {
			for j, line := range wrapText(T(k), w-4) {
				lead := "    "
				if j == 0 {
					lead = fmt.Sprintf(" %d. ", i+1)
				}
				rows = append(rows, theme.Faint.Render(lead)+theme.Plain.Render(line))
			}
		}
	}
	box := func(label string) {
		cursor := theme.Hi.Render("▏")
		shown := z.input
		if z.step == wizSpeaker && len(shown) > 6 { // anahtar ekranda açık durmasın
			shown = shown[:4] + strings.Repeat("•", len(shown)-4)
		}
		rows = append(rows, "", theme.Dim.Render(label), theme.Hi.Render("› ")+theme.Plain.Render(truncate(shown, w-4))+cursor)
	}
	choices := func(a, b string) {
		for i, s := range []string{a, b} {
			mark := "  "
			if i == z.choice {
				mark = theme.Hi.Render("› ")
			}
			rows = append(rows, mark+row(s, "", w-2, i == z.choice))
		}
	}
	progress := func() {
		if z.dl == nil {
			return
		}
		done, total := z.dl.Done.Load(), z.dl.Total.Load()
		label := fmt.Sprintf("%.1f MB", float64(done)/1e6)
		if total > 0 {
			label = fmt.Sprintf("%.1f / %.1f MB", float64(done)/1e6, float64(total)/1e6)
		}
		barW := max(min(w-len(label)-2, 40), 4)
		rows = append(rows, "", bar(time.Duration(done), time.Duration(max(total, 1)), barW)+"  "+theme.Dim.Render(label))
	}

	switch z.step {
	case wizWelcome:
		para("wiz.welcome.1")
		rows = append(rows, "")
		para("wiz.welcome.2")
		rows = append(rows, "")
		dim(T("wiz.welcome.3"))

	case wizApp:
		switch m.spState {
		case spReady:
			name := m.me.DisplayName
			if name == "" {
				name = "Spotify"
			}
			ok(F("wiz.app.ok", name))
			cont()
		case spLoggingIn:
			para("wiz.app.waiting")
		default:
			para("wiz.app.intro")
			rows = append(rows, "")
			numbered("wiz.app.1", "wiz.app.2", "wiz.app.3", "wiz.app.4")
			rows = append(rows, "", theme.Faint.Render("   Redirect URI  ")+theme.Plain.Render(redirectURI))
			box(T("wiz.app.box"))
		}

	case wizSpeaker:
		switch {
		case z.speaker == 0:
			para("wiz.speaker.intro")
			rows = append(rows, "")
			choices(T("wiz.speaker.light"), T("wiz.speaker.remote"))
			rows = append(rows, "")
			for _, line := range wrapText(T("wiz.speaker.switch"), w) {
				rows = append(rows, theme.Dim.Render(line))
			}
		case z.speaker == 2:
			ok(T("wiz.speaker.remote.ok"))
			cont()
		case m.cfg.SoloistAPIKey() == "":
			para("wiz.key.intro")
			rows = append(rows, "")
			numbered("wiz.key.1", "wiz.key.2", "wiz.key.3")
			box(T("wiz.key.box"))
		case z.dl != nil:
			para("wiz.dl.soloist")
			progress()
		case !m.soloistInstalled():
			if z.dlError != "" {
				rows = append(rows, theme.Error.Render(truncate(z.dlError, w)), "")
			}
			para("wiz.dl.ask", m.cfg.SoloistPath())
			dim(T("wiz.dl.why"))
		case m.soloistLive():
			ok(T("wiz.pair.ok"))
			cont()
		default:
			ok(T("wiz.key.ok"))
			ok(T("wiz.dl.ok"))
			rows = append(rows, "")
			if msg := m.engineMessage(); len(msg) > 0 && z.started {
				rows = append(rows, msg...)
			} else {
				para("wiz.pair.start")
			}
		}

	case wizExtras:
		switch {
		case z.extras == 0:
			para("wiz.extras.intro")
			rows = append(rows, "")
			choices(T("wiz.extras.on"), T("wiz.extras.off"))
		case z.dl != nil:
			para("wiz.dl.librespot")
			progress()
		case z.dlError != "":
			rows = append(rows, theme.Error.Render(truncate(z.dlError, w)), "")
			para("wiz.dl.retry")
		case z.bridgeOK:
			ok(T("wiz.extras.ok"))
			cont()
		case z.bridgeLogin != "":
			para("wiz.extras.login")
			dim(T("wiz.extras.login.again"))
		default:
			para("wiz.extras.starting")
		}

	case wizDone:
		para("wiz.done.1")
		rows = append(rows, "")
		if m.spState == spReady {
			ok(T("wiz.done.app"))
		}
		switch {
		case m.soloistLive():
			ok(T("wiz.done.light"))
		case !m.cfg.Soloist.Autostart:
			ok(T("wiz.done.remote"))
		}
		if z.bridgeOK {
			ok(T("wiz.done.extras"))
		}
		rows = append(rows, "")
		dim(T("wiz.done.again"))
	}

	if z.err != "" {
		rows = append(rows, "", theme.Error.Render(truncate(z.err, w)))
	}
	return rows
}

// wizardHints, sihirbazdayken alt satırdaki kısayollar.
func (m Model) wizardHints() []hint {
	z := m.wiz
	hs := []hint{}
	switch {
	case m.wizTyping():
		hs = append(hs, hint{T("wiz.h.paste"), T("wiz.h.pastehint")}, hint{"enter", T("wiz.h.save")}, hint{"ctrl+o", T("wiz.h.open")})
	case (z.step == wizSpeaker && z.speaker == 0) || (z.step == wizExtras && z.extras == 0):
		hs = append(hs, hint{"↑ ↓", T("hint.select")}, hint{"enter", T("wiz.h.choose")})
	default:
		hs = append(hs, hint{"enter", T("wiz.h.next")})
		if z.step == wizExtras && z.bridgeLogin != "" {
			hs = append(hs, hint{"o", T("wiz.h.open")})
		}
	}
	if z.step != wizWelcome && z.step != wizDone && !m.wizTyping() {
		hs = append(hs, hint{"s", T("wiz.h.skip")})
	}
	return append(hs, hint{"esc", T("hint.back")})
}

// wrapText, metni kelime kelime w genişliğinde satırlara böler.
func wrapText(s string, w int) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(s) {
		switch {
		case line == "":
			line = word
		case textWidth(line)+1+textWidth(word) > w:
			lines = append(lines, line)
			line = word
		default:
			line += " " + word
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}
