package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Ehtisam/sotto/internal/config"
)

// Sürücü: sotto'yu gerçek ayarlar ve gerçek hesapla, gerçek Bubble Tea
// programı olarak çalıştırır; tuşları terminalin göndereceği baytlar olarak
// yollar ve ekranı okur. Soloist başlatılmaz.
//
//	SOTTO_DRIVE=1 go test ./internal/ui -run Drive -v
//
// Bayt kısaltmaları: <enter> <esc> <bs> <up> <down> <left> <right> <tab>,
// <wait:500> (ms), <screen> (ekranı yazdır).

type driver struct {
	t    *testing.T
	in   *io.PipeWriter
	p    *tea.Program
	mu   sync.Mutex
	last Model
	done chan struct{}
	seq  int
	sync chan int
}

// syncMsg: filtre bunu gördüğünde önceki bütün mesajlar işlenmiştir.
type syncMsg struct{ n int }

func drive(t *testing.T) *driver {
	t.Helper()
	if os.Getenv("SOTTO_DRIVE") == "" {
		t.Skip("SOTTO_DRIVE not set")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Soloist.Autostart = false
	r, w := io.Pipe()
	d := &driver{t: t, in: w, done: make(chan struct{}), sync: make(chan int, 16)}
	d.p = tea.NewProgram(New(cfg),
		tea.WithInput(r), tea.WithOutput(io.Discard), tea.WithWindowSize(110, 32),
		tea.WithoutSignalHandler(),
		tea.WithFilter(func(m tea.Model, msg tea.Msg) tea.Msg {
			if mm, ok := m.(Model); ok {
				d.mu.Lock()
				d.last = mm
				d.mu.Unlock()
			}
			if s, ok := msg.(syncMsg); ok {
				d.sync <- s.n
				return nil
			}
			return msg
		}))
	go func() { d.p.Run(); close(d.done) }()
	t.Cleanup(func() {
		d.p.Kill()
		<-d.done
		d.last.Shutdown()
	})
	time.Sleep(2 * time.Second) // ilk yüklemeler
	return d
}

var driveKeys = map[string]string{
	"enter": "\r", "esc": "\x1b", "bs": "\x7f", "tab": "\t", "ctrl+s": "\x13", "ctrl+u": "\x15",
	// kitty klavye protokolü: sol shift'e bas / bırak
	"shift-down": "\x1b[57441;2u", "shift-up": "\x1b[57441;2:3u",
	"up": "\x1b[A", "down": "\x1b[B", "right": "\x1b[C", "left": "\x1b[D",
}

// do, betiği çalıştırır.
func (d *driver) do(script string) {
	d.t.Helper()
	for script != "" {
		i := strings.Index(script, "<")
		if i != 0 {
			chunk := script
			if i > 0 {
				chunk = script[:i]
			}
			for _, r := range chunk {
				d.send(string(r))
			}
			script = script[len(chunk):]
			continue
		}
		j := strings.Index(script, ">")
		tok := script[1:j]
		script = script[j+1:]
		switch {
		case tok == "screen":
			d.t.Log("\n" + d.screen())
		case strings.HasPrefix(tok, "wait:"):
			ms, _ := time.ParseDuration(strings.TrimPrefix(tok, "wait:") + "ms")
			time.Sleep(ms)
		default:
			seq, ok := driveKeys[tok]
			if !ok {
				d.t.Fatalf("unknown key <%s>", tok)
			}
			d.send(seq)
			if tok == "esc" {
				time.Sleep(250 * time.Millisecond) // yalnız ESC alt+tuş sanılmasın
			}
		}
	}
}

func (d *driver) send(b string) {
	_, _ = d.in.Write([]byte(b))
	time.Sleep(60 * time.Millisecond) // insan hızı; kaçış dizileri ayrı kalsın
}

func (d *driver) model() Model {
	d.seq++
	d.p.Send(syncMsg{d.seq})
	for n := range d.sync {
		if n == d.seq {
			break
		}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.last
}

func (d *driver) screen() string {
	m := d.model()
	var lines []string
	for _, l := range strings.Split(ansi.Strip(m.render()), "\n") {
		lines = append(lines, strings.TrimRight(l, " "))
	}
	return strings.Join(lines, "\n")
}

func TestDriveSearchEmpty(t *testing.T) {
	d := drive(t)
	d.do("3<wait:500>ab<wait:450><bs><bs><wait:3000><screen>")
	if s := d.screen(); strings.Contains(s, "loading") {
		t.Errorf("empty search box still says loading")
	}
}

func TestDriveNewPlaylistFromMenu(t *testing.T) {
	d := drive(t)
	d.do("<down><down><down><wait:1500>N<wait:500>")
	if !d.model().form.on {
		t.Errorf("N on the Library menu row did nothing; page %v focus %v", d.model().page, d.model().focus)
	}
}

func TestDriveNewPlaylistInOpenList(t *testing.T) {
	d := drive(t)
	d.do("4<wait:2500><enter><wait:1500>N<wait:500>")
	if !d.model().form.on {
		t.Errorf("N inside an open playlist did nothing")
	}
}

func TestDriveNewPlaylistKey(t *testing.T) {
	d := drive(t)
	d.do("4<wait:2500>N<wait:500><screen>")
	if !d.model().form.on {
		t.Errorf("N did not open the new playlist form; page %v focus %v", d.model().page, d.model().focus)
	}
}

func TestDriveSleepKey(t *testing.T) {
	d := drive(t)
	d.do("z<wait:300>")
	if !strings.Contains(d.screen(), "☾ 15:00") && !strings.Contains(d.screen(), "☾ 14:5") {
		t.Errorf("sleep mark missing\n%s", d.screen())
	}
	// Çalan bir şey varsa "şarkı bitince" de sırada; yoksa atlanır. Kapanana
	// kadar en çok bir tam tur bas.
	for i := 0; i < len(sleepSteps) && d.model().sleep.on(); i++ {
		d.do("z<wait:200>")
	}
	if d.model().sleep.on() || strings.Contains(d.screen(), "☾") {
		t.Errorf("sleep timer should be off after a full cycle")
	}
}

// N her sayfada, hem menüdeyken hem sayfanın içindeyken formu açmalı.
func TestDriveNewPlaylistEveryPage(t *testing.T) {
	d := drive(t)
	for p := 1; p <= 8; p++ {
		for _, inside := range []bool{false, true} {
			d.do("<esc><esc><wait:200>")
			d.model()
			keys := strings.Repeat("<up>", 8)
			if inside {
				keys = fmt.Sprint(p)
			} else {
				keys += strings.Repeat("<down>", p-1)
			}
			d.do(keys + "<wait:600>N<wait:400>")
			m := d.model()
			if p == 3 && inside {
				d.do("<esc><esc>") // Ara'da yazarken N yazıya gider
				continue
			}
			if !m.form.on {
				t.Errorf("page %d inside=%v: N did nothing (page %v focus %v flash %q)", p, inside, m.page, m.focus, m.flash)
			}
			d.do("<esc><wait:200>")
		}
	}
}

// Kitaplık'ta küçük n: yeni liste (kullanıcı böyle bastı); başka sayfada sonraki şarkı.
func TestDriveLowerNInLibrary(t *testing.T) {
	d := drive(t)
	for _, keys := range []string{"<up><up><up><up><up><up><up><up><down><down><down>", "4", "4<wait:2500><enter><wait:1500>"} {
		d.do("<esc><esc><wait:200>" + keys + "<wait:800>n<wait:400>")
		if m := d.model(); !m.form.on {
			t.Errorf("after %q: n did not open the form (page %v focus %v)", keys, m.page, m.focus)
		}
		d.do("<esc><wait:200>")
	}
	d.do("<esc><esc>1<wait:300>n<wait:300>")
	if d.model().form.on {
		t.Error("n on Now playing should not open the form")
	}
}

// Shift basılıyken alt satır shift tuşlarını gösterir, bırakınca eski haline döner.
func TestDriveShiftHint(t *testing.T) {
	d := drive(t)
	d.do("4<wait:2500><enter><wait:1500><shift-down><wait:300>")
	s := d.screen()
	t.Log("\n" + s)
	if !strings.Contains(s, "shift held") && !strings.Contains(s, "shift basılıyken") {
		t.Fatal("no shift hint while shift is held")
	}
	d.do("<shift-up><wait:300>")
	if s := d.screen(); strings.Contains(s, "shift held") || strings.Contains(s, "shift basılıyken") {
		t.Fatal("shift hint stayed after release")
	}
}
