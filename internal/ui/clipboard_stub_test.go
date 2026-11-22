package ui

import "sync"

// Testler gerçek panoya dokunmasın: okuma boş döner, yazılanlar burada toplanır.
// Tarayıcı ve dosya açma da hiçbir şey açmaz (Ayarlar'daki "Tuşlar" ve "Ayar
// dosyası" satırları dosyayı varsayılan programda açıyordu).
var (
	testClipMu      sync.Mutex
	testClipWritten []string
)

func init() {
	openBrowser = func(string) error { return nil }
	readClipboard = func() (string, error) { return "", nil }
	writeClipboard = func(s string) error {
		testClipMu.Lock()
		testClipWritten = append(testClipWritten, s)
		testClipMu.Unlock()
		return nil
	}
}

func lastCopied() string {
	testClipMu.Lock()
	defer testClipMu.Unlock()
	if len(testClipWritten) == 0 {
		return ""
	}
	return testClipWritten[len(testClipWritten)-1]
}
