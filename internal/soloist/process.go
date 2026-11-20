package soloist

import (
	"context"
	"errors"
	"fmt"
	"net"
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
	ErrNotInstalled = errors.New("soloist not found")
	// ErrNoAPIKey: Soloist API anahtarı yok.
	ErrNoAPIKey = errors.New("no soloist api key")
	// ErrExpired: kurulu Soloist sürümünün 90 günlük ömrü dolmuş.
	ErrExpired = errors.New("soloist build has expired")
	// ErrAlreadyRunning: aynı veri klasörüyle başka bir Soloist çalışıyor
	// ve sotto ona bağlanamadı.
	ErrAlreadyRunning = errors.New("another soloist session is running")
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

// Process, sotto'nun başlattığı ya da zaten çalışıp bağlandığı bir Soloist.
type Process struct {
	cmd     *exec.Cmd
	logFile *os.File
	done    chan struct{}
	waitErr error

	// Addr, WebSocket API'nin adresi (127.0.0.1:port).
	Addr string
	// Attached: Soloist'i başka biri (ör. başka bir sotto penceresi)
	// başlatmış; bu sotto kapanınca onu kapatmaz.
	Attached bool
}

// running, veri klasöründe çalışan bir Soloist'in API adresini bulur.
func running(dataDir string) (string, bool) {
	port, err := os.ReadFile(filepath.Join(dataDir, "ws.port"))
	if err != nil || strings.TrimSpace(string(port)) == "" {
		return "", false
	}
	host := "127.0.0.1"
	if b, err := os.ReadFile(filepath.Join(dataDir, "ws.addr")); err == nil && strings.TrimSpace(string(b)) != "" {
		host = strings.TrimSpace(string(b))
	}
	addr := net.JoinHostPort(host, strings.TrimSpace(string(port)))
	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		return "", false
	}
	conn.Close()
	return addr, true
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
	// Soloist zaten çalışıyorsa (ör. başka bir sotto penceresi) ona bağlan.
	if addr, ok := running(o.DataDir); ok {
		return &Process{Addr: addr, Attached: true, done: make(chan struct{})}, nil
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
		return nil, fmt.Errorf("could not start soloist: %w", err)
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
		if log, _ := os.ReadFile(o.LogPath); strings.Contains(string(log), "another session is running") {
			return nil, ErrAlreadyRunning
		}
		if code := p.ExitCode(); code == exitExpired {
			return nil, ErrExpired
		} else if code > 0 {
			return nil, fmt.Errorf("soloist exited with code %d (see %s)", code, o.LogPath)
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
			return "", errors.New("soloist exited right away")
		case <-ctx.Done():
			return "", errors.New("soloist api did not open within 15s")
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
	if p.cmd == nil || p.cmd.ProcessState == nil {
		return -1
	}
	return p.cmd.ProcessState.ExitCode()
}

// Stop, Soloist'i kapatır ve bitmesini bekler.
func (p *Process) Stop() {
	if p == nil || p.Attached || p.cmd == nil || p.cmd.Process == nil {
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
