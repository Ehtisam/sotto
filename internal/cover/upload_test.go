package cover

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

func TestCoverJPEG(t *testing.T) {
	// Gürültülü, büyük, kare olmayan bir PNG: kırpılmalı, küçülmeli, sığmalı.
	src := image.NewRGBA(image.Rect(0, 0, 1600, 1000))
	r := rand.New(rand.NewSource(1))
	r.Read(src.Pix)
	path := filepath.Join(t.TempDir(), "big.png")
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := CoverJPEG(`  "` + path + `" `)
	if err != nil {
		t.Fatal(err)
	}
	if (len(out)+2)/3*4 > maxCoverBase64 {
		t.Fatalf("too big: %d bytes", len(out))
	}
	img, err := jpeg.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 640 || b.Dy() != 640 {
		t.Fatalf("size %v", b)
	}
	if _, err := CoverJPEG(filepath.Join(t.TempDir(), "missing.jpg")); err == nil {
		t.Fatal("missing file should fail")
	}
}
