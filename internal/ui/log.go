package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Günlük: sotto'nun yaptıkları ve hatalar. Alt satırda kısa süre görünen
// notlar, çalan şarkının değişmesi, komut satırından gelen istekler burada
// kalır; ekranda "g o" ile açılır, %LOCALAPPDATA%\sotto\logs\sotto.log'a da yazılır.

type logLevel int

const (
	logInfo logLevel = iota
	logError
	logEvent
)

type logEntry struct {
	at    time.Time
	level logLevel
	text  string
}

// maxLog: bellekte tutulan en çok kayıt.
const maxLog = 500

// maxLogFile: dosya bu boyutu geçince .1 olarak saklanır, yenisi başlar.
const maxLogFile = 1 << 20

type logBook struct {
	mu      sync.Mutex
	entries []logEntry
	path    string
}

// add, bir kayıt ekler (aynı metin art arda gelirse yinelenmez).
func (l *logBook) add(level logLevel, text string, at time.Time) {
	if l == nil || text == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if n := len(l.entries); n > 0 && l.entries[n-1].text == text && at.Sub(l.entries[n-1].at) < 5*time.Second {
		return
	}
	if at.IsZero() {
		at = time.Now()
	}
	l.entries = append(l.entries, logEntry{at, level, text})
	if len(l.entries) > maxLog {
		l.entries = append([]logEntry(nil), l.entries[len(l.entries)-maxLog:]...)
	}
	if l.path == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(l.path), 0o700)
	if fi, err := os.Stat(l.path); err == nil && fi.Size() > maxLogFile {
		_ = os.Rename(l.path, l.path+".1")
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s  %-5s  %s\n", at.Format("2006-01-02 15:04:05"), [...]string{"info", "error", "event"}[level], text)
}

// list, en yeniden eskiye kayıtlar.
func (l *logBook) list() []logEntry {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]logEntry, len(l.entries))
	for i, e := range l.entries {
		out[len(out)-1-i] = e
	}
	return out
}

func logFile(dir string) string { return filepath.Join(dir, "logs", "sotto.log") }
