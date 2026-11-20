//go:build windows

package desktop

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Panoyu değiştirmeden okur ve PowerShell'in okuduğuyla karşılaştırır.
// SOTTO_LIVE=1 go test ./internal/desktop -run LiveClipboard -v
func TestLiveClipboard(t *testing.T) {
	if os.Getenv("SOTTO_LIVE") == "" {
		t.Skip("SOTTO_LIVE=1")
	}
	got, err := ReadClipboard()
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("powershell", "-NoProfile", "-Command", "[Console]::OutputEncoding=[Text.Encoding]::UTF8; Get-Clipboard -Raw").Output()
	if err != nil {
		t.Fatal(err)
	}
	want := strings.TrimRight(string(out), "\r\n")
	if strings.TrimRight(got, "\r\n") != want {
		t.Fatalf("got %d chars, PowerShell %d chars", len(got), len(want))
	}
	t.Logf("read %d chars from the clipboard", len(got))
}
