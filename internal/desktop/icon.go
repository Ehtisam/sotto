package desktop

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

// sotto simgesi: koyu zeminde altın sarısı komut işareti (❯) ve üç mor ses
// çubuğu. 64×64'lük bir tuvalde tanımlıdır (assets/sotto.svg ile aynı); her
// boyutta yeniden, kenarları yumuşatılarak çizilir.

var (
	iconGround = color.NRGBA{0x0e, 0x0c, 0x0a, 0xff}
	iconGold   = color.NRGBA{0xd9, 0xa4, 0x41, 0xff}
	iconViolet = color.NRGBA{0x9d, 0x6e, 0xe0, 0xff}
	iconLilac  = color.NRGBA{0xb8, 0x92, 0xec, 0xff}
)

type iconBar struct {
	x, y, w, h float64
	c          color.NRGBA
}

var iconBars = []iconBar{
	{29, 34, 5, 10, iconViolet},
	{37, 24, 5, 20, iconLilac},
	{45, 29, 5, 15, iconViolet},
}

// iconChevron: komut işaretinin kırık çizgisi ve kalınlığı.
var iconChevron = [][2]float64{{13, 22}, {22, 31}, {13, 40}}

const iconStroke = 4.5

// inRoundRect: (x, y) köşeleri r yarıçaplı dikdörtgenin içinde mi.
func inRoundRect(x, y, x0, y0, w, h, r float64) bool {
	if x < x0 || y < y0 || x > x0+w || y > y0+h {
		return false
	}
	cx := math.Max(x0+r, math.Min(x, x0+w-r))
	cy := math.Max(y0+r, math.Min(y, y0+h-r))
	return (x-cx)*(x-cx)+(y-cy)*(y-cy) <= r*r
}

// segDist: (x, y)'nin a-b doğru parçasına uzaklığı.
func segDist(x, y float64, a, b [2]float64) float64 {
	dx, dy := b[0]-a[0], b[1]-a[1]
	t := ((x-a[0])*dx + (y-a[1])*dy) / (dx*dx + dy*dy)
	t = math.Max(0, math.Min(1, t))
	px, py := a[0]+t*dx, a[1]+t*dy
	return math.Hypot(x-px, y-py)
}

// glyphCenter: ok ve çubukların ortası (küçük boyutlarda buradan büyütülür).
const glyphCX, glyphCY = 30.4, 32

// glyphScale: küçük boyutlarda şekil büyütülür, kenar boşluğu azalır; yoksa
// 16 pikselde ok ve çubuklar birkaç piksele düşüp kaybolur.
func glyphScale(n int) float64 {
	switch {
	case n <= 20:
		return 1.45
	case n <= 32:
		return 1.3
	case n <= 48:
		return 1.15
	}
	return 1
}

// iconAt: 64'lük tuvalde (x, y) noktasının rengi; saydamsa ok=false. scale:
// şeklin (zemin değil) büyütülmesi.
func iconAt(x, y, scale float64) (color.NRGBA, bool) {
	if !inRoundRect(x, y, 0, 0, 64, 64, 12) {
		return color.NRGBA{}, false
	}
	x = glyphCX + (x-glyphCX)/scale
	y = glyphCY + (y-glyphCY)/scale
	for i := 0; i+1 < len(iconChevron); i++ {
		if segDist(x, y, iconChevron[i], iconChevron[i+1]) <= iconStroke/2 {
			return iconGold, true
		}
	}
	for _, b := range iconBars {
		if inRoundRect(x, y, b.x, b.y, b.w, b.h, 1) {
			return b.c, true
		}
	}
	return iconGround, true
}

// drawIcon, simgeyi n×n çizer (her piksel 4×4 örnekle yumuşatılır).
func drawIcon(n int) image.Image { return drawIconScaled(n, glyphScale(n)) }

// drawIconScaled, simgeyi n×n, şekli gs kat büyütülmüş çizer.
func drawIconScaled(n int, gs float64) image.Image {
	const ss = 4
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	scale := 64 / float64(n)
	for py := 0; py < n; py++ {
		for px := 0; px < n; px++ {
			var r, g, b, a float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					x := (float64(px) + (float64(sx)+0.5)/ss) * scale
					y := (float64(py) + (float64(sy)+0.5)/ss) * scale
					if c, ok := iconAt(x, y, gs); ok {
						r += float64(c.R)
						g += float64(c.G)
						b += float64(c.B)
						a++
					}
				}
			}
			if a == 0 {
				continue
			}
			img.SetNRGBA(px, py, color.NRGBA{
				uint8(r/a + 0.5), uint8(g/a + 0.5), uint8(b/a + 0.5),
				uint8(255*a/(ss*ss) + 0.5),
			})
		}
	}
	return img
}

// IconPNG, simgenin n×n PNG hali.
// NotifyIconPNG, bildirimlerin simgesi (64 px): Windows onu 16–20 piksele
// küçülterek gösterdiği için şekil küçük boyutlardaki gibi büyütülmüş çizilir.
func NotifyIconPNG() []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, drawIconScaled(64, glyphScale(16)))
	return b.Bytes()
}

func IconPNG(n int) []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, drawIcon(n))
	return b.Bytes()
}

// IconSizes, .ico dosyasına konan boyutlar.
var IconSizes = []int{16, 20, 24, 32, 40, 48, 64, 128, 256}

// IconICO, bütün boyutları içeren .ico dosyası (her boyut PNG olarak).
func IconICO() []byte {
	var pngs [][]byte
	for _, n := range IconSizes {
		pngs = append(pngs, IconPNG(n))
	}
	var b bytes.Buffer
	le := binary.LittleEndian
	binary.Write(&b, le, [3]uint16{0, 1, uint16(len(pngs))})
	off := 6 + 16*len(pngs)
	for i, p := range pngs {
		n := IconSizes[i]
		dim := byte(n)
		if n >= 256 {
			dim = 0
		}
		b.Write([]byte{dim, dim, 0, 0})
		binary.Write(&b, le, uint16(1))
		binary.Write(&b, le, uint16(32))
		binary.Write(&b, le, uint32(len(p)))
		binary.Write(&b, le, uint32(off))
		off += len(p)
	}
	for _, p := range pngs {
		b.Write(p)
	}
	return b.Bytes()
}

// writeIfChanged, dosyayı içeriği farklıysa yazar; yazdıysa true.
func writeIfChanged(path string, data []byte) (bool, error) {
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, data) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, data, 0o600)
}
