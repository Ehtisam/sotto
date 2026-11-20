package soloist

import (
	"context"
	"errors"
	"os/exec"
	"regexp"
	"strconv"
	"time"

	"github.com/Ehtisam/sotto/internal/proc"
)

// Soloist sürümleri derlendikten 90 gün sonra çalışmayı bırakır (çıkış kodu 10);
// yenisi resmî adresten indirilir.
const Lifetime = 90 * 24 * time.Hour

// Build, kurulu Soloist sürümü.
type Build struct {
	Version string
	Built   time.Time
}

// Expires, sürümün çalışmayı bırakacağı an.
func (b Build) Expires() time.Time { return b.Built.Add(Lifetime) }

// DaysLeft, süresinin dolmasına kalan gün (dolduysa 0 ya da eksi).
func (b Build) DaysLeft(now time.Time) int {
	return int(b.Expires().Sub(now).Hours() / 24)
}

var versionRE = regexp.MustCompile(`soloist (\S+) build (\d+)(?: \((\d{8})\))?`)

// ParseVersion, "soloist 1.3.10.87 build 1795111339 (20261119) …" satırını okur.
func ParseVersion(s string) (Build, error) {
	m := versionRE.FindStringSubmatch(s)
	if m == nil {
		return Build{}, errors.New("soloist: unknown version output")
	}
	b := Build{Version: m[1]}
	if n, err := strconv.ParseInt(m[2], 10, 64); err == nil && n > 1_600_000_000 && n < 4_000_000_000 {
		b.Built = time.Unix(n, 0)
	} else if m[3] != "" {
		b.Built, _ = time.ParseInLocation("20060102", m[3], time.Local)
	}
	if b.Built.IsZero() {
		return b, errors.New("soloist: no build date")
	}
	return b, nil
}

// ReadBuild, verilen Soloist programının sürümünü sorar.
func ReadBuild(ctx context.Context, path string) (Build, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version")
	proc.Detach(cmd)
	out, err := cmd.Output()
	if b, perr := ParseVersion(string(out)); perr == nil {
		return b, nil
	}
	if err != nil {
		return Build{}, err
	}
	return Build{}, errors.New("soloist: unknown version output")
}
