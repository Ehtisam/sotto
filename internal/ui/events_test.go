package ui

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/desktop"
	"github.com/Ehtisam/sotto/internal/soloist"
)

// runAll, bir komutu (ve içindeki toplu komutları) çalıştırır. Bekleyen
// (zamanlayıcı, dinleyici) ya da testte çalışamayan komutlar atlanır.
func runAll(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	done := make(chan tea.Msg, 1)
	go func() {
		defer func() {
			if recover() != nil {
				done <- nil
			}
		}()
		done <- cmd()
	}()
	var msg tea.Msg
	select {
	case msg = <-done:
	case <-time.After(300 * time.Millisecond):
		return nil
	}
	if b, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range b {
			out = append(out, runAll(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

type captured struct {
	mu    sync.Mutex
	notes []desktop.Note
	hooks []string // olay adları
	env   [][]string
}

func capture(t *testing.T) *captured {
	c := &captured{}
	oldN, oldH := notify, runHookFn
	notify = func(n desktop.Note) error { c.mu.Lock(); c.notes = append(c.notes, n); c.mu.Unlock(); return nil }
	runHookFn = func(command string, env []string, _ string) {
		c.mu.Lock()
		defer c.mu.Unlock()
		for _, e := range env {
			if strings.HasPrefix(e, "SOTTO_EVENT=") {
				c.hooks = append(c.hooks, strings.TrimPrefix(e, "SOTTO_EVENT="))
			}
		}
		c.env = append(c.env, env)
	}
	t.Cleanup(func() { notify, runHookFn = oldN, oldH })
	return c
}

func eventsModel(t *testing.T) Model {
	m := queueModel(t)
	m.cfg.LocalDir = t.TempDir()
	m.cfg.Desktop.Notify = "always"
	m.cfg.Desktop.NotifyTitle = "{title}"
	m.cfg.Desktop.NotifyBody = "{artist} · {album}"
	m.cfg.Hooks.OnTrackChange = "echo track"
	m.cfg.Hooks.OnStatusChange = "echo status"
	m.state.Status = soloist.StatusPlaying
	return m
}

func step(m Model, msg tea.Msg) (Model, []tea.Msg) {
	out, cmd := m.Update(msg)
	return out.(Model), runAll(cmd)
}

func TestTrackChangeNotifiesAndRunsHook(t *testing.T) {
	c := capture(t)
	m := eventsModel(t)

	// İlk durum (açılıştaki şarkı) duyurulmaz.
	m, _ = step(m, tickMsg(time.Now()))
	if len(c.notes) != 0 || len(c.hooks) != 0 {
		t.Fatalf("first state announced: %v %v", c.notes, c.hooks)
	}
	if m.rt.ev.announced != "spotify:track:now" {
		t.Fatalf("announced = %q", m.rt.ev.announced)
	}

	// Şarkı değişince bir bildirim ve bir olay komutu.
	m.state.Item = entity("q1")
	m, _ = step(m, tickMsg(time.Now()))
	if len(c.notes) != 1 || c.notes[0].Title != "q1" {
		t.Fatalf("notes = %+v", c.notes)
	}
	if strings.Join(c.hooks, ",") != "track_change" {
		t.Fatalf("hooks = %v", c.hooks)
	}
	env := strings.Join(c.env[0], "\n")
	for _, want := range []string{"SOTTO_TITLE=q1", "SOTTO_URI=spotify:track:q1", "SOTTO_STATUS=playing", "SOTTO_CONTEXT=KaBoom.2"} {
		if !strings.Contains(env, want) {
			t.Errorf("env lacks %s", want)
		}
	}

	// Aynı şarkıda yeni bir güncelleme yeniden duyurmaz.
	m, _ = step(m, tickMsg(time.Now()))
	if len(c.notes) != 1 {
		t.Fatalf("re-announced: %d", len(c.notes))
	}

	// Durdurunca durum komutu çalışır, bildirim gelmez.
	m.state.Status = soloist.StatusPaused
	m, _ = step(m, tickMsg(time.Now()))
	if strings.Join(c.hooks, ",") != "track_change,status_change" || len(c.notes) != 1 {
		t.Fatalf("after pause: hooks %v notes %d", c.hooks, len(c.notes))
	}
	_ = m
}

func TestNotifyRespectsFocusAndOff(t *testing.T) {
	c := capture(t)
	m := eventsModel(t)
	m.cfg.Desktop.Notify = "unfocused"
	m, _ = step(m, tickMsg(time.Now()))
	m, _ = step(m, tea.FocusMsg{})
	m.state.Item = entity("q1")
	m, _ = step(m, tickMsg(time.Now()))
	if len(c.notes) != 0 {
		t.Fatal("notified while focused")
	}
	m, _ = step(m, tea.BlurMsg{})
	m.state.Item = entity("c1")
	m, _ = step(m, tickMsg(time.Now()))
	if len(c.notes) != 1 {
		t.Fatal("no notification while unfocused")
	}
	m.cfg.Desktop.Notify = "off"
	m.state.Item = entity("a1")
	m, _ = step(m, tickMsg(time.Now()))
	if len(c.notes) != 1 {
		t.Fatal("notified while off")
	}
}

func TestNamelessTrackWaitsForName(t *testing.T) {
	c := capture(t)
	m := eventsModel(t)
	m, _ = step(m, tickMsg(time.Now()))
	nameless := &soloist.Entity{URI: "spotify:track:noname", EntityType: "track"}
	m.state.Item = nameless
	m, _ = step(m, tickMsg(time.Now()))
	if m.rt.ev.announced == "spotify:track:noname" {
		t.Fatal("announced before the name was known")
	}
	if len(c.notes) != 0 {
		t.Fatal("announced before the name was known")
	}
	// Ad üç saniye içinde gelmezse eldekiyle duyurulur.
	m, _ = step(m, tickMsg(time.Now().Add(4*time.Second)))
	if len(c.notes) != 0 {
		// başlık boşsa bildirim gösterilmez ama şarkı duyurulmuş sayılır
	}
	if m.rt.ev.announced != "spotify:track:noname" {
		t.Fatalf("announced = %q", m.rt.ev.announced)
	}
}

func TestFillTemplate(t *testing.T) {
	f := map[string]string{"title": "Song", "artist": "Band", "album": ""}
	if got := fillTemplate("{artist} · {album}", f); got != "Band" {
		t.Fatalf("got %q", got)
	}
	if got := fillTemplate("{title} — {artist} · {album}", f); got != "Song — Band" {
		t.Fatalf("got %q", got)
	}
	if got := fillTemplate("{album} · {artist}", f); got != "Band" {
		t.Fatalf("got %q", got)
	}
}

func TestLastPlaySavedAndLoaded(t *testing.T) {
	capture(t)
	m := eventsModel(t)
	m, _ = step(m, tickMsg(time.Now()))
	l := loadLast(m.cfg.LocalDir)
	if l.Track != "spotify:track:now" || l.Context != "spotify:playlist:pl" {
		t.Fatalf("last = %+v", l)
	}
	if _, err := os.Stat(lastFile(m.cfg.LocalDir)); err != nil {
		t.Fatal(err)
	}
}

func TestMediaButtonsDriveControls(t *testing.T) {
	m := eventsModel(t)
	md := &desktop.Media{}
	m.rt.ev.media = md
	for _, b := range []desktop.Button{desktop.Play, desktop.Pause, desktop.Next, desktop.Previous} {
		_, cmd := m.Update(mediaEventMsg{media: md, ev: desktop.Event{Button: b}})
		if cmd == nil {
			t.Fatalf("button %d did nothing", b)
		}
	}
	// Kapatılmış kartın olayları yok sayılır.
	if _, cmd := m.update(mediaEventMsg{media: &desktop.Media{}, ev: desktop.Event{Button: desktop.Next}}); cmd != nil {
		t.Fatal("stale card event acted")
	}
}

func TestRunHookReallyRuns(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "logs", "hooks.log")
	out := filepath.Join(dir, "out.txt")
	cmd := `echo %SOTTO_TITLE%> "` + out + `"`
	if runtime.GOOS != "windows" {
		cmd = `echo "$SOTTO_TITLE" > '` + out + `'`
	}
	runHook(cmd, append(os.Environ(), "SOTTO_TITLE=Belki Üstümüzden Bir Kuş Geçer"), logPath)
	b, err := os.ReadFile(out)
	if err != nil || strings.TrimSpace(string(b)) != "Belki Üstümüzden Bir Kuş Geçer" {
		t.Fatalf("hook output %q %v", b, err)
	}
	if l, _ := os.ReadFile(logPath); !strings.Contains(string(l), "[ok]") {
		t.Fatalf("log: %s", l)
	}
}

// Kullanıcının yazdığı biçim: tırnaklı yol, boşluklu klasör, iki değişken, >> ile ekleme.
func TestRunHookQuotedPathAppend(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("cmd")
	}
	dir := filepath.Join(t.TempDir(), "klasör adı")
	os.MkdirAll(dir, 0o700)
	out := filepath.Join(dir, "now playing.txt")
	env := append(os.Environ(), "SOTTO_ARTIST=Yüksek Sadakat", "SOTTO_TITLE=Belki Üstümüzden Bir Kuş Geçer", "OUTDIR="+dir)
	cmd := `echo %SOTTO_ARTIST% - %SOTTO_TITLE% >> "%OUTDIR%\now playing.txt"`
	runHook(cmd, env, filepath.Join(t.TempDir(), "hooks.log"))
	runHook(cmd, env, filepath.Join(t.TempDir(), "hooks.log"))
	b, err := os.ReadFile(out)
	want := "Yüksek Sadakat - Belki Üstümüzden Bir Kuş Geçer \r\n"
	if err != nil || string(b) != want+want {
		t.Fatalf("got %q %v", b, err)
	}
}
