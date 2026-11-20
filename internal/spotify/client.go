package spotify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const apiBase = "https://api.spotify.com/v1/"

// ErrNotLoggedIn: kayıtlı giriş yok ya da Spotify onu geçersiz saydı.
var ErrNotLoggedIn = errors.New("spotify account is not connected")

// ErrNoClientID: ayarlarda geliştirici uygulamasının Client ID'si yok.
var ErrNoClientID = errors.New("spotify client id is not set")

// HTTPError, Web API'nin başarısız bir yanıtı.
type HTTPError struct {
	Status  int
	Path    string
	Message string
}

func (e *HTTPError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("Spotify %d: %s", e.Status, e.Message)
	}
	return fmt.Sprintf("Spotify %d (%s)", e.Status, e.Path)
}

// IsStatus, hatanın verilen HTTP durumunda olup olmadığını söyler.
func IsStatus(err error, status int) bool {
	var he *HTTPError
	return errors.As(err, &he) && he.Status == status
}

// Client, Web API istemcisi. Jetonu kendisi yeniler ve diske yazar.
type Client struct {
	clientID  string
	tokenPath string
	http      *http.Client

	mu  sync.Mutex
	tok Token

	cacheMu sync.Mutex
	tracks  map[string]Track
}

func NewClient(clientID, tokenPath string) *Client {
	c := &Client{
		clientID:  clientID,
		tokenPath: tokenPath,
		http:      &http.Client{Timeout: 20 * time.Second},
		tracks:    map[string]Track{},
	}
	if t, err := loadToken(tokenPath); err == nil {
		c.tok = t
	}
	return c
}

func (c *Client) HasClientID() bool { return c.clientID != "" }

// HasLogin, kayıtlı bir giriş olup olmadığını söyler (geçerliliğini denetlemez).
func (c *Client) HasLogin() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tok.RefreshToken != ""
}

// Login, tarayıcıda izin ister ve jetonu kaydeder.
func (c *Client) Login(ctx context.Context, open func(string) error) error {
	if c.clientID == "" {
		return ErrNoClientID
	}
	t, err := login(ctx, c.clientID, open)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.tok = t
	c.mu.Unlock()
	return saveToken(c.tokenPath, t)
}

func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tok.fresh() {
		return c.tok.AccessToken, nil
	}
	if c.clientID == "" {
		return "", ErrNoClientID
	}
	if c.tok.RefreshToken == "" {
		return "", ErrNotLoggedIn
	}
	t, err := refresh(ctx, c.clientID, c.tok.RefreshToken)
	if err != nil {
		if errors.Is(err, ErrNotLoggedIn) {
			c.tok = Token{}
		}
		return "", err
	}
	c.tok = t
	_ = saveToken(c.tokenPath, t)
	return t.AccessToken, nil
}

func (c *Client) expireAccess() {
	c.mu.Lock()
	c.tok.Expiry = time.Time{}
	c.mu.Unlock()
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, path, nil, out)
}

// do, isteği gönderir. 401'de jetonu bir kez yeniler, 429'da Spotify'ın
// istediği kadar (en çok 10 sn) bekleyip yeniden dener.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return err
		}
	}
	for attempt := 0; ; attempt++ {
		at, err := c.accessToken(ctx)
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, method, apiBase+path, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+at)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return err
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		switch {
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			if out != nil && len(bytes.TrimSpace(data)) > 0 {
				return json.Unmarshal(data, out)
			}
			return nil
		case resp.StatusCode == http.StatusUnauthorized && attempt == 0:
			c.expireAccess()
			continue
		case resp.StatusCode == http.StatusTooManyRequests && attempt < 3:
			wait, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
			wait = max(1, min(wait, 10))
			select {
			case <-time.After(time.Duration(wait) * time.Second):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(data, &e)
		return &HTTPError{Status: resp.StatusCode, Path: strings.SplitN(path, "?", 2)[0], Message: e.Error.Message}
	}
}
