// Package ipc, komut satırındaki sotto'nun açık olan sotto'ya bağlanmasını
// sağlar. Bağlantı yerel bir Unix soketi üzerinden (Windows 10'dan beri
// Windows'ta da var): %LOCALAPPDATA%\sotto\sotto.sock. Soket kullanıcının
// kendi klasöründe durduğu için başkası bağlanamaz.
//
// Her bağlantıda bir istek, bir yanıt: tek satır JSON.
package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"time"
)

// Request, komut satırından gelen istek.
type Request struct {
	Cmd  string   `json:"cmd"`
	Args []string `json:"args,omitempty"`
}

// Response, isteğin sonucu. Text insanlar için, Data --json için.
type Response struct {
	OK    bool            `json:"ok"`
	Error string          `json:"error,omitempty"`
	Text  string          `json:"text,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// Fail, hata yanıtı kurar.
func Fail(err error) Response { return Response{Error: err.Error()} }

// Done, başarılı yanıt kurar; data nil olabilir.
func Done(text string, data any) Response {
	r := Response{OK: true, Text: text}
	if data != nil {
		if b, err := json.Marshal(data); err == nil {
			r.Data = b
		}
	}
	return r
}

// Path, soketin yeri.
func Path(localDir string) string { return filepath.Join(localDir, "sotto.sock") }

// ErrNotRunning: açık bir sotto yok.
var ErrNotRunning = errors.New("sotto is not running")

// Server, açık sotto'nun dinleyicisi.
type Server struct {
	l    net.Listener
	path string
}

// Listen, soketi açar ve gelen istekleri handle'a verir. Başka bir sotto
// zaten dinliyorsa ErrInUse döner (o zaman bu pencere sunucu olmaz).
func Listen(path string, handle func(context.Context, Request) Response) (*Server, error) {
	if c, err := net.DialTimeout("unix", path, 500*time.Millisecond); err == nil {
		c.Close()
		return nil, ErrInUse
	}
	_ = os.Remove(path) // kapanmamış eski sotto'dan kalan
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	s := &Server{l: l, path: path}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go serve(c, handle)
		}
	}()
	return s, nil
}

// ErrInUse: başka bir sotto zaten dinliyor.
var ErrInUse = errors.New("another sotto is already listening")

func serve(c net.Conn, handle func(context.Context, Request) Response) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(2 * time.Minute))
	var req Request
	if err := json.NewDecoder(bufio.NewReader(c)).Decode(&req); err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	resp := func() (r Response) {
		defer func() {
			if p := recover(); p != nil {
				r = Response{Error: "internal error"}
			}
		}()
		return handle(ctx, req)
	}()
	_ = json.NewEncoder(c).Encode(resp)
}

// Close, dinleyiciyi kapatır ve soketi siler.
func (s *Server) Close() {
	if s == nil {
		return
	}
	s.l.Close()
	_ = os.Remove(s.path)
}

// Call, açık sotto'ya bir istek gönderir. Açık sotto yoksa ErrNotRunning.
func Call(path string, req Request, timeout time.Duration) (Response, error) {
	c, err := net.DialTimeout("unix", path, time.Second)
	if err != nil {
		return Response{}, ErrNotRunning
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(timeout))
	if err := json.NewEncoder(c).Encode(req); err != nil {
		return Response{}, err
	}
	var resp Response
	if err := json.NewDecoder(bufio.NewReader(c)).Decode(&resp); err != nil {
		return Response{}, err
	}
	return resp, nil
}
