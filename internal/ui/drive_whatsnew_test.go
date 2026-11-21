package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Yenilikler penceresi gerçek programda: açılışta görünür, j kaydırır ve
// pencere açık kalır, başka bir tuş kapatır ve o tuş başka iş yapmaz.
func TestDriveWhatsNew(t *testing.T) {
	oldV, oldEN := appVersion, whatsNewEN
	defer func() { appVersion, whatsNewEN = oldV, oldEN }()
	appVersion = func() string { return "0.2.0" }
	whatsNewEN = "## 0.2.0\n- Clear the queue with shift+C\n\n## 0.1.0\n- First\n"
	driveBeforeNew = func(dir string) {
		_ = os.WriteFile(filepath.Join(dir, "seen-version"), []byte("0.1.0\n"), 0o600)
	}
	defer func() { driveBeforeNew = nil }()

	d, _ := driveOffline(t)
	s := d.screen()
	for _, want := range []string{"What's new in sotto 0.2.0", "Clear the queue with shift+C", "Any key closes"} {
		if !strings.Contains(s, want) {
			t.Fatalf("window lacks %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "- First") {
		t.Fatal("notes of the version already seen are shown")
	}
	d.do("j<wait:100>")
	if !d.model().whatsnew.open {
		t.Fatal("j should scroll, not close")
	}
	d.do("4<wait:200>")
	if m := d.model(); m.whatsnew.open || m.page == pageQueue {
		t.Fatalf("open %v, page %v (the closing key must not act)", m.whatsnew.open, m.page)
	}
}
