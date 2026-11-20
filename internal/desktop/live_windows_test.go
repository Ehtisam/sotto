//go:build windows

package desktop

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// SOTTO_LIVE=1 go test ./internal/desktop -run Live -v
// Gerçek Windows'ta bir bildirim gösterir ve medya kartını açar.
func TestLiveDesktop(t *testing.T) {
	if os.Getenv("SOTTO_LIVE") == "" {
		t.Skip("SOTTO_LIVE=1 ile çalışır")
	}
	SetDataDir(t.TempDir())
	if err := Notify(Note{Title: "sotto test", Body: "Notification check"}); err != nil {
		t.Fatalf("notify: %v", err)
	}
	m, err := OpenMedia()
	if err != nil {
		t.Fatalf("media: %v", err)
	}
	if err := m.SetTrack(Track{Title: "Test Song", Artist: "Test Artist", Album: "Test Album",
		CoverURL: "https://i.scdn.co/image/ab67616d0000b273e8b066f70c206551210d902b", Duration: 3 * time.Minute}); err != nil {
		t.Fatalf("track: %v", err)
	}
	if err := m.SetStatus(Playing); err != nil {
		t.Fatalf("status: %v", err)
	}
	if err := m.SetPosition(30*time.Second, 3*time.Minute); err != nil {
		t.Fatalf("position: %v", err)
	}
	time.Sleep(1500 * time.Millisecond)
	ps := `$null=[Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager,Windows.Media.Control,ContentType=WindowsRuntime]
Add-Type -AssemblyName System.Runtime.WindowsRuntime
$as=([System.WindowsRuntimeSystemExtensions].GetMethods()|?{$_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation` + "`" + `1'})[0]
function Await($op,$t){ $task=$as.MakeGenericMethod($t).Invoke($null,@($op)); $task.Wait(-1)|Out-Null; $task.Result }
$mgr=Await ([Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager]::RequestAsync()) ([Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager])
foreach($s in $mgr.GetSessions()){ $p=Await ($s.TryGetMediaPropertiesAsync()) ([Windows.Media.Control.GlobalSystemMediaTransportControlsSessionMediaProperties]); "$($s.SourceAppUserModelId)|$($p.Title)|$($p.Artist)|$($s.GetPlaybackInfo().PlaybackStatus)|thumb=$($p.Thumbnail -ne $null)" }`
	out, err := exec.Command("powershell", "-NoProfile", "-Command", ps).CombinedOutput()
	t.Logf("sessions:\n%s", out)
	if err != nil || !strings.Contains(string(out), "Test Song") {
		t.Fatalf("our session not visible (%v)", err)
	}
	m.Close()
}
