// Package setup, ilk kurulumda gereken yardımcı programları resmî
// adreslerinden indirir: Spotify Soloist (hafif mod) ve go-librespot (✱).
//
// Soloist dağıtılamaz; sotto onu içinde taşımaz, kullanıcının onayıyla
// Spotify'ın kendi adresinden indirir. Belge:
// https://developer.spotify.com/documentation/soloist/reference/downloads-and-updates
package setup

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
)

// ErrUnsupported: bu işletim sistemi / işlemci için resmî sürüm yok.
var ErrUnsupported = errors.New("no official build for this system")

// LibrespotVersion, indirilen go-librespot sürümü.
const LibrespotVersion = "v0.10.7"

// SoloistURL, bu sistem için Soloist'in resmî indirme adresi.
func SoloistURL() (string, error) {
	const base = "https://soloist-builds.spotifycdn.com/"
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "windows/amd64":
		return base + "soloist_release_windows_x86_64.zip", nil
	case "linux/amd64":
		return base + "soloist_release_x86_64.tar.gz", nil
	case "linux/arm64":
		return base + "soloist_release_arm64.tar.gz", nil
	case "linux/arm":
		return base + "soloist_release_arm32.tar.gz", nil
	case "darwin/arm64":
		return base + "soloist_release_osx_arm64.tar.gz", nil
	}
	return "", ErrUnsupported
}

// LibrespotURL, bu sistem için go-librespot'un resmî sürüm dosyası (GitHub).
func LibrespotURL() (string, error) {
	base := "https://github.com/devgianlu/go-librespot/releases/download/" + LibrespotVersion + "/"
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "windows/amd64":
		return base + "go-librespot_windows_amd64.tar.gz", nil
	case "linux/amd64":
		return base + "go-librespot_linux_x86_64.tar.gz", nil
	case "linux/arm64":
		return base + "go-librespot_linux_arm64.tar.gz", nil
	}
	return "", ErrUnsupported
}

// Progress, süren bir indirmenin durumu; arayüz arada bir okur.
type Progress struct {
	Done, Total atomic.Int64
}

// Fetch, url'deki arşivi indirir, içinden adı name olan programı çıkarıp
// dest'e yazar (varsa yenisiyle değiştirir). Arşiv .zip ya da .tar.gz olabilir.
func Fetch(ctx context.Context, url, name, dest string, p *Progress) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download: %s", resp.Status)
	}
	if p != nil {
		p.Total.Store(resp.ContentLength)
	}

	// Önce geçici dosyaya: zip okumak için baştan sona erişim gerekiyor.
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), "download-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	var body io.Reader = resp.Body
	if p != nil {
		body = &counter{r: resp.Body, p: p}
	}
	if _, err := io.Copy(tmp, body); err != nil {
		return err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return err
	}

	var exe io.Reader
	switch {
	case strings.HasSuffix(url, ".zip"):
		st, err := tmp.Stat()
		if err != nil {
			return err
		}
		zr, err := zip.NewReader(tmp, st.Size())
		if err != nil {
			return err
		}
		for _, f := range zr.File {
			if path.Base(f.Name) == name && !f.FileInfo().IsDir() {
				rc, err := f.Open()
				if err != nil {
					return err
				}
				defer rc.Close()
				exe = rc
				break
			}
		}
	case strings.HasSuffix(url, ".tar.gz"):
		gz, err := gzip.NewReader(tmp)
		if err != nil {
			return err
		}
		defer gz.Close()
		tr := tar.NewReader(gz)
		for {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			if path.Base(h.Name) == name && h.Typeflag == tar.TypeReg {
				exe = tr
				break
			}
		}
	default:
		return fmt.Errorf("download: unknown archive %s", path.Base(url))
	}
	if exe == nil {
		return fmt.Errorf("download: %s not in the archive", name)
	}
	return writeExe(dest, exe)
}

// writeExe, programı önce yanına yazar, sonra yerine taşır: yarım kalan bir
// indirme çalışan sürümü bozmasın.
func writeExe(dest string, r io.Reader) error {
	part := dest + ".part"
	f, err := os.OpenFile(part, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		os.Remove(part)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(part)
		return err
	}
	return os.Rename(part, dest)
}

type counter struct {
	r io.Reader
	p *Progress
}

func (c *counter) Read(b []byte) (int, error) {
	n, err := c.r.Read(b)
	c.p.Done.Add(int64(n))
	return n, err
}

// ExeName, programın bu işletim sistemindeki dosya adı.
func ExeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}
