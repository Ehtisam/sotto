package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Ehtisam/sotto/internal/setup"
	"github.com/Ehtisam/sotto/internal/soloist"
)

// fakeSoloist: indirme sahte dosya yazar; sürüm dosyanın içeriğinden okunur
// ("built:<unix>").
func fakeSoloist(t *testing.T, published time.Time) (exe string, downloads *int) {
	t.Helper()
	dir := t.TempDir()
	exe = filepath.Join(dir, "soloist.exe")
	n := 0
	oldD, oldR := downloadSoloist, readBuild
	downloadSoloist = func(_ context.Context, _, _, dest string, _ *setup.Progress) error {
		n++
		return os.WriteFile(dest, []byte("built:"+published.Format(time.RFC3339)), 0o755)
	}
	readBuild = func(_ context.Context, path string) (soloist.Build, error) {
		b, err := os.ReadFile(path)
		if err != nil {
			return soloist.Build{}, err
		}
		at, _ := time.Parse(time.RFC3339, strings.TrimPrefix(string(b), "built:"))
		return soloist.Build{Version: "1.3.10.87", Built: at}, nil
	}
	t.Cleanup(func() { downloadSoloist, readBuild = oldD, oldR })
	return exe, &n
}

func installedModel(t *testing.T, exe string, built time.Time, auto bool) Model {
	m := eventsModel(t)
	m.cfg.Soloist.Path = exe
	m.cfg.Soloist.AutoUpdate = auto
	if err := os.WriteFile(exe, []byte("built:"+built.Format(time.RFC3339)), 0o755); err != nil {
		t.Fatal(err)
	}
	return m
}

// buildArrives, açılıştaki sürüm okumasını ve ardından gelen komutları işler.
func buildArrives(m Model) (Model, []string) {
	out, cmd := m.Update(soloistBuildMsg{build: mustBuild(m.cfg.SoloistPath())})
	m = out.(Model)
	var notes []string
	if m.flash != "" {
		notes = append(notes, m.flash)
	}
	for _, msg := range runAll(cmd) {
		if u, ok := msg.(soloistUpdatedMsg); ok {
			out, _ = m.Update(u)
			m = out.(Model)
			notes = append(notes, m.flash)
		}
	}
	return m, notes
}

func mustBuild(path string) soloist.Build {
	b, _ := readBuild(context.Background(), path)
	return b
}

func TestSoloistAutoUpdateSwapsNewerBuild(t *testing.T) {
	now := time.Now()
	exe, n := fakeSoloist(t, now.Add(-24*time.Hour)) // Spotify dün yeni sürüm yayımladı
	m := installedModel(t, exe, now.Add(-80*24*time.Hour), true)
	m, notes := buildArrives(m)
	if *n != 1 {
		t.Fatalf("downloads = %d", *n)
	}
	if b := mustBuild(exe); b.DaysLeft(now) < 85 {
		t.Fatalf("not swapped: %d days left", b.DaysLeft(now))
	}
	if m.set.build == nil || m.set.build.DaysLeft(now) < 85 || !strings.Contains(strings.Join(notes, "|"), "up to date") {
		t.Fatalf("notes %v build %+v", notes, m.set.build)
	}
	// Aynı gün yeniden açılınca yeniden indirilmez (süre artık uzun zaten).
	if _, notes = buildArrives(m); *n != 1 {
		t.Fatalf("downloaded again: %v", notes)
	}
}

func TestSoloistAutoUpdateNoNewerBuild(t *testing.T) {
	now := time.Now()
	old := now.Add(-80 * 24 * time.Hour)
	exe, n := fakeSoloist(t, old) // Spotify henüz yenisini yayımlamadı
	m := installedModel(t, exe, old, true)
	m, _ = buildArrives(m)
	if *n != 1 || mustBuild(exe).Built.Unix() != old.Unix() {
		t.Fatal("old build replaced")
	}
	if _, err := os.Stat(soloist.StagedPath(exe)); err == nil {
		t.Fatal("staged file left")
	}
	// Bugün bakıldı: bir sonraki açılışta indirme yok.
	m.set.updating = nil
	if _, _ = buildArrives(m); *n != 1 {
		t.Fatalf("downloads = %d", *n)
	}
}

func TestSoloistUpdateWhileRunningKeepsMusic(t *testing.T) {
	now := time.Now()
	exe, _ := fakeSoloist(t, now)
	m := installedModel(t, exe, now.Add(-85*24*time.Hour), true)
	m.rt.proc = &soloist.Process{}
	m, notes := buildArrives(m)
	if m.rt.proc == nil || m.engine == engineStarting {
		t.Fatal("running Soloist was touched")
	}
	if !strings.Contains(strings.Join(notes, "|"), "next time Soloist starts") {
		t.Fatalf("notes = %v", notes)
	}
}

func TestSoloistAutoUpdateOffOnlyWarns(t *testing.T) {
	now := time.Now()
	exe, n := fakeSoloist(t, now)
	m := installedModel(t, exe, now.Add(-80*24*time.Hour), false)
	_, notes := buildArrives(m)
	if *n != 0 || !strings.Contains(strings.Join(notes, "|"), "stops working in") {
		t.Fatalf("downloads %d notes %v", *n, notes)
	}
}

func TestSoloistExpiredUpdatesAndStarts(t *testing.T) {
	now := time.Now()
	exe, _ := fakeSoloist(t, now)
	m := installedModel(t, exe, now.Add(-95*24*time.Hour), true)
	soloist.MarkChecked(exe, now) // bugün bakılmış olsa da süresi dolduysa hemen
	m.engine, m.engineErr = engineFailed, soloist.ErrExpired
	m, _ = buildArrives(m)
	if m.engine != engineStarting {
		t.Fatalf("engine = %v", m.engine)
	}
}
