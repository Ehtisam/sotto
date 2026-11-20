package setup

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func zipWith(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(body))
	}
	_ = zw.Close()
	return b.Bytes()
}

func tgzWith(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		_ = tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte(body))
	}
	_ = tw.Close()
	_ = gz.Close()
	return b.Bytes()
}

func serve(t *testing.T, files map[string][]byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFetchFromZipAndTarGz(t *testing.T) {
	srv := serve(t, map[string][]byte{
		"/soloist.zip":   zipWith(t, map[string]string{"README.txt": "hi", "bin/soloist.exe": "SOLOIST"}),
		"/librespot.tgz": nil, // aşağıda
	})
	srv2 := serve(t, map[string][]byte{
		"/go-librespot.tar.gz": tgzWith(t, map[string]string{"LICENSE": "x", "go-librespot.exe": "LIBRESPOT"}),
	})
	dir := t.TempDir()
	var p Progress
	dest := filepath.Join(dir, "soloist", "soloist.exe")
	if err := Fetch(context.Background(), srv.URL+"/soloist.zip", "soloist.exe", dest, &p); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dest); string(b) != "SOLOIST" {
		t.Fatalf("soloist = %q", b)
	}
	if p.Done.Load() == 0 || p.Done.Load() != p.Total.Load() {
		t.Fatalf("progress %d / %d", p.Done.Load(), p.Total.Load())
	}
	dest2 := filepath.Join(dir, "bridge", "go-librespot.exe")
	if err := Fetch(context.Background(), srv2.URL+"/go-librespot.tar.gz", "go-librespot.exe", dest2, nil); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dest2); string(b) != "LIBRESPOT" {
		t.Fatalf("librespot = %q", b)
	}
	// Var olanın üstüne yazar; geride .part kalmaz.
	if err := Fetch(context.Background(), srv.URL+"/soloist.zip", "soloist.exe", dest, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dest + ".part"); !os.IsNotExist(err) {
		t.Fatal("leftover .part file")
	}
}

func TestFetchErrors(t *testing.T) {
	srv := serve(t, map[string][]byte{"/empty.zip": zipWith(t, map[string]string{"other.exe": "x"})})
	dir := t.TempDir()
	dest := filepath.Join(dir, "soloist.exe")
	if err := Fetch(context.Background(), srv.URL+"/missing.zip", "soloist.exe", dest, nil); err == nil {
		t.Fatal("404 should fail")
	}
	if err := Fetch(context.Background(), srv.URL+"/empty.zip", "soloist.exe", dest, nil); err == nil {
		t.Fatal("archive without the program should fail")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("nothing should be written on failure")
	}
}

func TestOfficialURLs(t *testing.T) {
	u, err := SoloistURL()
	if err != nil {
		t.Skip("no Soloist build for this system")
	}
	if u == "" || !bytes.HasPrefix([]byte(u), []byte("https://soloist-builds.spotifycdn.com/")) {
		t.Fatalf("soloist url = %q", u)
	}
	if u, err := LibrespotURL(); err == nil && !bytes.HasPrefix([]byte(u), []byte("https://github.com/devgianlu/go-librespot/releases/download/")) {
		t.Fatalf("librespot url = %q", u)
	}
}
