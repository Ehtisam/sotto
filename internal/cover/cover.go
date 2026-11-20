// Package cover, albüm ve liste kapaklarını indirir, diske saklar ve
// terminalde renkli yarım bloklarla (▀) çizer. Bu yöntem gerçek renkli her
// terminalde çalışır; her karakter üst üste iki piksel taşır.
package cover

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/jpeg" // Spotify kapakları JPEG
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Store, indirilen kapakları bellekte ve diskte tutar.
type Store struct {
	dir  string
	http *http.Client

	mu     sync.Mutex
	images map[string]image.Image
	drawn  map[string]string // url|en|boy|… -> çizilmiş metin
	sixels map[string]*sixelSet
}

func NewStore(dir string) *Store {
	return &Store{
		dir:    dir,
		http:   &http.Client{Timeout: 20 * time.Second},
		images: map[string]image.Image{},
		drawn:  map[string]string{},
		sixels: map[string]*sixelSet{},
	}
}

// Has, kapağın bellekte olup olmadığını söyler.
func (s *Store) Has(url string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.images[url]
	return ok
}

// Fetch, kapağı önce diskten, yoksa internetten alır ve belleğe koyar.
func (s *Store) Fetch(ctx context.Context, url string) error {
	if url == "" || s.Has(url) {
		return nil
	}
	path := filepath.Join(s.dir, key(url)+".img")
	data, err := os.ReadFile(path)
	if err != nil {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := s.http.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("cover: %s", resp.Status)
		}
		if data, err = io.ReadAll(io.LimitReader(resp.Body, 8<<20)); err != nil {
			return err
		}
		if err := os.MkdirAll(s.dir, 0o700); err == nil {
			_ = os.WriteFile(path, data, 0o600)
		}
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.images[url] = img
	s.mu.Unlock()
	return nil
}

// Render, kapağı cols sütun, rows satırlık kutuya yarım bloklarla çizer.
// ratio, hücrenin gerçek en/boy oranı (genişlik / yükseklik); kapak bu orana
// göre gerçekten kare görünecek boyutta ortalanır, kalan yer bg ile dolar.
// Kapak henüz yoksa "" döner.
func (s *Store) Render(url string, cols, rows int, ratio float64, bg [3]uint8) string {
	if url == "" || cols <= 0 || rows <= 0 {
		return ""
	}
	k := fmt.Sprintf("%s|%d|%d|%.3f|%v", url, cols, rows, ratio, bg)
	s.mu.Lock()
	defer s.mu.Unlock()
	if out, ok := s.drawn[k]; ok {
		return out
	}
	img, ok := s.images[url]
	if !ok {
		return ""
	}
	// Bir hücrede üst üste iki piksel: pikselin gerçek oranı 2*ratio.
	iw, ih := FitSquare(cols, rows*2, 2*ratio)
	out := halfBlocks(fitGrid(img, cols, rows*2, iw, ih, bg), cols, rows)
	s.drawn[k] = out
	return out
}

// FitSquare, w x h piksellik alana gerçekte kare görünen en büyük resmin
// boyutu. pixelRatio, bir pikselin gerçek en/boy oranı.
func FitSquare(w, h int, pixelRatio float64) (iw, ih int) {
	if pixelRatio <= 0 {
		pixelRatio = 1
	}
	iw, ih = w, int(float64(w)*pixelRatio+0.5)
	if ih > h {
		ih = h
		iw = int(float64(h)/pixelRatio + 0.5)
	}
	return max(1, min(iw, w)), max(1, min(ih, h))
}

// fitGrid, gw x gh piksellik zemine (bg) iw x ih boyutunda küçültülmüş resmi ortalar.
func fitGrid(img image.Image, gw, gh, iw, ih int, bg [3]uint8) [][][3]uint8 {
	px := average(img, iw, ih)
	ox, oy := (gw-iw)/2, (gh-ih)/2
	out := make([][][3]uint8, gh)
	for y := range out {
		out[y] = make([][3]uint8, gw)
		for x := range out[y] {
			out[y][x] = bg
			if ix, iy := x-ox, y-oy; ix >= 0 && ix < iw && iy >= 0 && iy < ih {
				out[y][x] = px[iy][ix]
			}
		}
	}
	return out
}

// halfBlocks, cols x (rows*2) piksellik ızgarayı, her hücreye üstte bir,
// altta bir piksel düşecek şekilde ▀ ile çizer.
func halfBlocks(px [][][3]uint8, cols, rows int) string {
	var b strings.Builder
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			t, u := px[2*y][x], px[2*y+1][x]
			fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀", t[0], t[1], t[2], u[0], u[1], u[2])
		}
		b.WriteString("\x1b[0m")
		if y < rows-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// average, kutu süzgeciyle küçültür: her hedef piksel, kaynaktaki karşılık
// gelen alanın ortalama rengidir. Küçük boyutlarda en temiz sonucu verir.
func average(img image.Image, w, h int) [][][3]uint8 {
	bnd := img.Bounds()
	sw, sh := bnd.Dx(), bnd.Dy()
	out := make([][][3]uint8, h)
	for y := 0; y < h; y++ {
		out[y] = make([][3]uint8, w)
		y0, y1 := bnd.Min.Y+y*sh/h, bnd.Min.Y+(y+1)*sh/h
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < w; x++ {
			x0, x1 := bnd.Min.X+x*sw/w, bnd.Min.X+(x+1)*sw/w
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, bl, n uint64
			// Büyük kapaklarda her pikseli okumak gereksiz; en çok 8x8 örnek yeter.
			stepX, stepY := max(1, (x1-x0)/8), max(1, (y1-y0)/8)
			for yy := y0; yy < y1; yy += stepY {
				for xx := x0; xx < x1; xx += stepX {
					cr, cg, cb, _ := img.At(xx, yy).RGBA()
					r, g, bl, n = r+uint64(cr>>8), g+uint64(cg>>8), bl+uint64(cb>>8), n+1
				}
			}
			out[y][x] = [3]uint8{uint8(r / n), uint8(g / n), uint8(bl / n)}
		}
	}
	return out
}

func key(url string) string {
	h := sha1.Sum([]byte(url))
	return hex.EncodeToString(h[:])
}

// File, kapağın diskteki kopyasını .jpg adıyla verir (bildirimlerde
// kullanılır); gerekirse önce indirir.
func (s *Store) File(ctx context.Context, url string) (string, error) {
	if url == "" {
		return "", fmt.Errorf("cover: no url")
	}
	jpg := filepath.Join(s.dir, key(url)+".jpg")
	if _, err := os.Stat(jpg); err == nil {
		return jpg, nil
	}
	if err := s.Fetch(ctx, url); err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(s.dir, key(url)+".img"))
	if err != nil {
		return "", err
	}
	return jpg, os.WriteFile(jpg, data, 0o600)
}
