package soloist

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Soloist'in belgelenmiş çıkış kodları.
const exitExpired = 10

var (
	// ErrNotInstalled: soloist çalıştırılabilir dosyası yok.
	ErrNotInstalled = errors.New("soloist bulunamadı")
	// ErrNoAPIKey: Soloist API anahtarı yok.
	ErrNoAPIKey = errors.New("soloist api anahtarı yok")
	// ErrExpired: kurulu Soloist sürümünün 90 günlük ömrü dolmuş.
	ErrExpired = errors.New("soloist sürümünün süresi dolmuş")
)

// Options, Soloist'i başlatmak için gerekenler.
type Options struct {
	Path          string // soloist(.exe)
	APIKey        string
	DeviceName    string
	DataDir       string // giriş bilgisi ve ws.port burada
	CacheDir      string
	LogPath       string
	InitialVolume int
}

// Process, sotto'nun başlattığı bir Soloist süreci.
type Process struct {
	cmd     *exec.Cmd
	logFile *os.File
	done    chan struct{}
	waitErr error

	// Addr, WebSocket API'nin adresi (127.0.0.1:port).
	Addr string
}

// Start, Soloist'i WebSocket API'si açık olarak başlatır ve API hazır olana
// kadar bekler. Port işletim sistemine bırakılır, Soloist'in yazdığı
// ws.port dosyasından okunur.
func Start(ctx context.Context, o Options) (*Process, error) {
	if o.Path == "" {
		return nil, ErrNotInstalled
	}
	if _, err := os.Stat(o.Path); err != nil {
		return nil, ErrNotInstalled
	}
	if strings.TrimSpace(o.APIKey) == "" {
		return nil, ErrNoAPIKey
	}
	for _, d := range []string{o.DataDir, o.CacheDir, filepath.Dir(o.LogPath)} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return nil, err
		}
	}
	// Önceki bir çalışmadan kalan adres dosyaları yanlış porta götürmesin.
	_ = os.Remove(filepath.Join(o.DataDir, "ws.port"))
	_ = os.Remove(filepath.Join(o.DataDir, "ws.addr"))

	logFile, err := os.OpenFile(o.LogPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, err
	}

	args := []string{
		"--device-name", o.DeviceName,
		"--api-key", strings.TrimSpace(o.APIKey),
		"--data-dir", o.DataDir,
		"--cache-dir", o.CacheDir,
		"--ws", "127.0.0.1:0",
	}
	if o.InitialVolume > 0 {
		args = append(args, "--initial-volume", fmt.Sprint(o.InitialVolume))
	}
	cmd := exec.Command(o.Path, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	detach(cmd)
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, fmt.Errorf("soloist başlatılamadı: %w", err)
	}

	_ = bindToParent(cmd.Process.Pid) // olmazsa da sotto kapanırken Stop ile kapatılır

	p := &Process{cmd: cmd, logFile: logFile, done: make(chan struct{})}
	go func() {
		p.waitErr = cmd.Wait()
		logFile.Close()
		close(p.done)
	}()

	addr, err := waitForAddr(ctx, o.DataDir, p.done)
	if err != nil {
		p.Stop()
		if code := p.ExitCode(); code == exitExpired {
			return nil, ErrExpired
		} else if code > 0 {
			return nil, fmt.Errorf("soloist kapandı (çıkış kodu %d), ayrıntı: %s", code, o.LogPath)
		}
		return nil, err
	}
	p.Addr = addr
	return p, nil
}

func waitForAddr(ctx context.Context, dataDir string, done <-chan struct{}) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		port, perr := os.ReadFile(filepath.Join(dataDir, "ws.port"))
		host, herr := os.ReadFile(filepath.Join(dataDir, "ws.addr"))
		if perr == nil && strings.TrimSpace(string(port)) != "" {
			h := "127.0.0.1"
			if herr == nil && strings.TrimSpace(string(host)) != "" {
				h = strings.TrimSpace(string(host))
			}
			return h + ":" + strings.TrimSpace(string(port)), nil
		}
		select {
		case <-done:
			return "", errors.New("soloist hemen kapandı")
		case <-ctx.Done():
			return "", errors.New("soloist'in API'si açılmadı (15 sn)")
		case <-tick.C:
		}
	}
}

// Done, süreç bitince kapanır.
func (p *Process) Done() <-chan struct{} { return p.done }

// ExitCode, süreç bittiyse çıkış kodu; bitmediyse -1.
func (p *Process) ExitCode() int {
	select {
	case <-p.done:
	default:
		return -1
	}
	if p.cmd.ProcessState == nil {
		return -1
	}
	return p.cmd.ProcessState.ExitCode()
}

// Stop, Soloist'i kapatır ve bitmesini bekler.
func (p *Process) Stop() {
	if p == nil || p.cmd.Process == nil {
		return
	}
	select {
	case <-p.done:
		return
	default:
	}
	_ = p.cmd.Process.Kill()
	select {
	case <-p.done:
	case <-time.After(3 * time.Second):
	}
}
