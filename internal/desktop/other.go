//go:build !windows

package desktop

import (
	"os/exec"
	"time"
)

// SetDataDir: yalnızca Windows'ta kullanılır.
func SetDataDir(string) {}

// Registered: Windows dışında kayıt gerekmez.
func Registered() bool { return true }

// Notify, notify-send ile bildirim gösterir (Linux; yoksa sessizce geçer).
func Notify(n Note) error {
	path, err := exec.LookPath("notify-send")
	if err != nil {
		return ErrUnsupported
	}
	args := []string{"-a", "sotto", "-h", "string:x-canonical-private-synchronous:sotto"}
	if n.Image != "" {
		args = append(args, "-i", n.Image)
	}
	args = append(args, n.Title, n.Body)
	return exec.Command(path, args...).Run()
}

// Media: bu sistemde medya denetimi yok.
type Media struct{}

func OpenMedia() (*Media, error)                      { return nil, ErrUnsupported }
func (m *Media) Events() <-chan Event                 { return nil }
func (m *Media) SetTrack(Track) error                 { return nil }
func (m *Media) SetStatus(Status) error               { return nil }
func (m *Media) SetPosition(_, _ time.Duration) error { return nil }
func (m *Media) Close()                               {}

// Başlat menüsü kısayolu yalnızca Windows'ta.
func InstallShortcut(string) error                    { return ErrUnsupported }
func RemoveShortcut() error                           { return nil }
func ShortcutInfo() (target, appID string, err error) { return "", "", ErrUnsupported }
func ShortcutOK(string) bool                          { return true }

// RefreshIcons: yalnızca Windows.
func RefreshIcons() bool { return false }

// CleanupOld: yalnızca Windows.
func CleanupOld() {}

// ReadClipboard, panodaki metni okur (pbpaste, wl-paste, xclip ya da xsel).
func ReadClipboard() (string, error) {
	for _, c := range [][]string{{"pbpaste"}, {"wl-paste", "--no-newline"}, {"xclip", "-o", "-selection", "clipboard"}, {"xsel", "-ob"}} {
		if _, err := exec.LookPath(c[0]); err != nil {
			continue
		}
		out, err := exec.Command(c[0], c[1:]...).Output()
		if err == nil {
			return string(out), nil
		}
	}
	return "", ErrUnsupported
}
