package cover

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
)

// Spotify kapak olarak en çok 256 KB'lık base64 JPEG kabul ediyor.
const maxCoverBase64 = 256 * 1000

// CoverJPEG, bir resim dosyasını (JPEG ya da PNG) liste kapağına çevirir:
// ortasından kare kırpar, 640 pikseli geçmeyecek şekilde küçültür ve base64
// hali 256 KB'ı geçmeyecek kalitede JPEG olarak kodlar.
func CoverJPEG(path string) ([]byte, error) {
	data, err := os.ReadFile(CleanPath(path))
	if err != nil {
		return nil, err
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	side := min(b.Dx(), b.Dy())
	if side < 1 {
		return nil, errors.New("cover: empty image")
	}
	x0, y0 := b.Min.X+(b.Dx()-side)/2, b.Min.Y+(b.Dy()-side)/2
	sq := image.Rectangle{Min: image.Pt(x0, y0), Max: image.Pt(x0+side, y0+side)}
	type sub interface {
		SubImage(image.Rectangle) image.Image
	}
	var crop image.Image = src
	if s, ok := src.(sub); ok {
		crop = s.SubImage(sq)
	}
	out := min(side, 640)
	px := average(crop, out, out)
	img := image.NewRGBA(image.Rect(0, 0, out, out))
	for y := range out {
		for x := range out {
			c := px[y][x]
			i := img.PixOffset(x, y)
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c[0], c[1], c[2], 0xff
		}
	}
	for q := 90; q >= 30; q -= 10 {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: q}); err != nil {
			return nil, err
		}
		if (buf.Len()+2)/3*4 <= maxCoverBase64 {
			return buf.Bytes(), nil
		}
	}
	return nil, errors.New("cover: image too large")
}

// CleanPath, yapıştırılmış bir dosya yolundaki tırnakları ve boşlukları
// atar, ~'yi ev klasörüne çevirir.
func CleanPath(p string) string {
	p = strings.Trim(strings.TrimSpace(p), `"'`)
	if strings.HasPrefix(p, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, p[1:])
		}
	}
	return p
}
