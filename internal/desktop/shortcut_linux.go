//go:build linux

package desktop

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// dataHome: $XDG_DATA_HOME ya da ~/.local/share.
func dataHome() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share")
}

func desktopFile() string { return filepath.Join(dataHome(), "applications", "sotto.desktop") }

func iconFile() string {
	return filepath.Join(dataHome(), "icons", "hicolor", "256x256", "apps", "sotto.png")
}

// InstallShortcut, uygulama menüsüne sotto'yu (simgesiyle) koyar.
func InstallShortcut(target string) error {
	for _, d := range []string{filepath.Dir(desktopFile()), filepath.Dir(iconFile())} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	if _, err := writeIfChanged(iconFile(), IconPNG(256)); err != nil {
		return err
	}
	return os.WriteFile(desktopFile(), []byte(desktopEntry(target)), 0o644)
}

// RemoveShortcut, menüdeki sotto'yu siler.
func RemoveShortcut() error {
	for _, p := range []string{desktopFile(), iconFile()} {
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

// ShortcutInfo: menüdeki kısayolun açtığı program.
func ShortcutInfo() (target, appID string, err error) {
	b, err := os.ReadFile(desktopFile())
	if err != nil {
		return "", "", err
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "Exec=") {
			return execTarget(line), "sotto", nil
		}
	}
	return "", "", errors.New("sotto.desktop has no Exec line")
}

// ShortcutOK: kısayol var, bu programı açıyor ve simgesi yerinde.
func ShortcutOK(target string) bool {
	b, err := os.ReadFile(desktopFile())
	if err != nil || string(b) != desktopEntry(target) {
		return false
	}
	_, err = os.Stat(iconFile())
	return err == nil
}
