package cover

import (
	"bytes"
	"fmt"
	"image"
	"image/color"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/sixel"
)

// Gerçek resim: Sixel destekleyen terminallerde (Windows Terminal, WezTerm,
// foot, Konsole…) kapak piksel piksel çizilir. Çevirmek birkaç yüz ms
// sürebildiği için ayrı adımda yapılır ve sonucu saklanır.

// Spec, bir Sixel çiziminin tarifi: W x H piksellik zemin (BG) üstüne
// ortalanmış IW x IH boyutunda kapak. Çizim RowH piksellik şeritlere de
// bölünür: terminal bir satırı silerse yalnızca o şerit yeniden gönderilir.
type Spec struct {
	URL          string
	W, H, IW, IH int
	RowH         int
	BG           [3]uint8
}

func (sp Spec) key() string {
	return fmt.Sprintf("sixel|%s|%d|%d|%d|%d|%d|%v", sp.URL, sp.W, sp.H, sp.IW, sp.IH, sp.RowH, sp.BG)
}

type sixelSet struct {
	full   string
	strips []string // satır satır
}

// SixelReady, çevrilmiş çizim; yoksa ok=false.
func (s *Store) SixelReady(sp Spec) (full string, strips []string, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	set, ok := s.sixels[sp.key()]
	if !ok {
		return "", nil, false
	}
	return set.full, set.strips, true
}

// BuildSixel, kapağı tarife göre çizip Sixel dizisine çevirir ve saklar.
// Kapak henüz indirilmemişse bir şey yapmaz.
func (s *Store) BuildSixel(sp Spec) error {
	if _, _, ok := s.SixelReady(sp); ok || sp.W <= 0 || sp.H <= 0 || sp.RowH <= 0 {
		return nil
	}
	s.mu.Lock()
	img, ok := s.images[sp.URL]
	s.mu.Unlock()
	if !ok {
		return nil
	}
	grid := fitGrid(img, sp.W, sp.H, sp.IW, sp.IH, sp.BG)
	canvas := image.NewRGBA(image.Rect(0, 0, sp.W, sp.H))
	for y := 0; y < sp.H; y++ {
		for x := 0; x < sp.W; x++ {
			c := grid[y][x]
			canvas.SetRGBA(x, y, color.RGBA{c[0], c[1], c[2], 255})
		}
	}
	set := &sixelSet{}
	var err error
	if set.full, err = encode(canvas); err != nil {
		return err
	}
	for y0 := 0; y0 < sp.H; y0 += sp.RowH {
		strip, err := encode(canvas.SubImage(image.Rect(0, y0, sp.W, min(y0+sp.RowH, sp.H))))
		if err != nil {
			return err
		}
		set.strips = append(set.strips, strip)
	}
	s.mu.Lock()
	s.sixels[sp.key()] = set
	s.mu.Unlock()
	return nil
}

func encode(img image.Image) (string, error) {
	// Kodlayıcı resmin 0,0'dan başladığını varsayıyor; alt resmi kopyala.
	b := img.Bounds()
	flat := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			flat.Set(x, y, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	var buf bytes.Buffer
	if err := (&sixel.Encoder{}).Encode(&buf, flat); err != nil {
		return "", err
	}
	// p2=1: işlenmemiş pikseller arkadakini göstersin; şeridin son 6'lık
	// bandı bir alttaki satıra taşmasın.
	return ansi.SixelGraphics(0, 1, 0, buf.Bytes()), nil
}

// Put, bir resmi doğrudan belleğe koyar (testler için).
func (s *Store) Put(url string, img image.Image) {
	s.mu.Lock()
	s.images[url] = img
	s.mu.Unlock()
}
