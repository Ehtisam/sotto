package soloist

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStreamQualityLabel(t *testing.T) {
	for _, tc := range []struct {
		q    StreamQuality
		want string
	}{
		{StreamQuality{Codec: "flac", SampleRate: 44100, Bitrate: 812000}, "FLAC 44.1"},
		{StreamQuality{Codec: "flac", SampleRate: 48000}, "FLAC 48"},
		{StreamQuality{Codec: "flac"}, "FLAC"},
		{StreamQuality{Codec: "vorbis", SampleRate: 44100, Bitrate: 320000}, "Vorbis 320"},
		{StreamQuality{Codec: "aac", Bitrate: 128000}, "AAC 128"},
		{StreamQuality{}, ""},
	} {
		if got := tc.q.Label(); got != tc.want {
			t.Errorf("%+v: %q, want %q", tc.q, got, tc.want)
		}
	}
}

func TestLogSinkQuality(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "soloist.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	s := &logSink{file: f}
	// Gerçek sıra (bit hızı codec'ten önce), parça parça, iki biçim, CRLF.
	chunks := []string{
		"09:50:11.277 I [/f:462   ] ready, device_name=sotto device_id=x\r\n",
		"2026-11-21 21:48:18.639: FileStreamer file average bitrate: 812000\n09:50:12.001 I [/f:347   ]     co",
		"dec: FLAC\r\n09:50:12.002 I [/f:349   ]     sample rate: 44100 Hz\n",
	}
	for _, c := range chunks {
		if _, err := s.Write([]byte(c)); err != nil {
			t.Fatal(err)
		}
	}
	if q := s.Quality(); q != (StreamQuality{Codec: "flac", SampleRate: 44100, Bitrate: 812000}) || !q.Lossless() {
		t.Fatalf("quality = %+v", q)
	}
	// Sonraki şarkı: yeni codec satırı örneklemeyi sıfırlar; "bitrate:" satırı da okunur.
	_, _ = s.Write([]byte("10:00:00.000 I [/f:347 ]     codec: vorbis\n10:00:00.000 I [/f:348 ]     bitrate: 320000 bits/s\n"))
	if q := s.Quality(); q.Codec != "vorbis" || q.SampleRate != 0 || q.Lossless() || q.Label() != "Vorbis 320" {
		t.Fatalf("after next song = %+v", q)
	}
	// Gerçek günlükten alınmış satırlar.
	var real StreamQuality
	for _, l := range []string{
		"19:16:38.680 I [/f:134                          ] FileStreamer file average bitrate: 320000",
		"19:16:38.683 I [/f:347                          ]     codec: vorbis",
		"19:16:38.683 I [/f:349                          ]     sample rate: 44100 Hz",
	} {
		applyLogLine(&real, l)
	}
	if real != (StreamQuality{Codec: "vorbis", SampleRate: 44100, Bitrate: 320000}) {
		t.Fatalf("real log = %+v", real)
	}
	b, _ := os.ReadFile(f.Name())
	if !strings.Contains(string(b), "ready, device_name=sotto") || !strings.Contains(string(b), "codec: vorbis") {
		t.Fatalf("log file missing lines:\n%s", b)
	}
}

func TestLogSinkCapsFile(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "soloist.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	s := &logSink{file: f}
	line := []byte(strings.Repeat("x", 1023) + "\n")
	for range maxLogBytes/len(line) + 10 {
		_, _ = s.Write(line)
	}
	st, _ := f.Stat()
	if st.Size() > maxLogBytes {
		t.Fatalf("log grew to %d bytes", st.Size())
	}
}
