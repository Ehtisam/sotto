// Package keys, sotto'nun tuş haritası: hangi tuşun hangi işi yaptığı.
//
// Varsayılanlar Catalog'da. Kullanıcı %APPDATA%\sotto\keymap.toml dosyasında
// bir işe başka tuşlar verebilir; "g l" gibi boşlukla ayrılmış tuşlar sırayla
// basılan bir dizidir. Bir işe tuş verilince o işin varsayılan tuşları
// (başka bir işe verilmedikçe) boşa çıkar.
package keys

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// Action, tuşla yapılan bir iş.
type Action struct {
	Name  string   // keymap.toml'daki adı
	Keys  []string // varsayılan tuşlar
	Canon string   // arayüzün bu iş için beklediği tuş ("" ise bir komut: Name ile yürür)
	Help  string   // ? listesindeki açıklamanın çeviri anahtarı
	Group string   // ? listesindeki bölümü
}

// Catalog, bütün işler ve varsayılan tuşları.
var Catalog = []Action{
	// gezinme
	{"up", []string{"k", "up"}, "k", "help.k.up", "move"},
	{"down", []string{"j", "down"}, "j", "help.k.down", "move"},
	{"page_up", []string{"pgup"}, "pgup", "help.k.pgup", "move"},
	{"page_down", []string{"pgdown"}, "pgdown", "help.k.pgdown", "move"},
	{"top", []string{"g g", "home"}, "home", "help.k.top", "move"},
	{"bottom", []string{"G", "end"}, "G", "help.k.bottom", "move"},
	{"open", []string{"enter"}, "enter", "help.open", "move"},
	{"back", []string{"esc"}, "esc", "help.back", "move"},
	{"next_tab", []string{"tab"}, "tab", "help.tab", "move"},
	{"prev_tab", []string{"shift+tab"}, "shift+tab", "help.backtab", "move"},
	{"find", []string{"ctrl+f"}, "ctrl+f", "help.infind", "move"},
	{"sort", []string{"O"}, "O", "help.sort", "move"},
	{"find_playing", []string{"c"}, "c", "help.find", "move"},

	// sayfalar
	{"go_now", []string{"g n"}, "", "help.g.now", "pages"},
	{"go_home", []string{"g h"}, "", "help.g.home", "pages"},
	{"go_search", []string{"g s"}, "", "help.g.search", "pages"},
	{"go_library", []string{"g l"}, "", "help.g.library", "pages"},
	{"go_queue", []string{"g q"}, "", "help.g.queue", "pages"},
	{"go_devices", []string{"g d"}, "", "help.g.devices", "pages"},
	{"go_profile", []string{"g p"}, "", "help.g.profile", "pages"},
	{"go_settings", []string{"g ,"}, "", "help.g.settings", "pages"},
	{"go_top", []string{"g t"}, "", "help.g.top", "pages"},
	{"go_recent", []string{"g r"}, "", "help.g.recent", "pages"},
	{"go_log", []string{"g o"}, "", "help.g.log", "pages"},
	{"search", []string{"/"}, "/", "help.search", "pages"},

	// çalma
	{"play_pause", []string{"space"}, "space", "help.pause", "play"},
	{"next", []string{"n"}, "n", "help.k.next", "play"},
	{"previous", []string{"p"}, "p", "help.k.prev", "play"},
	{"seek_back", []string{","}, ",", "help.k.seekback", "play"},
	{"seek_forward", []string{"."}, ".", "help.k.seekfwd", "play"},
	{"restart", []string{"0"}, "0", "help.restart", "play"},
	{"volume_up", []string{"+", "="}, "+", "help.k.volup", "play"},
	{"volume_down", []string{"-"}, "-", "help.k.voldown", "play"},
	{"mute", []string{"m"}, "m", "help.mute", "play"},
	{"shuffle", []string{"s"}, "s", "help.shuf", "play"},
	{"repeat", []string{"r"}, "r", "help.rep", "play"},
	{"switch_speaker", []string{"t"}, "t", "help.switch", "play"},
	{"soloist", []string{"o"}, "o", "help.solo", "play"},
	{"sleep", []string{"z"}, "z", "help.sleep", "play"},

	// seçili öğe
	{"actions", []string{"a"}, "a", "help.actions", "item"},
	{"like", []string{"h"}, "h", "help.like", "item"},
	{"queue_add", []string{"u"}, "u", "help.queue", "item"},
	{"shuffle_play", []string{"S"}, "S", "help.shufpl", "item"},
	{"open_clipboard", []string{"v"}, "v", "help.paste", "item"},
	{"move_down", []string{"J"}, "J", "help.k.movedown", "item"},
	{"move_up", []string{"K"}, "K", "help.k.moveup", "item"},
	{"delete", []string{"x", "delete"}, "x", "help.k.delete", "item"},
	{"new_playlist", []string{"N"}, "N", "help.newlist", "item"},
	{"new_folder", []string{"f"}, "f", "help.folder", "item"},
	{"refresh", []string{"R"}, "R", "help.k.refresh", "item"},

	// genel
	{"lyrics", []string{"l"}, "l", "help.lyrics", "other"},
	{"lyrics_login", []string{"L"}, "L", "help.lyrlogin", "other"},
	{"help", []string{"?"}, "?", "help.keys", "other"},
	{"quit", []string{"q"}, "q", "help.quit", "other"},
}

// Groups, ? listesindeki bölümlerin sırası ve başlık anahtarları.
var Groups = []struct{ ID, Title string }{
	{"move", "help.grp.move"}, {"pages", "help.grp.pages"}, {"play", "help.grp.play"},
	{"item", "help.grp.item"}, {"other", "help.grp.other"},
}

// ByName, adı verilen iş.
func ByName(name string) (Action, bool) {
	for _, a := range Catalog {
		if a.Name == name {
			return a, true
		}
	}
	return Action{}, false
}

// Map, yüklenmiş tuş haritası.
type Map struct {
	bind     map[string]string   // tuş ya da dizi ("g l") → iş adı; "" ise boşa çıkmış
	prefixes map[string]bool     // dizilerin başları ("g", "g x" …)
	keysOf   map[string][]string // iş → tuşları (gösterim için)

	Counts  bool          // vim tarzı sayılar: 5j
	Timeout time.Duration // dizi ve sayı için bekleme
	Errors  []string      // dosyadaki sorunlar (açılışta gösterilir)
}

// Default, dosya olmadan varsayılan harita.
func Default() Map {
	m, _ := build(nil, true, 0)
	return m
}

func build(user map[string][]string, counts bool, timeoutMS int) (Map, []string) {
	m := Map{bind: map[string]string{}, prefixes: map[string]bool{}, keysOf: map[string][]string{}, Counts: counts}
	m.Timeout = time.Duration(timeoutMS) * time.Millisecond
	if m.Timeout <= 0 {
		m.Timeout = time.Second
	}
	var errs []string
	// Önce varsayılanlar; sonra kullanıcınınkiler üstüne yazılır.
	for _, a := range Catalog {
		if _, mine := user[a.Name]; mine {
			continue
		}
		for _, k := range a.Keys {
			m.bind[k] = a.Name
		}
		m.keysOf[a.Name] = a.Keys
	}
	names := make([]string, 0, len(user))
	for n := range user {
		names = append(names, n)
	}
	sort.Strings(names)
	userKey := map[string]string{}
	for _, name := range names {
		for _, k := range user[name] {
			if prev, ok := userKey[k]; ok && prev != name {
				errs = append(errs, fmt.Sprintf("%q is set for both %s and %s; %s wins", k, prev, name, name))
			}
			userKey[k] = name
			m.bind[k] = name
		}
		m.keysOf[name] = user[name]
	}
	// Kullanıcının tuş verdiği işlerin varsayılan tuşları, kullanıcı onları
	// başka bir işe vermediyse boşa çıkar (yoksa eski işi yapmaya devam ederdi).
	for _, name := range names {
		a, _ := ByName(name)
		for _, k := range a.Keys {
			if _, mine := userKey[k]; mine {
				continue
			}
			if owner, ok := m.bind[k]; !ok || owner == name {
				m.bind[k] = ""
			}
		}
	}
	// Başka bir işe verilen varsayılan tuş, eski işin gösteriminden düşer.
	for name, ks := range m.keysOf {
		var keep []string
		for _, k := range ks {
			if m.bind[k] == name {
				keep = append(keep, k)
			}
		}
		m.keysOf[name] = keep
	}
	for k, name := range m.bind {
		if name == "" {
			continue
		}
		parts := strings.Fields(k)
		for i := 1; i < len(parts); i++ {
			m.prefixes[strings.Join(parts[:i], " ")] = true
		}
	}
	return m, errs
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// Lookup, basılan tuşun (ya da dizinin) işi. bound=false: haritada yok (tuş
// olduğu gibi kullanılır). Boş ad: tuş boşa çıkarılmış.
func (m Map) Lookup(k string) (action string, bound bool) {
	a, ok := m.bind[k]
	return a, ok
}

// IsPrefix: k bir dizinin başı (sonraki tuş beklenmeli).
func (m Map) IsPrefix(k string) bool { return m.prefixes[k] }

// KeysOf, işin tuşları (gösterim için; ilk sıradaki asıl tuş).
func (m Map) KeysOf(action string) []string { return m.keysOf[action] }

// Key, işin gösterilecek ilk tuşu ("" ise tuşu yok).
func (m Map) Key(action string) string {
	if ks := m.keysOf[action]; len(ks) > 0 {
		return ks[0]
	}
	return ""
}

// ---- dosya ----

// File, tuş haritası dosyasının yolu.
func File(configDir string) string { return filepath.Join(configDir, "keymap.toml") }

type fileForm struct {
	Keys    map[string]any `toml:"keys"`
	Options struct {
		Counts            *bool `toml:"counts"`
		SequenceTimeoutMS int   `toml:"sequence_timeout_ms"`
	} `toml:"options"`
}

// Load, keymap.toml'u okur. Dosya yoksa açıklamalı bir tane yazar ve
// varsayılanları döner. Sorunlu satırlar atlanır, Errors'ta söylenir.
func Load(configDir string) Map {
	path := File(configDir)
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		_ = os.MkdirAll(configDir, 0o700)
		_ = os.WriteFile(path, []byte(Template()), 0o600)
		return Default()
	}
	if err != nil {
		m := Default()
		m.Errors = []string{err.Error()}
		return m
	}
	return Parse(b)
}

// Parse, keymap.toml içeriğini çözer.
func Parse(b []byte) Map {
	var f fileForm
	if err := toml.Unmarshal(b, &f); err != nil {
		m := Default()
		m.Errors = []string{"keymap.toml: " + err.Error()}
		return m
	}
	var errs []string
	user := map[string][]string{}
	for name, v := range f.Keys {
		if _, ok := ByName(name); !ok {
			errs = append(errs, fmt.Sprintf("unknown action %q", name))
			continue
		}
		var raw []string
		switch x := v.(type) {
		case string:
			raw = []string{x}
		case []any:
			for _, e := range x {
				s, ok := e.(string)
				if !ok {
					errs = append(errs, fmt.Sprintf("%s: keys must be text", name))
					continue
				}
				raw = append(raw, s)
			}
		default:
			errs = append(errs, fmt.Sprintf("%s: expected a key or a list of keys", name))
			continue
		}
		var ks []string
		for _, r := range raw {
			k, err := Normalize(r)
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", name, err))
				continue
			}
			if k != "" {
				ks = append(ks, k)
			}
		}
		user[name] = ks // boş liste: iş tuşsuz kalır
	}
	counts := true
	if f.Options.Counts != nil {
		counts = *f.Options.Counts
	}
	m, more := build(user, counts, f.Options.SequenceTimeoutMS)
	m.Errors = append(errs, more...)
	sort.Strings(m.Errors)
	return m
}

var named = map[string]string{
	" ": "space", "<space>": "space", "spacebar": "space", "return": "enter", "escape": "esc",
	"pageup": "pgup", "pagedown": "pgdown", "page_up": "pgup", "page_down": "pgdown",
	"del": "delete", "bs": "backspace", "<cr>": "enter", "<esc>": "esc", "<tab>": "tab",
}

// Normalize, kullanıcının yazdığı tuşu Bubble Tea'nın adlandırmasına çevirir:
// "Ctrl+N" → "ctrl+n", "shift+j" → "J", "g  l" → "g l".
func Normalize(s string) (string, error) {
	if strings.TrimSpace(s) == "" {
		if s == " " {
			return "space", nil
		}
		return "", nil
	}
	var out []string
	for _, part := range strings.Fields(s) {
		k, err := normalizeOne(part)
		if err != nil {
			return "", err
		}
		out = append(out, k)
	}
	if len(out) > 3 {
		return "", fmt.Errorf("%q: a sequence can have at most 3 keys", s)
	}
	return strings.Join(out, " "), nil
}

func normalizeOne(p string) (string, error) {
	if len([]rune(p)) == 1 {
		return p, nil
	}
	low := strings.ToLower(p)
	if n, ok := named[low]; ok {
		return n, nil
	}
	mods := strings.Split(low, "+")
	base := mods[len(mods)-1]
	if base == "" { // "ctrl++"
		base, mods = "+", mods[:len(mods)-2]
	} else {
		mods = mods[:len(mods)-1]
	}
	if n, ok := named[base]; ok {
		base = n
	}
	// Özgün yazımdaki büyük harfi koru ("ctrl+K" değil ama tek başına "K").
	if len([]rune(base)) == 1 && len(mods) == 0 {
		return string([]rune(p)[len([]rune(p))-1]), nil
	}
	shift := false
	var keep []string
	for _, md := range mods {
		switch md {
		case "ctrl", "alt", "super", "meta", "hyper":
			keep = append(keep, md)
		case "shift":
			shift = true
		case "cmd", "win":
			keep = append(keep, "super")
		case "control":
			keep = append(keep, "ctrl")
		case "opt", "option":
			keep = append(keep, "alt")
		default:
			return "", fmt.Errorf("unknown modifier %q in %q", md, p)
		}
	}
	if shift {
		r := []rune(base)
		if len(r) == 1 && strings.ToUpper(base) != base && len(keep) == 0 {
			return strings.ToUpper(base), nil // shift+j → J
		}
		keep = append(keep, "shift")
	}
	// Bubble Tea'nın sırası: ctrl+alt+shift+super…
	order := map[string]int{"ctrl": 0, "alt": 1, "shift": 2, "meta": 3, "hyper": 4, "super": 5}
	sort.Slice(keep, func(i, j int) bool { return order[keep[i]] < order[keep[j]] })
	return strings.Join(append(keep, base), "+"), nil
}

// Template, ilk açılışta yazılan açıklamalı keymap.toml.
func Template() string {
	var b strings.Builder
	b.WriteString(`# sotto keys
# Give any action other keys here. Keys are written as sotto shows them:
# "n", "N" (shift+n), "ctrl+n", "alt+x", "space", "enter", "esc", "tab",
# "up", "pgdown", "f5". Keys separated by a space are pressed one after the
# other: "g l". A list gives several keys: next = ["n", "ctrl+right"].
# When you give an action new keys, its default keys stop working (unless you
# give them to something else). An empty list [] leaves the action without keys.
# Remove the # in front of a line to change it. sotto reads this file when it
# starts.

[options]
# Vim-style counts: 5j moves down five rows, 3K moves a song up three places.
# While a count could follow, a lone 1-8 in a list waits a moment before
# going to that page.
counts = true
# How long sotto waits for the next key of a sequence like "g l" (ms).
sequence_timeout_ms = 1000

[keys]
`)
	for _, g := range Groups {
		fmt.Fprintf(&b, "\n# %s\n", g.ID)
		for _, a := range Catalog {
			if a.Group != g.ID {
				continue
			}
			ks := make([]string, len(a.Keys))
			for i, k := range a.Keys {
				ks[i] = fmt.Sprintf("%q", k)
			}
			v := ks[0]
			if len(ks) > 1 {
				v = "[" + strings.Join(ks, ", ") + "]"
			}
			fmt.Fprintf(&b, "# %s = %s\n", a.Name, v)
		}
	}
	return b.String()
}
