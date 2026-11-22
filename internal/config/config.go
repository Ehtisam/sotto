// Package config, sotto'nun ayar dosyasını ve klasörlerini yönetir.
//
// Bütün ayarlar tek dosyada durur. Windows'ta:
//
//	%APPDATA%\sotto\config.toml     ayarların hepsi (Soloist anahtarı dahil)
//	%LOCALAPPDATA%\sotto\           sotto'nun kendi tuttukları: Soloist,
//	                                Spotify girişi, önbellek, kayıtlar
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	Language string  `toml:"language"`
	Look     Look    `toml:"look"`
	Spotify  Spotify `toml:"spotify"`
	Soloist  Soloist `toml:"soloist"`
	Extras   Extras  `toml:"extras"`
	Desktop  Desktop `toml:"desktop"`
	Hooks    Hooks   `toml:"hooks"`

	// Colors: temanın üstüne öğe öğe renkler ("text" = "#e8e0d0"). Anahtarlar
	// theme.ColorKeys'te.
	Colors map[string]string `toml:"colors"`

	// Doldurulan yollar (dosyada yok).
	ConfigDir string `toml:"-"`
	LocalDir  string `toml:"-"`
}

// Look: görünüm ayarları.
type Look struct {
	Theme  string `toml:"theme"`  // cardo, night, moss, mono, paper
	Accent string `toml:"accent"` // boş: temanınki; purple, blue, teal, green, amber, pink, red
	Covers string `toml:"covers"` // image (destek varsa resim, yoksa piksel), pixel ya da off

	// CellRatio, hücrenin gerçek en/boy oranı; 0: kendiliğinden. Kapaklar
	// kare görünmüyorsa elle ayarlanır (ör. 0.45).
	CellRatio float64 `toml:"cell_ratio"`

	LineNumbers string `toml:"line_numbers"`  // absolute, relative ya da off
	Border      string `toml:"border"`        // line, heavy, double, dashed, ascii ya da none
	Icons       string `toml:"icons"`         // unicode, ascii ya da nerd
	Strip       string `toml:"strip"`         // çalan şarkı şeridi: bottom ya da top
	StripFormat string `toml:"strip_format"`  // şeridin ilk satırı: {title} {artist} {album} {context} {device}
	Progress    string `toml:"progress"`      // ilerleme çubuğu: line, block, dots ya da thin
	WindowTitle string `toml:"window_title"`  // pencere başlığı; aynı alanlar
	Mouse       bool   `toml:"mouse"`         // fare: tıklama ve tekerlek
	Visualizer  string `toml:"visualizer"`    // ses görselleştirmesi: now, strip ya da off
	Menu        string `toml:"menu"`          // sayfa menüsünün yeri: left, right ya da hidden
	Buttons     bool   `toml:"strip_buttons"` // şeritte tıklanabilir düğmeler
	VizStyle    string `toml:"viz_style"`     // tam ekran görselleştirme (V): bars, winamp, scope, vu, cassette
	Layout      string `toml:"layout"`        // plain (çizgiler) ya da boxed (kutular)
	Panel       string `toml:"side_panel"`    // geniş pencerede sağda çalan şarkı paneli: auto ya da off
}

// PanelOn: yan panel açık mı (yer varsa gösterilir).
func (l Look) PanelOn() bool { return l.Panel != "off" }

// CoversOn: kapaklar çizilsin mi.
func (l Look) CoversOn() bool { return l.Covers != "off" }

type Spotify struct {
	ClientID string `toml:"client_id"`
}

// Extras: Spotify'ın resmî olmayan iç bağlantılarıyla gelen özellikler (✱).
type Extras struct {
	Enabled bool `toml:"enabled"`
}

// Desktop: işletim sistemiyle bağlar (bildirimler, medya tuşları).
type Desktop struct {
	// Notify: şarkı değişince bildirim: "always", "unfocused" (sotto'nun
	// penceresi öndeyken gösterme) ya da "off".
	Notify      string `toml:"notify"`
	NotifyTitle string `toml:"notify_title"`
	NotifyBody  string `toml:"notify_body"`
	// MediaControls: medya tuşları ve Windows'un medya kartı.
	MediaControls bool `toml:"media_controls"`
	// StartMenu: Başlat menüsünde kısayol (Windows medya kartında ve
	// bildirimlerde "sotto" adı ve simgesi için gerekli).
	StartMenu bool `toml:"start_menu"`
	// Discord'da "şu an dinliyor". DiscordAppID boşsa sotto'nun kendi
	// Discord uygulaması (DefaultDiscordAppID) kullanılır.
	Discord      bool   `toml:"discord"`
	DiscordAppID string `toml:"discord_app_id"`
}

// DefaultDiscordAppID: discord.com/developers'daki "sotto" uygulaması
// (profilde "Listening to sotto" ve sotto simgesi). Herkese açık bir numara.
const DefaultDiscordAppID = "1573809415608467519"

// DiscordID, kullanılacak Discord uygulama kimliği.
func (d Desktop) DiscordID() string {
	if id := strings.TrimSpace(d.DiscordAppID); id != "" {
		return id
	}
	return DefaultDiscordAppID
}

// Hooks: olaylarda çalıştırılan komutlar.
type Hooks struct {
	OnTrackChange  string `toml:"on_track_change"`
	OnStatusChange string `toml:"on_status_change"`
}

type Soloist struct {
	Autostart bool `toml:"autostart"`
	// OnStart: açılışta hiçbir yerde bir şey çalmıyorsa "paused" (kaldığın yeri
	// duraklatılmış yükle), "play" (kaldığın yerden çal) ya da "off".
	OnStart    string `toml:"on_start"`
	Crossfade  int    `toml:"crossfade"` // şarkılar arası geçiş, sn (0: kapalı)
	Quality    string `toml:"quality"`   // normal, high, very_high ya da lossless
	Normalize  bool   `toml:"normalize"` // ses düzeyini eşitle
	AutoUpdate bool   `toml:"auto_update"`

	// Eski sürümlerin "resume = true/false" ayarı; okunur, on_start'a çevrilir.
	Resume *bool `toml:"resume"`

	APIKey        string `toml:"api_key"`
	Path          string `toml:"path"`
	DeviceName    string `toml:"device_name"`
	InitialVolume int    `toml:"initial_volume"`

	// Eski sürümlerden kalan ad; okunur, yazılmaz.
	Enabled *bool `toml:"enabled"`
}

// normalize: eski "resume"u on_start'a çevirir, bilinmeyen değerleri düzeltir.
func (s *Soloist) normalize() {
	// "resume" yalnız eski dosyalarda olur (yenisi yazılmaz).
	if s.Resume != nil && !*s.Resume {
		s.OnStart = "off"
	}
	s.Resume = nil
	switch s.OnStart {
	case "paused", "play", "off":
	default:
		s.OnStart = "paused"
	}
	switch s.Quality {
	case "normal", "high", "very_high", "lossless":
	default:
		s.Quality = "very_high"
	}
	s.Crossfade = max(0, min(12, s.Crossfade))
}

// Görünüm varsayılanları.
const (
	DefaultStripFormat = "{title} — {artist}"
	DefaultWindowTitle = "{title} — {artist} · sotto"
)

func defaults() Config {
	return Config{Language: "en", Look: Look{Theme: "cardo", Covers: "image", LineNumbers: "absolute", Border: "line",
		Icons: "unicode", Strip: "bottom", StripFormat: DefaultStripFormat, Progress: "line", WindowTitle: DefaultWindowTitle, Mouse: true, Visualizer: "now", Menu: "left", Layout: "plain", VizStyle: "bars", Panel: "auto"}, Soloist: Soloist{Autostart: true, OnStart: "paused", Quality: "very_high", Normalize: true, AutoUpdate: true, DeviceName: "sotto", InitialVolume: 40}, Extras: Extras{Enabled: true},
		Desktop: Desktop{Notify: "unfocused", NotifyTitle: "{title}", NotifyBody: "{artist} · {album}", MediaControls: true, StartMenu: true}}
}

// Load, ayar dosyasını okur. Dosya yoksa açıklamalı bir tane yazar.
// Eski sürümlerin ayrı dosyalarını (soloist.key, spotify-token.json) yerine taşır.
func Load() (Config, error) {
	c := defaults()
	cfgRoot, err := os.UserConfigDir()
	if err != nil {
		return c, err
	}
	localRoot, err := os.UserCacheDir() // Windows'ta %LOCALAPPDATA%
	if err != nil {
		return c, err
	}
	c.ConfigDir = filepath.Join(cfgRoot, "sotto")
	c.LocalDir = filepath.Join(localRoot, "sotto")

	b, err := os.ReadFile(c.ConfigFile())
	exists := true
	switch {
	case errors.Is(err, fs.ErrNotExist):
		exists = false
		c.Language = SystemLanguage() // ilk açılış: sistemin dili
	case err != nil:
		return c, err
	default:
		if err := toml.Unmarshal(b, &c); err != nil {
			return c, fmt.Errorf("%s: %w", c.ConfigFile(), err)
		}
		// Yeni sürümle gelen bölümler dosyada yoksa açıklamalarıyla eklensin.
		for _, section := range []string{"[look]", "[spotify]", "[soloist]", "[extras]", "[desktop]", "[hooks]", "on_start =", "crossfade =", "auto_update =", "start_menu =", "line_numbers =", "[colors]", "visualizer =", "layout =", "discord =", "side_panel ="} {
			if !strings.Contains(string(b), section) {
				exists = false
			}
		}
	}
	if c.Soloist.Enabled != nil {
		c.Soloist.Autostart, c.Soloist.Enabled = *c.Soloist.Enabled, nil
		exists = false // yeni adla yeniden yaz
	}
	if c.Soloist.DeviceName == "" {
		c.Soloist.DeviceName = "sotto"
	}
	c.Soloist.normalize()

	// Eski: ayrı anahtar dosyası.
	oldKey := filepath.Join(c.ConfigDir, "soloist.key")
	if k, err := os.ReadFile(oldKey); err == nil {
		if c.Soloist.APIKey == "" {
			c.Soloist.APIKey = strings.TrimSpace(strings.TrimPrefix(string(k), string(rune(0xFEFF))))
		}
		if err := c.Save(); err == nil {
			_ = os.Remove(oldKey)
		}
		exists = true
	}
	// Eski: giriş bilgisi ayar klasöründeydi; sotto'nun kendi klasörüne taşı.
	oldTok := filepath.Join(c.ConfigDir, "spotify-token.json")
	if _, err := os.Stat(oldTok); err == nil {
		if _, err := os.Stat(c.TokenFile()); errors.Is(err, fs.ErrNotExist) {
			_ = os.MkdirAll(c.LocalDir, 0o700)
			_ = os.Rename(oldTok, c.TokenFile())
		}
	}

	if !exists {
		if err := c.Save(); err != nil {
			return c, err
		}
	}
	return c, nil
}

// Save, ayarları açıklamalarıyla birlikte dosyaya yazar.
func (c Config) Save() error {
	if err := os.MkdirAll(c.ConfigDir, 0o700); err != nil {
		return err
	}
	text := fmt.Sprintf(template,
		quote(c.Language),
		quote(c.Look.Theme),
		quote(c.Look.Accent),
		quote(c.Look.Covers),
		c.Look.CellRatio,
		quote(c.Look.LineNumbers),
		quote(c.Look.Border),
		quote(c.Look.Icons),
		quote(c.Look.Strip),
		quote(c.Look.StripFormat),
		quote(c.Look.Progress),
		quote(c.Look.WindowTitle),
		c.Look.Mouse,
		quote(c.Look.Visualizer),
		quote(c.Look.Menu),
		quote(c.Look.Layout),
		quote(c.Look.Panel),
		c.Look.Buttons,
		quote(c.Look.VizStyle),
		quote(c.Spotify.ClientID),
		c.Soloist.Autostart,
		quote(c.Soloist.OnStart),
		c.Soloist.Crossfade,
		quote(c.Soloist.Quality),
		c.Soloist.Normalize,
		c.Soloist.AutoUpdate,
		quote(c.Soloist.APIKey),
		quote(c.Soloist.Path),
		quote(c.Soloist.DeviceName),
		c.Soloist.InitialVolume,
		c.Extras.Enabled,
		quote(c.Desktop.Notify),
		quote(c.Desktop.NotifyTitle),
		quote(c.Desktop.NotifyBody),
		c.Desktop.MediaControls,
		c.Desktop.StartMenu,
		c.Desktop.Discord,
		quote(c.Desktop.DiscordAppID),
		quote(c.Hooks.OnTrackChange),
		quote(c.Hooks.OnStatusChange),
	)
	text += colorsSection(c.Colors)
	return os.WriteFile(c.ConfigFile(), []byte(text), 0o600)
}

const template = `# sotto settings
# Everything sotto can be configured with lives in this file.

# Interface language: "en" or "tr".
language = %s

[look]
# Theme: "cardo", "night", "moss", "mono", "paper" (light), "dracula", "nord",
# "gruvbox", "catppuccin", "tokyonight", "solarized", "monokai" or "terminal"
# (the terminal's own colours). Themes you import show up by their file name.
theme = %s
# Accent colour over the theme's own: "purple", "blue", "teal", "green", "amber",
# "pink" or "red". Leave empty to keep the theme's.
accent = %s
# Album covers: "image" (real pictures where the terminal supports Sixel, like
# Windows Terminal; pixel blocks elsewhere), "pixel" or "off".
covers = %s
# Width / height of one character cell on screen, so covers come out square.
# 0 works it out (0.45 on Windows Terminal). Set it by hand if covers look
# stretched, e.g. 0.42 makes them wider, 0.5 taller.
cell_ratio = %g
# Numbers in front of songs: "absolute", "relative" (distance from the
# selected row, like vim's relativenumber) or "off".
line_numbers = %s
# Lines between the parts of the screen: "line", "heavy", "double", "dashed",
# "ascii" or "none".
border = %s
# Symbols: "unicode" (♪ ⇄ ↻ ♥), "ascii" (for fonts without them) or "nerd"
# (Nerd Font icons).
icons = %s
# Where the now-playing strip sits: "bottom" or "top".
strip = %s
# First line of the strip. Fields: {title} {artist} {album} {context} {device}.
strip_format = %s
# Progress bar: "line" (━──), "block" (██░░), "dots" (●○○) or "thin" (──•──).
progress = %s
# Terminal window title; same fields. Empty leaves the title alone.
window_title = %s
# Mouse: click a page or a row, click the progress bar to seek, scroll lists,
# scroll over the strip to change the volume. Turn off to select text freely.
mouse = %t
# Audio visualizer: "now" (bars on the Now playing page), "strip" (a small
# one in the strip) or "off". It listens to what this computer plays (Windows),
# so it moves while music plays here, through Soloist.
visualizer = %s
# Where the page menu sits: "left", "right" or "hidden" (pages are still a
# number away: 1-8).
menu = %s
# "plain" separates the parts of the screen with lines; "boxed" puts the menu,
# the page and the now-playing strip in rounded boxes, the one in use coloured.
layout = %s
# In a wide window (about 150 columns or more), a panel on the right keeps the
# playing song, its cover, the lyrics and what comes next in view on every
# page: "auto" or "off".
side_panel = %s
# Buttons in the now-playing strip for the mouse: previous, play/pause, next,
# shuffle and repeat.
strip_buttons = %t
# Full-screen visualizer (shift+V): "bars", "winamp", "scope" (oscilloscope),
# "vu" (a loudness meter) or "cassette". Left and right change it there too.
viz_style = %s

[spotify]
# Client ID of your own Spotify app (https://developer.spotify.com/dashboard).
# The app needs the redirect URI http://127.0.0.1:8765/callback and the Web API.
client_id = %s

[soloist]
# Light mode plays through Spotify Soloist on this computer.
# Start Soloist when sotto opens. You can also start and stop it with "o".
autostart = %t
# When sotto opens and nothing is playing anywhere: "paused" loads where you
# left off without playing, "play" carries on playing from there, "off" does
# nothing.
on_start = %s
# The settings below are Soloist's own; sotto writes them into Soloist's
# settings file and they take effect when Soloist (re)starts. Changing them
# on the Settings page restarts Soloist for you.
# Crossfade between songs in seconds, 0 to 12 (0 is off).
crossfade = %d
# Sound quality: "normal" (96 kbps), "high" (160), "very_high" (320) or
# "lossless" (FLAC, where your plan and country have Spotify lossless).
quality = %s
# Keep every song at about the same loudness.
normalize = %t
# Each Soloist build stops working 90 days after it is made. With this on,
# sotto downloads Spotify's newer build in the background when 14 days are
# left; the music doesn't stop, the new build is used the next time Soloist starts.
auto_update = %t
# Soloist API key (https://developer.spotify.com/dashboard/soloist). Keep it private.
api_key = %s
# Leave empty to use the copy sotto keeps in its own folder.
path = %s
# Name shown in Spotify's device list.
device_name = %s
initial_volume = %d

[extras]
# Features that use Spotify's unofficial routes, like Spotify's own lyrics.
# They start go-librespot for a few seconds about once an hour to get a key.
# Turn this off to use only official routes (lyrics then come from LRCLIB).
enabled = %t

[desktop]
# Notification when the song changes: "always", "unfocused" (only when sotto's
# window is not in front) or "off".
notify = %s
# Notification text. Fields: {title} {artist} {album} {context} {device}.
notify_title = %s
notify_body = %s
# Media keys and the system media card (Windows: volume flyout, lock screen;
# Linux: MPRIS, so desktop media controls and playerctl see sotto).
media_controls = %t
# Put sotto in the Start menu (Windows) or the app menu (Linux). Windows takes
# the name and icon on the media card and on notifications from there; without
# it the card says "Unknown app".
start_menu = %t
# Show what you're listening to on your Discord profile (Discord must be open).
# It shows as "Listening to sotto". To use your own app instead, put the
# Application ID of an app made at https://discord.com/developers/applications
# in discord_app_id; its name then shows as "Listening to <name>".
discord = %t
discord_app_id = %s

[hooks]
# Commands run in the background (cmd /C on Windows, sh -c elsewhere). The
# details come in environment variables: SOTTO_EVENT, SOTTO_TITLE,
# SOTTO_ARTIST, SOTTO_ALBUM, SOTTO_URI, SOTTO_CONTEXT, SOTTO_DEVICE,
# SOTTO_STATUS, SOTTO_DURATION_MS, SOTTO_COVER_URL.
# Runs when the song changes.
on_track_change = %s
# Runs when playback starts, pauses or stops.
on_status_change = %s
`

// quote, bir metni TOML dizgisi olarak yazar.
func quote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", "", "\t", `\t`)
	return `"` + r.Replace(s) + `"`
}

func (c Config) ConfigFile() string { return filepath.Join(c.ConfigDir, "config.toml") }

// TokenFile, Spotify girişinin saklandığı dosya. Bir ayar değil; sotto kendisi yazar ve yeniler.
func (c Config) TokenFile() string { return filepath.Join(c.LocalDir, "spotify-token.json") }

func (c Config) SoloistDir() string { return filepath.Join(c.LocalDir, "soloist") }

func (c Config) SoloistPath() string {
	if c.Soloist.Path != "" {
		return c.Soloist.Path
	}
	name := "soloist"
	if runtime.GOOS == "windows" {
		name = "soloist.exe"
	}
	return filepath.Join(c.SoloistDir(), name)
}

func (c Config) SoloistDataDir() string  { return filepath.Join(c.SoloistDir(), "data") }
func (c Config) SoloistCacheDir() string { return filepath.Join(c.SoloistDir(), "cache") }
func (c Config) SoloistLog() string      { return filepath.Join(c.LocalDir, "logs", "soloist.log") }

// SoloistAPIKey, önce SOLOIST_API_KEY ortam değişkenine, sonra ayar dosyasına bakar.
func (c Config) SoloistAPIKey() string {
	if k := strings.TrimSpace(os.Getenv("SOLOIST_API_KEY")); k != "" {
		return k
	}
	return strings.TrimSpace(c.Soloist.APIKey)
}

// colorsSection, [colors] bölümü: verilen renkler ve kullanılabilecek anahtarlar.
func colorsSection(colors map[string]string) string {
	var b strings.Builder
	b.WriteString(`
[colors]
# Your own colours over the theme, one element at a time, as "#rrggbb" or a
# terminal colour number "0"-"15". Keys: ground, text, muted, line, accent,
# accent_hi, selected, gold, red, and for single parts of the screen:
# brand (the sotto name), title, playing (♪ and the playing song), progress,
# selected_text, hint_key, hint_text, border.
# Example:  accent = "#e0a040"
`)
	keys := make([]string, 0, len(colors))
	for k := range colors {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "%s = %s\n", k, quote(colors[k]))
	}
	return b.String()
}
