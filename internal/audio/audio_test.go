package audio

import (
	"math"
	"os"
	"testing"
	"time"
)

// Bir sinüs dalgası kendi frekansının bandını en yükseğe çıkarmalı.
func TestBandsFindTheTone(t *testing.T) {
	a := newAnalyzer()
	const rate = 48000
	tone := func(hz float64) {
		s := make([]float64, fftSize)
		for i := range s {
			s[i] = 0.5 * math.Sin(2*math.Pi*hz*float64(i)/rate)
		}
		a.Push(s, rate)
	}
	peakBand := func() int {
		a.smooth = nil
		b := a.Bands(24)
		best := 0
		for i, v := range b {
			if v > b[best] {
				best = i
			}
		}
		if b[best] < 0.5 {
			t.Fatalf("tone too quiet: %v", b)
		}
		return best
	}
	tone(100)
	low := peakBand()
	tone(5000)
	high := peakBand()
	if low >= high || low > 6 || high < 16 {
		t.Fatalf("100 Hz in band %d, 5 kHz in band %d", low, high)
	}
}

func TestSilenceFallsToZero(t *testing.T) {
	a := newAnalyzer()
	a.Push(make([]float64, fftSize), 48000)
	for _, v := range a.Bands(16) {
		if v > 0.01 {
			t.Fatalf("silence shows %v", v)
		}
	}
	a.last = time.Now().Add(-time.Second)
	if !a.Silent() {
		t.Fatal("Silent false after a second without samples")
	}
}

// Gerçek ses aygıtı: SOTTO_LIVE=1 go test ./internal/audio -run Live -v
func TestLiveCapture(t *testing.T) {
	if os.Getenv("SOTTO_LIVE") == "" {
		t.Skip("SOTTO_LIVE=1")
	}
	v, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	time.Sleep(1500 * time.Millisecond)
	if err := v.Err(); err != nil {
		t.Fatal(err)
	}
	t.Logf("silent=%v bands=%.2f", v.Silent(), v.Bands(12))
}
