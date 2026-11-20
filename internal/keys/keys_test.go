package keys

import (
	"os"
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"n": "n", "N": "N", "Ctrl+N": "ctrl+n", "shift+j": "J", "SPACE": "space", " ": "space",
		"g  l": "g l", "<CR>": "enter", "PageDown": "pgdown", "ctrl+shift+x": "ctrl+shift+x",
		"shift+ctrl+x": "ctrl+shift+x", "alt+Enter": "alt+enter", "cmd+k": "super+k",
	}
	for in, want := range cases {
		got, err := Normalize(in)
		if err != nil || got != want {
			t.Errorf("Normalize(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := Normalize("hyperx+k"); err == nil {
		t.Error("unknown modifier accepted")
	}
	if _, err := Normalize("a b c d"); err == nil {
		t.Error("4-key sequence accepted")
	}
}

func TestDefaults(t *testing.T) {
	m := Default()
	if a, ok := m.Lookup("n"); !ok || a != "next" {
		t.Fatalf("n → %q", a)
	}
	if a, _ := m.Lookup("down"); a != "down" {
		t.Fatalf("down → %q", a)
	}
	if !m.IsPrefix("g") || m.IsPrefix("n") {
		t.Fatal("g should start sequences, n should not")
	}
	if a, _ := m.Lookup("g l"); a != "go_library" {
		t.Fatalf("g l → %q", a)
	}
	if a, _ := m.Lookup("g g"); a != "top" {
		t.Fatalf("g g → %q", a)
	}
	if _, ok := m.Lookup("1"); ok {
		t.Fatal("digits are not in the map")
	}
	// Her işin varsayılan tuşu çakışmasız olmalı.
	seen := map[string]string{}
	for _, a := range Catalog {
		for _, k := range a.Keys {
			if prev, ok := seen[k]; ok {
				t.Errorf("%q is the default of both %s and %s", k, prev, a.Name)
			}
			seen[k] = a.Name
		}
	}
}

func TestUserKeysReplaceDefaults(t *testing.T) {
	m := Parse([]byte(`
[keys]
next = "ctrl+n"
quit = ["Q", "ctrl+q"]
lyrics = []
go_library = "space l"
`))
	if len(m.Errors) != 0 {
		t.Fatalf("errors: %v", m.Errors)
	}
	if a, _ := m.Lookup("ctrl+n"); a != "next" {
		t.Fatalf("ctrl+n → %q", a)
	}
	if a, ok := m.Lookup("n"); !ok || a != "" {
		t.Fatalf("n should be freed, got %q %v", a, ok)
	}
	if a, _ := m.Lookup("Q"); a != "quit" {
		t.Fatal("Q not quit")
	}
	if a, ok := m.Lookup("q"); !ok || a != "" {
		t.Fatal("q should be freed")
	}
	if a, ok := m.Lookup("l"); !ok || a != "" {
		t.Fatal("l should be freed (lyrics = [])")
	}
	// space bir dizinin başı oldu; çal/durdur'un kendi tuşu hâlâ space.
	if !m.IsPrefix("space") {
		t.Fatal("space l should make space a prefix")
	}
	if m.Key("next") != "ctrl+n" || m.Key("lyrics") != "" {
		t.Fatalf("display keys: %q %q", m.Key("next"), m.Key("lyrics"))
	}
}

func TestUserKeyTakesAnotherActionsDefault(t *testing.T) {
	m := Parse([]byte("[keys]\nquit = \"x\"\n"))
	if a, _ := m.Lookup("x"); a != "quit" {
		t.Fatalf("x → %q", a)
	}
	if ks := m.KeysOf("delete"); len(ks) != 1 || ks[0] != "delete" {
		t.Fatalf("delete keys %v", ks)
	}
}

func TestParseErrors(t *testing.T) {
	m := Parse([]byte(`
[keys]
nonsense = "k"
next = 5
quit = "hyperx+q"
up = "w"
down = "w"
`))
	joined := strings.Join(m.Errors, "\n")
	for _, want := range []string{`unknown action "nonsense"`, "next: expected", "unknown modifier", `"w" is set for both`} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing error %q in:\n%s", want, joined)
		}
	}
	if Parse([]byte("[keys\n")).Errors == nil {
		t.Error("broken toml gave no error")
	}
}

func TestOptionsAndTemplate(t *testing.T) {
	m := Parse([]byte("[options]\ncounts = false\nsequence_timeout_ms = 400\n"))
	if m.Counts || m.Timeout.Milliseconds() != 400 {
		t.Fatalf("options %+v", m)
	}
	// Yazılan dosya olduğu gibi okununca varsayılanları verir.
	d := Parse([]byte(Template()))
	if len(d.Errors) != 0 || !d.Counts {
		t.Fatalf("template: %v", d.Errors)
	}
	for _, a := range Catalog {
		if !strings.Contains(Template(), "# "+a.Name+" = ") {
			t.Errorf("template misses %s", a.Name)
		}
	}
	dir := t.TempDir()
	if Load(dir).Errors != nil {
		t.Fatal("first load errors")
	}
	if _, err := os.Stat(File(dir)); err != nil {
		t.Fatal("keymap.toml not written")
	}
}
