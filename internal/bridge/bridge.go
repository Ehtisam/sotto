// Package bridge, Spotify'ın resmî olmayan iç bağlantıları (✱) için
// masaüstü kimliğiyle bir erişim jetonu alır. Bunu go-librespot'u ses
// çalmadan, yalnızca jeton gerektiğinde birkaç saniyeliğine çalıştırarak
// yapar: go-librespot açıkken Spotify'ın cihaz listesinde göründüğü için
// sürekli açık tutulmaz.
package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Ehtisam/sotto/internal/proc"
)

var (
	// ErrNotInstalled: go-librespot yok; ✱ özellikleri kullanılamaz.
	ErrNotInstalled = errors.New("go-librespot not found")
)

// LoginNeeded, bir kerelik tarayıcı girişi gerektiğinde döner.
type LoginNeeded struct{ URL string }

func (e *LoginNeeded) Error() string { return "spotify login needed for extras" }

// tokenLife: go-librespot jetonun süresini bildirmiyor; Spotify'ın jetonları bir saat geçerli.
const tokenLife = 50 * time.Minute

type Bridge struct {
	dir string

	mu      sync.Mutex
	token   string
	expiry  time.Time
	waiting *running // tarayıcı girişi bekleyen süreç
}

type running struct {
	cmd  *exec.Cmd
	port int
	done chan struct{}
}

// New, go-librespot'un ve giriş bilgisinin durduğu klasörle bir köprü kurar.
func New(dir string) *Bridge { return &Bridge{dir: dir} }

func (b *Bridge) exe() string {
	name := "go-librespot"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(b.dir, name)
}

// Installed, go-librespot'un yerinde olup olmadığını söyler.
func (b *Bridge) Installed() bool {
	_, err := os.Stat(b.exe())
	return err == nil
}

// Token, geçerli bir jeton döner; gerekirse go-librespot'u kısa süreliğine çalıştırır.
// Hesap henüz bağlanmamışsa *LoginNeeded döner; kullanıcı girişi bitirince
// sonraki çağrı jetonu alır.
func (b *Bridge) Token(ctx context.Context) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.token != "" && time.Now().Before(b.expiry) {
		return b.token, nil
	}
	if !b.Installed() {
		return "", ErrNotInstalled
	}

	r := b.waiting
	if r == nil {
		var err error
		if r, err = b.start(); err != nil {
			return "", err
		}
	}
	tok, login, err := b.await(ctx, r)
	if login != "" {
		b.waiting = r // giriş bitene kadar açık kalsın
		return "", &LoginNeeded{URL: login}
	}
	b.waiting = nil
	stop(r)
	if err != nil {
		return "", err
	}
	b.token, b.expiry = tok, time.Now().Add(tokenLife)
	return tok, nil
}

// Invalidate, jeton reddedildiyse bir sonrakinin yeniden alınmasını sağlar.
func (b *Bridge) Invalidate() {
	b.mu.Lock()
	b.token = ""
	b.mu.Unlock()
}

// Close, giriş bekleyen bir süreç varsa kapatır.
func (b *Bridge) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.waiting != nil {
		stop(b.waiting)
		b.waiting = nil
	}
}

const config = `device_name: sotto helper
device_type: computer
zeroconf_enabled: false
credentials:
  type: interactive
server:
  enabled: true
  address: localhost
log_level: info
`

func (b *Bridge) start() (*running, error) {
	if err := os.MkdirAll(b.dir, 0o700); err != nil {
		return nil, err
	}
	cfg := filepath.Join(b.dir, "config.yml")
	if _, err := os.Stat(cfg); err != nil {
		if err := os.WriteFile(cfg, []byte(config), 0o600); err != nil {
			return nil, err
		}
	}
	_ = os.Remove(filepath.Join(b.dir, "lockfile")) // önceki ani kapanıştan kalmış olabilir

	port, err := freePort()
	if err != nil {
		return nil, err
	}
	log, err := os.Create(filepath.Join(b.dir, "bridge.log"))
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(b.exe(), "--config_dir", b.dir, "-c", fmt.Sprintf("server.port=%d", port))
	cmd.Stdout, cmd.Stderr = log, log
	proc.Detach(cmd)
	if err := cmd.Start(); err != nil {
		log.Close()
		return nil, err
	}
	_ = proc.BindToParent(cmd.Process.Pid)
	r := &running{cmd: cmd, port: port, done: make(chan struct{})}
	go func() { cmd.Wait(); log.Close(); close(r.done) }()
	return r, nil
}

var loginRE = regexp.MustCompile(`visit the following link: (https://accounts\.spotify\.com/\S+?)"`)

// await, jeton gelene kadar bekler. Giriş gerekiyorsa adresini döner.
func (b *Bridge) await(ctx context.Context, r *running) (token, login string, err error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	url := fmt.Sprintf("http://127.0.0.1:%d/token", r.port)
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			var body struct {
				Token string `json:"token"`
			}
			ok := resp.StatusCode == http.StatusOK && json.NewDecoder(resp.Body).Decode(&body) == nil && body.Token != ""
			resp.Body.Close()
			if ok {
				return body.Token, "", nil
			}
		}
		if data, err := os.ReadFile(filepath.Join(b.dir, "bridge.log")); err == nil {
			if m := loginRE.FindStringSubmatch(string(data)); m != nil && !strings.Contains(string(data), "authenticated AP") {
				return "", m[1], nil
			}
		}
		select {
		case <-r.done:
			return "", "", errors.New("go-librespot exited, see bridge.log")
		case <-ctx.Done():
			return "", "", errors.New("go-librespot did not give a token in time")
		case <-time.After(300 * time.Millisecond):
		}
	}
}

func stop(r *running) {
	if r == nil || r.cmd.Process == nil {
		return
	}
	select {
	case <-r.done:
		return
	default:
	}
	_ = r.cmd.Process.Kill()
	select {
	case <-r.done:
	case <-time.After(3 * time.Second):
	}
}

func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}
