package soloist

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	in, err := os.Open(from)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	out.Close()
}

// Çalışan programın yerine yenisi konabilmeli (Windows'ta asıl mesele bu).
func TestSwapWhileRunning(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "prog.exe")
	src, args := `C:\Windows\System32\ping.exe`, []string{"-n", "6", "127.0.0.1"}
	if runtime.GOOS != "windows" {
		src, args = "/bin/sleep", []string{"5"}
	}
	copyFile(t, src, exe)
	cmd := exec.Command(exe, args...)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	time.Sleep(300 * time.Millisecond)

	if err := os.WriteFile(StagedPath(exe), []byte("new build"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Swap(exe); err != nil {
		t.Fatalf("swap while running: %v", err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "new build" {
		t.Fatalf("exe = %q", b)
	}
	if _, err := os.Stat(StagedPath(exe)); err == nil {
		t.Fatal("staged file left")
	}
	cmd.Process.Kill()
	cmd.Wait()
	CleanOld(exe)
	if _, err := os.Stat(exe + ".old"); err == nil {
		t.Fatal("old file left after it stopped")
	}
}

func TestCheckedSince(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "soloist.exe")
	if !CheckedSince(exe).IsZero() {
		t.Fatal("never checked")
	}
	now := time.Now().Truncate(time.Second)
	MarkChecked(exe, now)
	if !CheckedSince(exe).Equal(now) {
		t.Fatalf("%v", CheckedSince(exe))
	}
}
