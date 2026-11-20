// Package spotify, kullanıcının kendi Spotify geliştirici uygulamasıyla
// (PKCE, gizli anahtar yok) giriş yapar ve resmî Web API'yi çağırır.
package spotify

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Ehtisam/sotto/internal/i18n"
)

// RedirectURI, geliştirici uygulamasına eklenmesi gereken dönüş adresi.
const RedirectURI = "http://127.0.0.1:8765/callback"

const (
	authURL  = "https://accounts.spotify.com/authorize"
	tokenURL = "https://accounts.spotify.com/api/token"
)

var scopes = []string{
	"user-read-private", "user-read-email",
	"user-top-read", "user-read-recently-played",
	"user-library-read", "user-library-modify",
	"user-follow-read", "user-follow-modify",
	"playlist-read-private", "playlist-read-collaborative",
	"playlist-modify-private", "playlist-modify-public",
	"user-read-playback-state", "user-modify-playback-state", "user-read-currently-playing",
}

// Token, Spotify'dan alınan erişim ve yenileme jetonu.
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	Expiry       time.Time `json:"expiry"`
}

func (t Token) fresh() bool { return t.AccessToken != "" && time.Until(t.Expiry) > time.Minute }

// login, tarayıcıda Spotify'ın izin sayfasını açar ve dönüşü 127.0.0.1:8765'te bekler.
func login(ctx context.Context, clientID string, open func(string) error) (Token, error) {
	verifier := randomString(64)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	state := randomString(16)

	ln, err := net.Listen("tcp", "127.0.0.1:8765")
	if err != nil {
		return Token{}, fmt.Errorf("port 8765 is in use: %w", err)
	}

	type result struct {
		code string
		err  error
	}
	results := make(chan result, 1)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/callback" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		if q.Get("state") != state {
			http.Error(w, i18n.T("web.stale"), http.StatusBadRequest)
			return
		}
		res := result{code: q.Get("code")}
		page := i18n.T("web.done")
		if e := q.Get("error"); e != "" {
			res = result{err: fmt.Errorf("spotify denied access (%s)", e)}
			page = i18n.T("web.denied")
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<p style="font-family:sans-serif;padding:2em">%s</p>`, page)
		select {
		case results <- res:
		default:
		}
	})}
	go srv.Serve(ln)
	defer srv.Shutdown(context.Background())

	q := url.Values{
		"client_id":             {clientID},
		"response_type":         {"code"},
		"redirect_uri":          {RedirectURI},
		"scope":                 {strings.Join(scopes, " ")},
		"code_challenge_method": {"S256"},
		"code_challenge":        {challenge},
		"state":                 {state},
	}
	if err := open(authURL + "?" + q.Encode()); err != nil {
		return Token{}, fmt.Errorf("could not open the browser: %w", err)
	}

	select {
	case r := <-results:
		if r.err != nil {
			return Token{}, r.err
		}
		return exchange(ctx, url.Values{
			"grant_type":    {"authorization_code"},
			"code":          {r.code},
			"redirect_uri":  {RedirectURI},
			"client_id":     {clientID},
			"code_verifier": {verifier},
		})
	case <-ctx.Done():
		return Token{}, errors.New("login timed out")
	}
}

func refresh(ctx context.Context, clientID, refreshToken string) (Token, error) {
	t, err := exchange(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {clientID},
	})
	if err == nil && t.RefreshToken == "" {
		t.RefreshToken = refreshToken
	}
	return t, err
}

func exchange(ctx context.Context, form url.Values) (Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Token{}, err
	}
	defer resp.Body.Close()
	var body struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Error        string `json:"error"`
		Description  string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Token{}, fmt.Errorf("could not read token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK || body.AccessToken == "" {
		if body.Error == "invalid_grant" {
			return Token{}, ErrNotLoggedIn
		}
		return Token{}, fmt.Errorf("could not get a token: %s %s", body.Error, body.Description)
	}
	return Token{
		AccessToken:  body.AccessToken,
		RefreshToken: body.RefreshToken,
		Expiry:       time.Now().Add(time.Duration(body.ExpiresIn) * time.Second),
	}, nil
}

func loadToken(path string) (Token, error) {
	var t Token
	b, err := os.ReadFile(path)
	if err != nil {
		return t, err
	}
	err = json.Unmarshal(b, &t)
	return t, err
}

func saveToken(path string, t Token) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func randomString(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)[:n]
}
