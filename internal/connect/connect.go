// Package connect, Spotify Connect'in iç durum servisine (connect-state, ✱)
// gizli bir gözlemci olarak bağlanır: çalan cihazın sırasını tam haliyle
// okur ve sırayı düzenleme (silme, yer değiştirme) komutunu gönderir. Soloist
// API'si de resmî Web API de sıradan silmeyi bilmiyor; Spotify'ın kendi
// uygulamaları bu yolu kullanıyor. Belgesi yok, haber verilmeden değişebilir;
// jetonu bridge paketi sağlar.
//
// Gözlemci cihaz listesinde görünmez (capabilities.hidden) ve iş bitince
// kaydını siler; bağlantı yalnızca bir düzenleme sürdüğü kadar açık kalır.
package connect

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"
)

// ErrUnauthorized: jeton reddedildi, yenisi alınmalı.
var ErrUnauthorized = errors.New("connect: token rejected")

// ErrNoPlayer: şu an hiçbir cihaz çalmıyor.
var ErrNoPlayer = errors.New("connect: nothing is playing")

// Adresler (testler değiştirir). apresolve cevap vermezse bunlar kullanılır.
var (
	dealerHost   = "dealer.spotify.com"
	spclientHost = "spclient.wg.spotify.com"
	resolveURL   = "https://apresolve.spotify.com/?type=dealer&type=spclient"
	httpClient   = &http.Client{Timeout: 15 * time.Second}
)

// Track, sıradaki bir satır. Spotify'ın gönderdiği hali (raw) olduğu gibi
// geri gönderilir; sotto yalnızca okuduğu alanlara bakar.
type Track struct {
	URI      string
	UID      string
	Provider string // queue, context, autoplay
	Metadata map[string]string
	raw      json.RawMessage
}

// Queued, satır kullanıcının sıraya eklediklerinden mi.
func (t Track) Queued() bool {
	return t.Provider == "queue" || t.Metadata["is_queued"] == "true"
}

// Delimiter, sıra ile bağlamın kalanını ayıran işaret satırı.
func (t Track) Delimiter() bool { return t.URI == "spotify:delimiter" }

func (t *Track) UnmarshalJSON(b []byte) error {
	var v struct {
		URI      string            `json:"uri"`
		UID      string            `json:"uid"`
		Provider string            `json:"provider"`
		Metadata map[string]string `json:"metadata"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*t = Track{URI: v.URI, UID: v.UID, Provider: v.Provider, Metadata: v.Metadata, raw: append(json.RawMessage(nil), b...)}
	return nil
}

func (t Track) MarshalJSON() ([]byte, error) {
	if len(t.raw) > 0 {
		return t.raw, nil
	}
	v := map[string]any{"uri": t.URI, "uid": t.UID, "metadata": t.Metadata}
	if t.Provider != "" {
		v["provider"] = t.Provider
	}
	if t.Metadata == nil {
		v["metadata"] = map[string]string{}
	}
	return json.Marshal(v)
}

// PlayerState, çalan cihazın durumundan sotto'nun kullandığı kısım.
type PlayerState struct {
	Track         *Track            `json:"track"`
	NextTracks    []Track           `json:"next_tracks"`
	PrevTracks    []json.RawMessage `json:"prev_tracks"`
	QueueRevision json.RawMessage   `json:"queue_revision"` // dize ya da sayı; aynen geri gider
	ContextURI    string            `json:"context_uri"`
}

// Cluster, hesabın bütün Connect cihazlarının ortak durumu.
type Cluster struct {
	ActiveDeviceID string      `json:"active_device_id"`
	PlayerState    PlayerState `json:"player_state"`
}

// Session, açık bir gözlemci bağlantısı. Open ile açılır, Close ile kapanır.
type Session struct {
	token    string
	spclient string
	conn     *websocket.Conn
	connID   string
	deviceID string
	cluster  Cluster
	stop     context.CancelFunc
}

// Open, dealer'a bağlanır, gizli gözlemci olarak kaydolur ve kümeyi okur.
func Open(ctx context.Context, token string) (*Session, error) {
	dealer, sp := resolve(ctx)
	conn, resp, err := websocket.Dial(ctx, "wss://"+dealer+"/?access_token="+url.QueryEscape(token), nil)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			return nil, ErrUnauthorized
		}
		return nil, fmt.Errorf("connect: dealer: %w", err)
	}
	conn.SetReadLimit(8 << 20)
	s := &Session{token: token, spclient: sp, conn: conn, deviceID: "hobs_" + randomHex(16)}

	if s.connID, err = s.awaitConnectionID(ctx); err != nil {
		conn.CloseNow()
		return nil, err
	}
	// Bağlantı açık kaldıkça dealer'ın gönderdiklerini okumak ve ping
	// atmak gerek; yoksa dealer bağlantıyı kapatır.
	bg, stop := context.WithCancel(context.Background())
	s.stop = stop
	go s.drain(bg)

	body := map[string]any{
		"member_type": "CONNECT_STATE",
		"device": map[string]any{"device_info": map[string]any{"capabilities": map[string]any{
			"can_be_player": false, "hidden": true, "needs_full_player_state": true,
		}}},
	}
	data, err := s.request(ctx, http.MethodPut, "/connect-state/v1/devices/"+s.deviceID, body)
	if err != nil {
		s.closeConn()
		return nil, err
	}
	if err := json.Unmarshal(data, &s.cluster); err != nil {
		s.Close()
		return nil, fmt.Errorf("connect: cluster: %w", err)
	}
	return s, nil
}

// Cluster, kayıt anındaki küme.
func (s *Session) Cluster() Cluster { return s.cluster }

// SetQueue, çalan cihazın sıradakilerini next ile değiştirir. Satırlar
// kümeden okunduğu haliyle gönderilmeli; sotto yalnızca çıkarır ya da yerini
// değiştirir.
func (s *Session) SetQueue(ctx context.Context, next []Track) error {
	c := s.cluster
	if c.ActiveDeviceID == "" {
		return ErrNoPlayer
	}
	prev := c.PlayerState.PrevTracks
	if prev == nil {
		prev = []json.RawMessage{}
	}
	if next == nil {
		next = []Track{}
	}
	cmd := map[string]any{
		"endpoint":       "set_queue",
		"next_tracks":    next,
		"prev_tracks":    prev,
		"queue_revision": c.PlayerState.QueueRevision,
		"logging_params": map[string]any{},
	}
	if len(c.PlayerState.QueueRevision) == 0 {
		cmd["queue_revision"] = ""
	}
	_, err := s.request(ctx, http.MethodPost,
		"/connect-state/v1/player/command/from/"+s.deviceID+"/to/"+url.PathEscape(c.ActiveDeviceID),
		map[string]any{"command": cmd})
	return err
}

// SkipTo, çalan cihazı sıradaki t satırına atlatır (aradakiler geçilir).
func (s *Session) SkipTo(ctx context.Context, t Track) error {
	c := s.cluster
	if c.ActiveDeviceID == "" {
		return ErrNoPlayer
	}
	cmd := map[string]any{
		"endpoint":       "skip_next",
		"track":          map[string]any{"uri": t.URI, "uid": t.UID},
		"logging_params": map[string]any{},
	}
	_, err := s.request(ctx, http.MethodPost,
		"/connect-state/v1/player/command/from/"+s.deviceID+"/to/"+url.PathEscape(c.ActiveDeviceID),
		map[string]any{"command": cmd})
	return err
}

// Close, gözlemci kaydını siler ve bağlantıyı kapatır.
func (s *Session) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _ = s.request(ctx, http.MethodDelete, "/connect-state/v1/devices/"+s.deviceID, nil)
	s.closeConn()
}

func (s *Session) closeConn() {
	if s.stop != nil {
		s.stop()
	}
	s.conn.Close(websocket.StatusNormalClosure, "")
}

// awaitConnectionID, dealer'ın ilk mesajındaki bağlantı kimliğini bekler.
func (s *Session) awaitConnectionID(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for {
		_, raw, err := s.conn.Read(ctx)
		if err != nil {
			return "", fmt.Errorf("connect: dealer: %w", err)
		}
		var msg struct {
			Type    string            `json:"type"`
			URI     string            `json:"uri"`
			Headers map[string]string `json:"headers"`
		}
		if json.Unmarshal(raw, &msg) != nil || !strings.HasPrefix(msg.URI, "hm://pusher/v1/connections/") {
			continue
		}
		for k, v := range msg.Headers {
			if strings.EqualFold(k, "Spotify-Connection-Id") && v != "" {
				return v, nil
			}
		}
	}
}

// drain, dealer'dan gelenleri okur (küme güncellemeleri burada önemsenmez)
// ve 30 sn'de bir ping atar.
func (s *Session) drain(ctx context.Context) {
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_ = s.conn.Write(ctx, websocket.MessageText, []byte(`{"type":"ping"}`))
			}
		}
	}()
	for {
		if _, _, err := s.conn.Read(ctx); err != nil {
			return
		}
	}
}

func (s *Session) request(ctx context.Context, method, path string, body any) ([]byte, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://"+s.spclient+path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("X-Spotify-Connection-Id", s.connID)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	data = gunzip(data)
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, ErrUnauthorized
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		var why struct {
			Type    string `json:"error_type"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(data, &why)
		msg := strings.TrimSpace(why.Type + " " + why.Message)
		if msg == "" {
			msg = strings.TrimSpace(string(data))
			if len(msg) > 200 {
				msg = msg[:200]
			}
		}
		return nil, fmt.Errorf("connect: %s %s: %s %s", method, strings.SplitN(path, "/", 5)[3], resp.Status, msg)
	}
	return data, nil
}

// resolve, dealer ve spclient adreslerini Spotify'a sorar; olmazsa varsayılanlar.
func resolve(ctx context.Context) (dealer, sp string) {
	dealer, sp = dealerHost, spclientHost
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resolveURL, nil)
	if err != nil {
		return
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	var r struct {
		Dealer   []string `json:"dealer"`
		Spclient []string `json:"spclient"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&r) != nil {
		return
	}
	host := func(list []string, def string) string {
		if len(list) == 0 || list[0] == "" {
			return def
		}
		return strings.TrimSuffix(list[0], ":443")
	}
	return host(r.Dealer, dealer), host(r.Spclient, sp)
}

func gunzip(b []byte) []byte {
	if len(b) < 2 || b[0] != 0x1f || b[1] != 0x8b {
		return b
	}
	r, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return b
	}
	defer r.Close()
	out, err := io.ReadAll(io.LimitReader(r, 8<<20))
	if err != nil {
		return b
	}
	return out
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
