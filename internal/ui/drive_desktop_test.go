package ui

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Ehtisam/sotto/internal/config"
	"github.com/Ehtisam/sotto/internal/desktop"
	"github.com/Ehtisam/sotto/internal/ipc"
)

// Komut satırı → açık sotto: gerçek program, gerçek soket, gerçek hesap.
// Yalnızca okuyan komutlar (çalan müziğe dokunmaz).
func TestDriveCLIToRunningSotto(t *testing.T) {
	d := drive(t)
	path := filepath.Join(t.TempDir(), "s.sock")
	srv, err := ipc.Listen(path, func(ctx context.Context, req ipc.Request) ipc.Response {
		reply := make(chan ipc.Response, 1)
		d.p.Send(IPCMsg{Req: req, Reply: reply})
		select {
		case r := <-reply:
			return r
		case <-ctx.Done():
			return ipc.Fail(ctx.Err())
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	time.Sleep(2 * time.Second) // kumanda yoklaması

	r, err := ipc.Call(path, ipc.Request{Cmd: "status"}, 10*time.Second)
	if err != nil || !r.OK {
		t.Fatalf("status: %+v %v", r, err)
	}
	t.Logf("status:\n%s", r.Text)
	name, _, _ := d.model().describe(d.model().state.Item)
	if name != "" && !strings.Contains(r.Text, name) {
		t.Fatalf("status %q lacks the shown song %q", r.Text, name)
	}
	r, err = ipc.Call(path, ipc.Request{Cmd: "connect"}, 20*time.Second)
	if err != nil || !r.OK {
		t.Fatalf("connect (list): %+v %v", r, err)
	}
	t.Logf("devices:\n%s", r.Text)
	r, _ = ipc.Call(path, ipc.Request{Cmd: "seek", Args: []string{"abc"}}, 10*time.Second)
	if r.OK {
		t.Fatal("bad seek accepted")
	}
}

// Ayarlar: yeni satırlar görünür, pencereye sığmayınca kayar, bildirim ayarı
// değişip geri döner (ayar dosyası eski haline gelir).
func TestDriveSettingsDesktopRows(t *testing.T) {
	d := drive(t)
	before, _ := config.Load()
	d.do("8<wait:600>")
	s := d.screen()
	for _, want := range []string{"Soloist build", "days left", "Continue where you left off"} {
		if !strings.Contains(s, want) && !strings.Contains(s, map[string]string{"Soloist build": "Soloist sürümü", "days left": "gün kaldı", "Continue where you left off": "Kaldığın yerden devam"}[want]) {
			t.Fatalf("settings lack %q:\n%s", want, s)
		}
	}
	// Bildirim satırına in, değiştir, geri al.
	for i := 0; i < 15 && d.model().settingSelected() != setNotify; i++ {
		d.do("<down><wait:80>")
	}
	if d.model().settingSelected() != setNotify {
		t.Fatal("notify row not reachable")
	}
	s = d.screen()
	if !strings.Contains(s, "notifications") && !strings.Contains(s, "bildirimleri") {
		t.Fatalf("selected row not on screen:\n%s", s)
	}
	d.do("<right><wait:300>")
	mid, _ := config.Load()
	d.do("<left><wait:300>")
	after, _ := config.Load()
	if mid.Desktop.Notify == before.Desktop.Notify || after.Desktop.Notify != before.Desktop.Notify {
		t.Fatalf("notify: %q → %q → %q", before.Desktop.Notify, mid.Desktop.Notify, after.Desktop.Notify)
	}
	// En alta in: son satır (ayar dosyası) görünmeli.
	d.do("G<wait:300>")
	if s = d.screen(); !strings.Contains(s, "Settings file") && !strings.Contains(s, "Ayar dosyası") {
		t.Fatalf("last row hidden:\n%s", s)
	}
	d.t.Log("\n" + s)
}

// Medya kartı: gerçek programda açılır ve çalan şarkıyı gösterir.
func TestDriveMediaCard(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows only")
	}
	d := drive(t)
	time.Sleep(3 * time.Second)
	m := d.model()
	if m.rt.ev.media == nil {
		t.Fatal("media card not opened")
	}
	name, _, _ := m.describe(m.state.Item)
	ps := `$null=[Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager,Windows.Media.Control,ContentType=WindowsRuntime]
Add-Type -AssemblyName System.Runtime.WindowsRuntime
$as=([System.WindowsRuntimeSystemExtensions].GetMethods()|?{$_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation` + "`" + `1'})[0]
function Await($op,$t){ $task=$as.MakeGenericMethod($t).Invoke($null,@($op)); $task.Wait(-1)|Out-Null; $task.Result }
$mgr=Await ([Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager]::RequestAsync()) ([Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager])
foreach($s in $mgr.GetSessions()){ if($s.SourceAppUserModelId -eq '` + desktop.AppID + `'){ $p=Await ($s.TryGetMediaPropertiesAsync()) ([Windows.Media.Control.GlobalSystemMediaTransportControlsSessionMediaProperties]); "$($p.Title)|$($p.Artist)|$($s.GetPlaybackInfo().PlaybackStatus)|$($s.GetTimelineProperties().EndTime)" } }`
	out, err := exec.Command("powershell", "-NoProfile", "-Command", ps).CombinedOutput()
	t.Logf("shown: %q, card: %s", name, out)
	if err != nil {
		t.Fatal(err)
	}
	if name != "" && !strings.Contains(string(out), name) {
		t.Fatalf("card does not show %q", name)
	}
}
