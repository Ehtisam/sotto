package ui

import (
	"errors"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Ehtisam/sotto/internal/config"
	"github.com/Ehtisam/sotto/internal/soloist"
)

// wizModel, ilk kez açılan bir sotto: ayar dosyası boş, tarayıcı açılmaz.
func wizModel(t *testing.T) (Model, *[]string) {
	t.Helper()
	opened := &[]string{}
	old := openBrowser
	openBrowser = func(u string) error { *opened = append(*opened, u); return nil }
	t.Cleanup(func() { openBrowser = old })
	dir := t.TempDir()
	m := New(config.Config{ConfigDir: dir, LocalDir: dir, Soloist: config.Soloist{DeviceName: "sotto", Autostart: true}, Extras: config.Extras{Enabled: true}})
	m.width, m.height = 110, 34
	return m, opened
}

func wkey(t *testing.T, m Model, k string) (Model, tea.Cmd) {
	t.Helper()
	var msg tea.KeyPressMsg
	switch k {
	case "enter":
		msg = tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		msg = tea.KeyPressMsg{Code: tea.KeyEscape}
	case "down":
		msg = tea.KeyPressMsg{Code: tea.KeyDown}
	case "ctrl+o":
		msg = tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl}
	default:
		msg = tea.KeyPressMsg{Text: k, Code: rune(k[0])}
	}
	out, cmd := m.Update(msg)
	return out.(Model), cmd
}

func screenText(m Model) string { return ansi.Strip(m.render()) }

func configText(t *testing.T, m Model) string {
	t.Helper()
	b, err := os.ReadFile(m.cfg.ConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestWizardOpensOnFirstRunOnly(t *testing.T) {
	m, _ := wizModel(t)
	if !m.wiz.on {
		t.Fatal("no Client ID: the wizard should open")
	}
	dir := t.TempDir()
	m2 := New(config.Config{ConfigDir: dir, LocalDir: dir, Spotify: config.Spotify{ClientID: "x"}})
	if m2.wiz.on {
		t.Fatal("already set up: no wizard")
	}
	if !m2.StartSetup().wiz.on {
		t.Fatal("sotto setup should open it")
	}
}

func TestWizardSpotifyAppStep(t *testing.T) {
	m, opened := wizModel(t)
	m, _ = wkey(t, m, "enter") // hoş geldin → uygulama
	if m.wiz.step != wizApp || !strings.Contains(screenText(m), "Redirect URI") {
		t.Fatalf("step %d", m.wiz.step)
	}
	m, _ = wkey(t, m, "ctrl+o")
	if len(*opened) != 1 || (*opened)[0] != dashboardURL {
		t.Fatalf("opened %v", *opened)
	}
	// Yanlış: kaydedilmez.
	out, _ := m.Update(tea.PasteMsg{Content: "not-an-id"})
	m = out.(Model)
	m, cmd := wkey(t, m, "enter")
	if cmd != nil || m.wiz.err == "" {
		t.Fatal("a bad Client ID should be refused")
	}
	// Doğru (boşluk ve satır sonuyla yapıştırılmış): kaydedilir, giriş başlar.
	m.wiz.input = ""
	out, _ = m.Update(tea.PasteMsg{Content: "  0123456789ABCDEF0123456789abcdef\r\n"})
	m = out.(Model)
	m, cmd = wkey(t, m, "enter")
	if cmd == nil || m.spState != spLoggingIn {
		t.Fatalf("login should start, state %d err %q", m.spState, m.wiz.err)
	}
	if !strings.Contains(configText(t, m), `client_id = "0123456789abcdef0123456789abcdef"`) {
		t.Fatal("Client ID not saved")
	}
	// Giriş bitti: ✓ ve devam.
	out, _ = m.Update(loginDoneMsg{})
	m = out.(Model)
	if s := screenText(m); !strings.Contains(s, "✓") || !strings.Contains(s, "Press enter to continue") {
		t.Fatal("connected step should show a tick and say how to go on")
	}
	m, _ = wkey(t, m, "enter")
	if m.wiz.step != wizSpeaker {
		t.Fatalf("step %d, want speaker", m.wiz.step)
	}
}

func TestWizardRemoteOnlyAndNoExtras(t *testing.T) {
	m, _ := wizModel(t)
	m.wiz.step, m.spState = wizSpeaker, spReady
	if !strings.Contains(screenText(m), "press t any time") {
		t.Fatal("the speaker step should say the choice can be switched with t")
	}
	m, _ = wkey(t, m, "down")
	m, _ = wkey(t, m, "enter") // yalnızca kumanda
	if m.wiz.step != wizExtras || m.cfg.Soloist.Autostart {
		t.Fatalf("step %d autostart %v", m.wiz.step, m.cfg.Soloist.Autostart)
	}
	if !strings.Contains(configText(t, m), "autostart = false") {
		t.Fatal("autostart not saved")
	}
	m, _ = wkey(t, m, "down")
	m, _ = wkey(t, m, "enter") // ekstralar kapalı
	if m.wiz.step != wizDone || m.cfg.Extras.Enabled {
		t.Fatalf("step %d extras %v", m.wiz.step, m.cfg.Extras.Enabled)
	}
	m, _ = wkey(t, m, "enter")
	if m.wiz.on {
		t.Fatal("enter on the last step should close the wizard")
	}
}

func TestWizardLightModeKeyDownloadAndPairing(t *testing.T) {
	m, opened := wizModel(t)
	m.wiz.step, m.spState = wizSpeaker, spReady
	m, _ = wkey(t, m, "enter") // hafif mod
	if m.wiz.speaker != 1 || !m.wizTyping() {
		t.Fatal("light mode should ask for the Soloist key")
	}
	m, _ = wkey(t, m, "ctrl+o")
	if (*opened)[len(*opened)-1] != soloistKeyURL {
		t.Fatalf("opened %v", *opened)
	}
	out, _ := m.Update(tea.PasteMsg{Content: "sk_live_0123456789abcdefSECRET"})
	m = out.(Model)
	if strings.Contains(screenText(m), "SECRET") {
		t.Fatal("the key should not be shown in full")
	}
	m, _ = wkey(t, m, "enter")
	if m.cfg.Soloist.APIKey != "sk_live_0123456789abcdefSECRET" {
		t.Fatalf("key = %q, err %q", m.cfg.Soloist.APIKey, m.wiz.err)
	}
	// Soloist kurulu değil: enter indirmeyi başlatır (burada çalıştırılmaz).
	if !strings.Contains(screenText(m), "25 MB") {
		t.Fatal("should offer the download")
	}
	m, cmd := wkey(t, m, "enter")
	if cmd == nil || m.wiz.dl == nil || m.wiz.dlWhat != "soloist" {
		t.Fatal("enter should start the download")
	}
	// İndirme başarısız: hata görünür, yeniden denenebilir.
	out, _ = m.Update(wizDownloadedMsg{what: "soloist", err: errors.New("offline")})
	m = out.(Model)
	if m.wiz.dl != nil || !strings.Contains(screenText(m), "offline") {
		t.Fatal("download error should show")
	}
	// İndi: Soloist başlatılır; eşleşince ✓.
	if err := os.MkdirAll(m.cfg.SoloistDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(m.cfg.SoloistPath(), []byte("x"), 0o700)
	out, cmd = m.Update(wizDownloadedMsg{what: "soloist"})
	m = out.(Model)
	if cmd == nil || m.engine != engineStarting {
		t.Fatal("Soloist should start after the download")
	}
	m.engine, m.haveAuth = engineReady, true
	m.rt.client = &soloist.Client{}
	m.auth = soloist.AuthState{LoggedIn: true}
	if !strings.Contains(screenText(m), "paired") {
		t.Fatalf("pairing tick missing:\n%s", screenText(m))
	}
	m, _ = wkey(t, m, "enter")
	if m.wiz.step != wizExtras {
		t.Fatalf("step %d", m.wiz.step)
	}
}

func TestWizardExtrasLogin(t *testing.T) {
	m, opened := wizModel(t)
	m.wiz.step = wizExtras
	m, cmd := wkey(t, m, "enter") // ekstraları aç: go-librespot yok → indir
	if cmd == nil || m.wiz.dlWhat != "librespot" || !m.cfg.Extras.Enabled {
		t.Fatal("extras on should download go-librespot")
	}
	m.wiz.dl = nil
	// Giriş gerekiyor: sayfa bir kez kendiliğinden açılır, sonra yeniden denenir.
	out, cmd := m.Update(wizBridgeMsg{login: "https://accounts.example.test/login"})
	m = out.(Model)
	if cmd == nil || len(*opened) != 1 || !strings.Contains(screenText(m), "sign") {
		t.Fatalf("login page: opened %v", *opened)
	}
	out, _ = m.Update(wizBridgeMsg{login: "https://accounts.example.test/login"})
	m = out.(Model)
	if len(*opened) != 1 {
		t.Fatal("the login page should open only once by itself")
	}
	out, _ = m.Update(wizBridgeMsg{ok: true})
	m = out.(Model)
	if !m.wiz.bridgeOK || !strings.Contains(screenText(m), "✓") || !strings.Contains(screenText(m), "Press enter to continue") {
		t.Fatal("extras should be ticked")
	}
	m, _ = wkey(t, m, "enter")
	if m.wiz.step != wizDone {
		t.Fatalf("step %d", m.wiz.step)
	}
}

func TestWizardEscGoesBackAndCloses(t *testing.T) {
	m, _ := wizModel(t)
	m, _ = wkey(t, m, "enter")
	m, _ = wkey(t, m, "esc")
	if m.wiz.step != wizWelcome || !m.wiz.on {
		t.Fatal("esc should go back a step")
	}
	m, _ = wkey(t, m, "esc")
	if m.wiz.on {
		t.Fatal("esc on the first step closes the wizard")
	}
}
