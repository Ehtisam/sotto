//go:build windows

package desktop

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// SOTTO_LIVE=1 SOTTO_EXE=…\sotto.exe go test ./internal/desktop -run LiveShortcut -v
// Başlat menüsüne gerçek kısayolu yazar ve Windows'un sotto'yu adıyla tanıdığını denetler.
func TestLiveShortcut(t *testing.T) {
	exe := os.Getenv("SOTTO_EXE")
	if os.Getenv("SOTTO_LIVE") == "" || exe == "" {
		t.Skip("SOTTO_LIVE=1 SOTTO_EXE=<sotto.exe> ile çalışır")
	}
	local, _ := os.UserCacheDir()
	SetDataDir(filepath.Join(local, "sotto"))
	if err := InstallShortcut(exe); err != nil {
		t.Fatal(err)
	}
	target, id, err := ShortcutInfo()
	t.Logf("target %s id %s err %v", target, id, err)
	if !ShortcutOK(exe) {
		t.Fatal("shortcut not as written")
	}
	// Windows Başlat menüsünü birkaç saniye içinde yeniden tarar.
	var out []byte
	for try := 0; try < 10; try++ {
		out, err = exec.Command("powershell", "-NoProfile", "-Command",
			`Get-StartApps | Where-Object AppID -eq '`+AppID+`' | ForEach-Object { $_.Name + '|' + $_.AppID }`).CombinedOutput()
		if strings.Contains(string(out), "sotto|"+AppID) {
			break
		}
		time.Sleep(time.Second)
	}
	t.Logf("start apps: %s", out)
	if err != nil || !strings.Contains(string(out), "sotto|"+AppID) {
		t.Fatal("Windows does not know sotto by name")
	}
}
