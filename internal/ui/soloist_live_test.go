package ui

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Ehtisam/sotto/internal/config"
	"github.com/Ehtisam/sotto/internal/soloist"
)

// SOTTO_LIVE=1 go test ./internal/ui -run LiveSoloistUpdate -v
// Kurulu Soloist'in kopyası üzerinde gerçek güncellemeyi dener (asıl kuruluma dokunmaz).
func TestLiveSoloistUpdate(t *testing.T) {
	if os.Getenv("SOTTO_LIVE") == "" {
		t.Skip("SOTTO_LIVE=1 ile çalışır")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(t.TempDir(), "soloist.exe")
	in, err := os.Open(cfg.SoloistPath())
	if err != nil {
		t.Skip("Soloist not installed")
	}
	out, _ := os.Create(exe)
	io.Copy(out, in)
	in.Close()
	out.Close()

	m := eventsModel(t)
	m.cfg.Soloist.Path = exe
	cur := mustBuild(exe)
	m.set.build = &cur
	t.Logf("installed: %s built %s", cur.Version, cur.Built)
	m, cmd := m.updateSoloist(false)
	u := cmd().(soloistUpdatedMsg)
	if u.err != nil {
		t.Fatal(u.err)
	}
	t.Logf("published: %s built %s, newer=%v", u.build.Version, u.build.Built, u.newer)
	if _, err := os.Stat(soloist.StagedPath(exe)); err == nil {
		t.Fatal("staged file left")
	}
	after := mustBuild(exe)
	if u.newer != after.Built.Equal(u.build.Built) || (!u.newer && !after.Built.Equal(cur.Built)) {
		t.Fatalf("after: %+v", after)
	}
	out2, _ := m.Update(u)
	t.Logf("note: %s", out2.(Model).flash)
}
