package soloist

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
)

// StreamQuality, Soloist'in o an çaldığı akış: -v ile açılınca her şarkıda
// günlüğüne "codec: flac", "sample rate: 44100" ve "FileStreamer file average
// bitrate: 812000" satırlarını yazıyor (spotatui'nin bulduğu yol).
type StreamQuality struct {
	Codec      string // "flac", "vorbis", …
	SampleRate int    // Hz
	Bitrate    int    // bit/sn
}

// Lossless, akış FLAC mı.
func (q StreamQuality) Lossless() bool { return q.Codec == "flac" }

// Label, "FLAC 44.1" ya da "Vorbis 320" gibi kısa ad; codec bilinmiyorsa boş.
func (q StreamQuality) Label() string {
	if q.Codec == "" {
		return ""
	}
	if q.Lossless() {
		switch {
		case q.SampleRate <= 0:
			return "FLAC"
		case q.SampleRate%1000 == 0:
			return fmt.Sprintf("FLAC %d", q.SampleRate/1000)
		}
		return fmt.Sprintf("FLAC %.1f", float64(q.SampleRate)/1000)
	}
	name := q.Codec
	switch name {
	case "vorbis":
		name = "Vorbis"
	case "mp3", "aac":
		name = strings.ToUpper(name)
	}
	if q.Bitrate > 0 {
		return fmt.Sprintf("%s %d", name, q.Bitrate/1000)
	}
	return name
}

// logMessage, satırın zaman/düzey/kaynak önekinden sonraki kısmı.
// Satırlar "09:50:11.277 I [/f:462   ] mesaj" ya da "2026-…: mesaj" biçiminde.
func logMessage(line string) string {
	if i := strings.Index(line, "] "); i >= 0 && strings.Contains(line[:i], "[") {
		return line[i+2:]
	}
	return line
}

// leadingInt, metnin başındaki sayı.
func leadingInt(s string) (int, bool) {
	s = strings.TrimSpace(s)
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	n, err := strconv.Atoi(s[:end])
	return n, err == nil
}

// applyLogLine, satır bir kalite bilgisiyse q'yu günceller. Soloist 1.3.10'da
// sıra (21.11.2026 gerçek günlük): "FileStreamer file average bitrate: N",
// sonra "codec: vorbis", "bitrate: N bits/s", "sample rate: 44100 Hz". Yeni
// "codec:" satırı yeni şarkıdır: örnekleme sıfırlanır, az önce gelen bit hızı kalır.
func applyLogLine(q *StreamQuality, line string) bool {
	msg := strings.TrimSpace(logMessage(line))
	if c, ok := strings.CutPrefix(msg, "codec: "); ok {
		*q = StreamQuality{Codec: strings.ToLower(strings.TrimSpace(c)), Bitrate: q.Bitrate}
		return true
	}
	if r, ok := strings.CutPrefix(msg, "bitrate: "); ok {
		if n, ok := leadingInt(r); ok && n > 0 {
			q.Bitrate = n
			return true
		}
	}
	if r, ok := strings.CutPrefix(msg, "sample rate: "); ok {
		if n, ok := leadingInt(r); ok {
			q.SampleRate = n
			return true
		}
	}
	if i := strings.Index(msg, "FileStreamer file average bitrate: "); i >= 0 {
		if n, ok := leadingInt(msg[i+len("FileStreamer file average bitrate: "):]); ok {
			q.Bitrate = n
			return true
		}
	}
	return false
}

// maxLogBytes: -v günlüğü uzun oturumda büyür; bu boyutu geçince dosya
// baştan yazılır (yalnız son kısım kalır).
const maxLogBytes = 4 << 20

// logSink, Soloist'in stdout/stderr'ini alır: satırları dosyaya yazar ve
// kalite satırlarını yakalar. exec aynı yazıcıyı iki akışa verince Write'ı
// aynı anda tek goroutine çağırır; Quality ise arayüzden okunur, o yüzden kilit.
type logSink struct {
	file    *os.File
	written int
	partial []byte

	mu      sync.Mutex
	quality StreamQuality
}

func (s *logSink) Write(p []byte) (int, error) {
	if s.file != nil {
		if s.written+len(p) > maxLogBytes {
			_ = s.file.Truncate(0)
			_, _ = s.file.Seek(0, 0)
			s.written = 0
		}
		n, _ := s.file.Write(p)
		s.written += n
	}
	s.partial = append(s.partial, p...)
	for {
		i := bytes.IndexByte(s.partial, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimRight(string(s.partial[:i]), "\r")
		s.partial = s.partial[i+1:]
		s.mu.Lock()
		applyLogLine(&s.quality, line)
		s.mu.Unlock()
	}
	if len(s.partial) > 64<<10 { // satır sonu gelmeyen dev çıktı
		s.partial = s.partial[:0]
	}
	return len(p), nil
}

// Quality, son okunan akış bilgisi.
func (s *logSink) Quality() StreamQuality {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.quality
}
