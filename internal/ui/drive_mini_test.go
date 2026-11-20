package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Ehtisam/sotto/internal/ipc"
)

// Gerçek programda pencereyi küçült: mini görünüm, sonra büyüt: normal görünüm.
func TestDriveMiniView(t *testing.T) {
	d := drive(t)
	time.Sleep(2 * time.Second)
	name, _, _ := d.model().describe(d.model().state.Item)
	for _, sz := range [][2]int{{56, 14}, {40, 6}, {90, 9}, {34, 2}} {
		d.p.Send(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
		time.Sleep(600 * time.Millisecond)
		s := d.screen()
		t.Logf("%dx%d\n%s", sz[0], sz[1], s)
		if !d.model().miniMode() {
			t.Fatalf("%v: not mini", sz)
		}
		if name != "" && !strings.Contains(ansi.Strip(s), firstRunes(name, 6)) {
			t.Fatalf("%v: song %q not shown", sz, name)
		}
	}
	d.p.Send(tea.WindowSizeMsg{Width: 110, Height: 32})
	time.Sleep(600 * time.Millisecond)
	if d.model().miniMode() || !strings.Contains(d.screen(), "contents") && !strings.Contains(d.screen(), "içindekiler") {
		t.Fatalf("not back to normal:\n%s", d.screen())
	}
}

func firstRunes(s string, n int) string {
	r := []rune(s)
	return string(r[:min(n, len(r))])
}

// Telefon sesini uzaktan değiştirtmiyor: "sotto vol +5" Spotify'a gitmeden
// anlaşılır bir hata döner. (Yalnızca kumandadayken ve cihaz izin vermiyorsa.)
func TestDriveVolumeRefusedByPhone(t *testing.T) {
	d := drive(t)
	time.Sleep(2500 * time.Millisecond)
	m := d.model()
	if !m.remote.active || m.remote.player == nil || m.remote.player.Device.SupportsVolume == nil || *m.remote.player.Device.SupportsVolume {
		t.Skip("no remote device that refuses volume right now")
	}
	reply := make(chan ipc.Response, 1)
	d.p.Send(IPCMsg{Req: ipc.Request{Cmd: "vol", Args: []string{"+5"}}, Reply: reply})
	select {
	case r := <-reply:
		t.Logf("%+v", r)
		if r.OK || strings.Contains(r.Error, "403") || !strings.Contains(r.Error, m.remote.device) {
			t.Fatalf("got %+v", r)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no reply")
	}
}

// 26 satırlık pencerede de menünün altında (küçülmüş) kapak çıkar.
func TestDriveMenuCoverShortWindow(t *testing.T) {
	d := drive(t)
	d.p.Send(tea.WindowSizeMsg{Width: 110, Height: 26})
	d.do("8<wait:1500>")
	if d.model().state.Item == nil {
		t.Skip("nothing playing")
	}
	s := d.screen()
	t.Log("\n" + s)
	if !strings.Contains(s, "▀") {
		t.Fatal("no cover under the menu")
	}
}
