//go:build !windows

package desktop

import (
	"os/exec"
	"strings"
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

// Medya kartı ve menü kısayolu: Linux'ta media_linux.go ve shortcut_linux.go,
// diğerlerinde other_nolinux.go.

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

// WriteClipboard, metni panoya yazar (pbcopy, wl-copy, xclip ya da xsel).
func WriteClipboard(text string) error {
	for _, c := range [][]string{{"pbcopy"}, {"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "-ib"}} {
		if _, err := exec.LookPath(c[0]); err != nil {
			continue
		}
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}
	return ErrUnsupported
}
