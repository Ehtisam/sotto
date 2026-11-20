// Package config, sotto'nun ayar dosyasını ve klasörlerini yönetir.
//
// Windows'ta:
//
//	%APPDATA%\sotto\config.toml     ayarlar
//	%APPDATA%\sotto\soloist.key     Soloist API anahtarı (gizli)
//	%LOCALAPPDATA%\sotto\           Soloist, önbellek, kayıtlar
package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	Soloist Soloist `toml:"soloist"`

	// Doldurulan yollar (dosyada yok).
	ConfigDir string `toml:"-"`
	LocalDir  string `toml:"-"`
}

type Soloist struct {
	// Enabled: sotto açılınca Soloist'i başlat (hafif mod).
	Enabled bool `toml:"enabled"`
	// Path: soloist(.exe). Boşsa LocalDir\soloist\ içinde aranır.
	Path          string `toml:"path"`
	DeviceName    string `toml:"device_name"`
	InitialVolume int    `toml:"initial_volume"`
}

func defaults() Config {
	return Config{Soloist: Soloist{Enabled: true, DeviceName: "sotto", InitialVolume: 40}}
}

// Load, ayar dosyasını okur; yoksa varsayılanları döner.
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
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return c, err
	default:
		if err := toml.Unmarshal(b, &c); err != nil {
			return c, err
		}
	}
	if c.Soloist.DeviceName == "" {
		c.Soloist.DeviceName = "sotto"
	}
	return c, nil
}

func (c Config) ConfigFile() string { return filepath.Join(c.ConfigDir, "config.toml") }

// SoloistKeyFile, Soloist API anahtarının durduğu dosya.
func (c Config) SoloistKeyFile() string { return filepath.Join(c.ConfigDir, "soloist.key") }

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

// SoloistAPIKey, önce SOLOIST_API_KEY ortam değişkenine, sonra anahtar dosyasına bakar.
func (c Config) SoloistAPIKey() string {
	if k := strings.TrimSpace(os.Getenv("SOLOIST_API_KEY")); k != "" {
		return k
	}
	b, err := os.ReadFile(c.SoloistKeyFile())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(string(b), "ï»¿")) // BOM
}
