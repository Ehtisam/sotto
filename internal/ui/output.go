package ui

import (
	"bytes"
	"os"
	"sync"

	"github.com/charmbracelet/x/ansi"
)

// Output, sotto'nun terminale yazdığı her şeyi sarar. Bubble Tea her kareyi
// tek seferde yazar; Output bu yazının sonuna, eşzamanlı çıktının kapanışından
// önce, o karede silinecek resim şeritlerini ekler (gfx.go, "after"). Böylece
// terminal satırın silinmesini ve şeridin geri gelmesini aynı anda gösterir,
// arada titreme olmaz.
//
// Bubble Tea'nin terminali tanıyıp ayarlayabilmesi için os.File gibi
// davranır (Read, Close, Fd).
type Output struct {
	f *os.File

	mu    sync.Mutex
	after string
	off   bool // alternatif ekrandan çıkıldı: normal ekrana resim basılmasın
}

func NewOutput(f *os.File) *Output { return &Output{f: f} }

// stop, bundan sonra hiçbir yazıya şerit eklenmesin (çıkarken).
func (o *Output) stop() {
	if o == nil {
		return
	}
	o.mu.Lock()
	o.off, o.after = true, ""
	o.mu.Unlock()
}

// setAfter, her yazının sonuna eklenecek diziyi değiştirir.
func (o *Output) setAfter(s string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	o.after = s
	o.mu.Unlock()
}

func (o *Output) Write(p []byte) (int, error) {
	o.mu.Lock()
	// Çıkışta Bubble Tea alternatif ekrandan çıkar; o yazıya ve sonrakilere
	// şerit eklenirse kabuğun ekranına basılır.
	switch {
	case bytes.Contains(p, []byte(ansi.ResetModeAltScreenSaveCursor)):
		o.off = true
	case bytes.Contains(p, []byte(ansi.SetModeAltScreenSaveCursor)):
		o.off = false
		o.mu.Unlock()
		return o.f.Write(p)
	}
	after := o.after
	if o.off {
		after = ""
	}
	o.mu.Unlock()
	if after == "" || len(p) == 0 {
		return o.f.Write(p)
	}
	_, err := o.f.Write(withAfter(p, after))
	return len(p), err
}

// withAfter, diziyi eşzamanlı çıktının kapanışından önceye, yoksa sona koyar.
func withAfter(p []byte, after string) []byte {
	end := []byte(ansi.ResetModeSynchronizedOutput)
	out := make([]byte, 0, len(p)+len(after))
	if bytes.HasSuffix(p, end) {
		out = append(out, p[:len(p)-len(end)]...)
		out = append(out, after...)
		return append(out, end...)
	}
	out = append(out, p...)
	return append(out, after...)
}

func (o *Output) Read(p []byte) (int, error) { return o.f.Read(p) }
func (o *Output) Close() error               { return o.f.Close() }
func (o *Output) Fd() uintptr                { return o.f.Fd() }
