package ui

import (
	"os"
	"strings"
	"testing"

	"github.com/Ehtisam/sotto/internal/config"
)

const testNotes = `<!--
comment
-->

## 0.3.0
- Third

## 0.2.0
- Second
- More

## 0.1.0
- First
`

func TestParseNotesAndRange(t *testing.T) {
	all := parseNotes(testNotes)
	if len(all) != 3 || all[0].version != "0.3.0" || strings.Join(all[1].lines, "|") != "- Second|- More" {
		t.Fatalf("parsed %+v", all)
	}
	got := notesBetween(all, "0.1.0", "0.3.0")
	if len(got) != 2 || got[0].version != "0.3.0" || got[1].version != "0.2.0" {
		t.Fatalf("between = %+v", got)
	}
	if !versionLess("0.2.9", "0.2.10") || versionLess("v1.0.0", "1.0.0-rc1") || !versionLess("0.9", "1.0.0") {
		t.Fatal("versionLess")
	}
	// Gömülü notlar okunabiliyor ve iki dilde aynı sürümler var.
	en, tr := parseNotes(whatsNewEN), parseNotes(whatsNewTR)
	if len(en) == 0 || len(en) != len(tr) {
		t.Fatalf("embedded notes: en %d, tr %d", len(en), len(tr))
	}
	for i := range en {
		if en[i].version != tr[i].version {
			t.Fatalf("version %d: en %s, tr %s", i, en[i].version, tr[i].version)
		}
	}
}

func TestWhatsNewOnce(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{ConfigDir: dir, LocalDir: dir, Spotify: config.Spotify{ClientID: "x"}}
	old, oldEN, oldTR := appVersion, whatsNewEN, whatsNewTR
	defer func() { appVersion, whatsNewEN, whatsNewTR = old, oldEN, oldTR }()

	// İlk kurulum: pencere yok, sürüm yazılır.
	appVersion = func() string { return "0.1.0" }
	if m := New(cfg); m.whatsnew.open {
		t.Fatal("shown on first run")
	}
	if b, _ := os.ReadFile(seenVersionFile(dir)); strings.TrimSpace(string(b)) != "0.1.0" {
		t.Fatalf("seen-version = %q", b)
	}
	// Sürüm değişti ama notu yok: pencere yok, yine de yazılır.
	appVersion = func() string { return "0.1.1" }
	if m := New(cfg); m.whatsnew.open {
		t.Fatal("shown with no notes")
	}
	// Notu olan bir sürüme geçiş: bir kez açılır.
	whatsNewEN, whatsNewTR = testNotes, testNotes
	appVersion = func() string { return "0.3.0" }
	m := New(cfg)
	if !m.whatsnew.open || len(m.whatsnew.notes) != 2 {
		t.Fatalf("not shown: %+v", m.whatsnew)
	}
	if m2 := New(cfg); m2.whatsnew.open {
		t.Fatal("shown twice")
	}
}
