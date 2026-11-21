// Package audio, ses görselleştirmesi için bilgisayarda çalan sesi dinler
// (Windows'ta WASAPI loopback: varsayılan hoparlöre giden ses) ve frekans
// bantlarına ayırır. Soloist ham ses vermediği için tek yol budur; yakalanan
// ses bütün sistemin sesidir (başka programlar da çalıyorsa onlar da görünür).
package audio

import (
	"errors"
	"math"
	"math/cmplx"
	"sync"
	"time"
)

// ErrUnsupported: bu sistemde ses yakalanamıyor.
var ErrUnsupported = errors.New("audio capture is not supported on this system")

// fftSize: çözümlenen örnek sayısı (48 kHz'de ~43 ms).
const fftSize = 2048

// Analyzer, gelen örnekleri tutar ve bantlara ayırır. Yakalayıcı Push ile
// örnek yazar, arayüz Bands ile okur.
type Analyzer struct {
	mu     sync.Mutex
	ring   [fftSize]float64
	pos    int
	rate   int
	last   time.Time // son örneğin geldiği an
	smooth []float64
	fast   []float64 // BandsFast'ın yumuşatması
}

func newAnalyzer() *Analyzer { return &Analyzer{rate: 48000} }

// Push, tek kanala indirilmiş örnekleri ekler.
func (a *Analyzer) Push(samples []float64, rate int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if rate > 0 {
		a.rate = rate
	}
	for _, s := range samples {
		a.ring[a.pos] = s
		a.pos = (a.pos + 1) % fftSize
	}
	a.last = time.Now()
}

// Silent: bir süredir ses gelmiyor.
func (a *Analyzer) Silent() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return time.Since(a.last) > 400*time.Millisecond
}

// Bands, n bandın 0–1 arası yüksekliği (40 Hz – 16 kHz, logaritmik aralıklı).
// Yükselişler hemen, düşüşler yavaşça görünür.
func (a *Analyzer) Bands(n int) []float64 { return a.bands(n, &a.smooth, 0.06) }

// BandsFast, Bands gibi ama düşüşler hızlı (tepe çizgili görünümler için:
// çubuk iner, tepe bir süre asılı kalır).
func (a *Analyzer) BandsFast(n int) []float64 { return a.bands(n, &a.fast, 0.22) }

func (a *Analyzer) bands(n int, smooth *[]float64, fall float64) []float64 {
	if n <= 0 {
		return nil
	}
	a.mu.Lock()
	buf := make([]complex128, fftSize)
	for i := 0; i < fftSize; i++ {
		s := a.ring[(a.pos+i)%fftSize]
		w := 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(fftSize-1)) // Hann
		buf[i] = complex(s*w, 0)
	}
	rate := a.rate
	quiet := time.Since(a.last) > 400*time.Millisecond
	if len(*smooth) != n {
		*smooth = make([]float64, n)
	}
	a.mu.Unlock()

	out := make([]float64, n)
	if !quiet {
		fft(buf)
		lo, hi := 40.0, math.Min(16000, float64(rate)/2)
		binHz := float64(rate) / fftSize
		for b := 0; b < n; b++ {
			f0 := lo * math.Pow(hi/lo, float64(b)/float64(n))
			f1 := lo * math.Pow(hi/lo, float64(b+1)/float64(n))
			i0, i1 := int(f0/binHz), int(math.Ceil(f1/binHz))
			i1 = max(i1, i0+1)
			peak := 0.0
			for i := i0; i < i1 && i < fftSize/2; i++ {
				peak = math.Max(peak, cmplx.Abs(buf[i]))
			}
			// Genliği dB'ye çevir: -65 dB → 0, 0 dB → 1 (Hann penceresi ~yarıya indirir).
			db := 20 * math.Log10(peak/(fftSize/4)+1e-9)
			out[b] = math.Max(0, math.Min(1, (db+65)/65))
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	sm := *smooth
	for i := range out {
		if out[i] >= sm[i] {
			sm[i] = out[i]
		} else {
			sm[i] = math.Max(out[i], sm[i]-fall)
		}
		out[i] = sm[i]
	}
	return out
}

// fft, yerinde radix-2 hızlı Fourier dönüşümü (len(x) ikinin kuvveti).
func fft(x []complex128) {
	n := len(x)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		w := cmplx.Exp(complex(0, -2*math.Pi/float64(size)))
		for start := 0; start < n; start += size {
			wk := complex(1, 0)
			for k := 0; k < size/2; k++ {
				u, v := x[start+k], x[start+k+size/2]*wk
				x[start+k], x[start+k+size/2] = u+v, u-v
				wk *= w
			}
		}
	}
}

// Visualizer, çalışan bir yakalayıcı ve çözümleyicisi.
type Visualizer struct {
	*Analyzer
	stop chan struct{}
	done chan struct{}
	mu   sync.Mutex
	err  error
}

// Err, yakalamanın son hatası (nil: çalışıyor).
func (v *Visualizer) Err() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.err
}

func (v *Visualizer) setErr(err error) {
	v.mu.Lock()
	v.err = err
	v.mu.Unlock()
}

// Close, yakalamayı durdurur.
func (v *Visualizer) Close() {
	if v == nil {
		return
	}
	select {
	case <-v.stop:
	default:
		close(v.stop)
	}
	<-v.done
}

// Start, yakalamayı başlatır. Aygıt değişirse ya da kaybolursa kendisi
// yeniden dener.
func Start() (*Visualizer, error) {
	v := &Visualizer{Analyzer: newAnalyzer(), stop: make(chan struct{}), done: make(chan struct{})}
	first := make(chan error, 1)
	go func() {
		defer close(v.done)
		once := false
		for {
			err := capture(v, func(err error) {
				if !once {
					once = true
					first <- err
				}
			})
			v.setErr(err)
			select {
			case <-v.stop:
				return
			case <-time.After(time.Second): // aygıt değişti ya da hata: yeniden dene
			}
		}
	}()
	if err := <-first; err != nil {
		v.Close()
		return nil, err
	}
	return v, nil
}

// Fake, yakalamasız bir görselleştirici (başka paketlerin testleri için):
// verilen örneklerle doldurulmuş.
func Fake(samples []float64, rate int) *Visualizer {
	v := &Visualizer{Analyzer: newAnalyzer(), stop: make(chan struct{}), done: make(chan struct{})}
	close(v.done)
	v.Push(samples, rate)
	return v
}

// Wave, son örneklerden n tanesi (eşit aralıklı seçilmiş), -1–1 arası
// (osiloskop için). Ses yoksa sıfırlar.
func (a *Analyzer) Wave(n int) []float64 {
	if n <= 0 {
		return nil
	}
	out := make([]float64, n)
	a.mu.Lock()
	defer a.mu.Unlock()
	if time.Since(a.last) > 400*time.Millisecond {
		return out
	}
	// Son ~20 ms'yi göster: dalganın birkaç periyodu görünsün.
	span := min(fftSize, max(n, a.rate/50))
	for i := 0; i < n; i++ {
		j := (a.pos - span + i*span/n + fftSize) % fftSize
		out[i] = math.Max(-1, math.Min(1, a.ring[j]))
	}
	return out
}

// Level, son ~40 ms'nin ses yüksekliği 0–1 (dB ölçekli; VU göstergesi için).
func (a *Analyzer) Level() float64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	if time.Since(a.last) > 400*time.Millisecond {
		return 0
	}
	sum := 0.0
	for _, s := range a.ring {
		sum += s * s
	}
	rms := math.Sqrt(sum / fftSize)
	db := 20 * math.Log10(rms+1e-9) // 0 dBFS = 1
	return math.Max(0, math.Min(1, (db+48)/48))
}

// Meter, son ~40 ms'nin ortalama (RMS) ve tepe seviyesi, dBFS (0 dB en yüksek).
// Ses yoksa ikisi de -96.
func (a *Analyzer) Meter() (rmsDB, peakDB float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if time.Since(a.last) > 400*time.Millisecond {
		return -96, -96
	}
	sum, peak := 0.0, 0.0
	for _, s := range a.ring {
		sum += s * s
		peak = math.Max(peak, math.Abs(s))
	}
	db := func(x float64) float64 { return math.Max(-96, 20*math.Log10(x+1e-9)) }
	return db(math.Sqrt(sum / fftSize)), db(peak)
}
