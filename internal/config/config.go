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
}

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
}

// Hooks: olaylarda çalıştırılan komutlar.
type Hooks struct {
	OnTrackChange  string `toml:"on_track_change"`
	OnStatusChange string `toml:"on_status_change"`
}

type Soloist struct {
	Autostart     bool   `toml:"autostart"`
	Resume        bool   `toml:"resume"`
	AutoUpdate    bool   `toml:"auto_update"`
	APIKey        string `toml:"api_key"`
	Path          string `toml:"path"`
	DeviceName    string `toml:"device_name"`
	InitialVolume int    `toml:"initial_volume"`

	// Eski sürümlerden kalan ad; okunur, yazılmaz.
	Enabled *bool `toml:"enabled"`
}

func defaults() Config {
	return Config{Language: "en", Look: Look{Theme: "cardo", Covers: "image"}, Soloist: Soloist{Autostart: true, Resume: true, AutoUpdate: true, DeviceName: "sotto", InitialVolume: 40}, Extras: Extras{Enabled: true},
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
	case err != nil:
		return c, err
	default:
		if err := toml.Unmarshal(b, &c); err != nil {
			return c, fmt.Errorf("%s: %w", c.ConfigFile(), err)
		}
		// Yeni sürümle gelen bölümler dosyada yoksa açıklamalarıyla eklensin.
		for _, section := range []string{"[look]", "[spotify]", "[soloist]", "[extras]", "[desktop]", "[hooks]", "resume =", "auto_update =", "start_menu ="} {
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
		quote(c.Spotify.ClientID),
		c.Soloist.Autostart,
		c.Soloist.Resume,
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
		quote(c.Hooks.OnTrackChange),
		quote(c.Hooks.OnStatusChange),
	)
	return os.WriteFile(c.ConfigFile(), []byte(text), 0o600)
}

const template = `# sotto settings
# Everything sotto can be configured with lives in this file.

# Interface language: "en" or "tr".
language = %s

[look]
# Theme: "cardo", "night", "moss", "mono" or "paper" (light).
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

[spotify]
# Client ID of your own Spotify app (https://developer.spotify.com/dashboard).
# The app needs the redirect URI http://127.0.0.1:8765/callback and the Web API.
client_id = %s

[soloist]
# Light mode plays through Spotify Soloist on this computer.
# Start Soloist when sotto opens. You can also start and stop it with "o".
autostart = %t
# When nothing is playing anywhere, load where you left off (paused) once
# Soloist is up.
resume = %t
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
# Media keys and the system media card (Windows: volume flyout, lock screen).
media_controls = %t
# Windows: put sotto in the Start menu. Windows takes the name and icon on the
# media card and on notifications from there; without it the card says
# "Unknown app".
start_menu = %t

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
