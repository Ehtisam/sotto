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
	Spotify  Spotify `toml:"spotify"`
	Soloist  Soloist `toml:"soloist"`

	// Doldurulan yollar (dosyada yok).
	ConfigDir string `toml:"-"`
	LocalDir  string `toml:"-"`
}

type Spotify struct {
	ClientID string `toml:"client_id"`
}

type Soloist struct {
	Autostart     bool   `toml:"autostart"`
	APIKey        string `toml:"api_key"`
	Path          string `toml:"path"`
	DeviceName    string `toml:"device_name"`
	InitialVolume int    `toml:"initial_volume"`

	// Eski sürümlerden kalan ad; okunur, yazılmaz.
	Enabled *bool `toml:"enabled"`
}

func defaults() Config {
	return Config{Language: "en", Soloist: Soloist{Autostart: true, DeviceName: "sotto", InitialVolume: 40}}
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
		quote(c.Spotify.ClientID),
		c.Soloist.Autostart,
		quote(c.Soloist.APIKey),
		quote(c.Soloist.Path),
		quote(c.Soloist.DeviceName),
		c.Soloist.InitialVolume,
	)
	return os.WriteFile(c.ConfigFile(), []byte(text), 0o600)
}

const template = `# sotto settings
# Everything sotto can be configured with lives in this file.

# Interface language: "en" or "tr".
language = %s

[spotify]
# Client ID of your own Spotify app (https://developer.spotify.com/dashboard).
# The app needs the redirect URI http://127.0.0.1:8765/callback and the Web API.
client_id = %s

[soloist]
# Light mode plays through Spotify Soloist on this computer.
# Start Soloist when sotto opens. You can also start and stop it with "o".
autostart = %t
# Soloist API key (https://developer.spotify.com/dashboard/soloist). Keep it private.
api_key = %s
# Leave empty to use the copy sotto keeps in its own folder.
path = %s
# Name shown in Spotify's device list.
device_name = %s
initial_volume = %d
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
