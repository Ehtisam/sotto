package soloist

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// Client, Soloist'in WebSocket API'sine bağlı bir istemci.
type Client struct {
	conn   *websocket.Conn
	events chan any
	once   sync.Once
	cancel context.CancelFunc
}

// Dial, ws://addr adresine bağlanır ve olayları okumaya başlar.
func Dial(ctx context.Context, addr string) (*Client, error) {
	conn, _, err := websocket.Dial(ctx, "ws://"+addr, nil)
	if err != nil {
		return nil, fmt.Errorf("soloist'e bağlanılamadı: %w", err)
	}
	conn.SetReadLimit(4 << 20)
	rctx, cancel := context.WithCancel(context.Background())
	c := &Client{conn: conn, events: make(chan any, 64), cancel: cancel}
	go c.readLoop(rctx)
	return c, nil
}

// Events, sunucudan gelen olayları taşır. Bağlantı kapanınca son olarak
// Disconnected gelir ve kanal kapanır.
func (c *Client) Events() <-chan any { return c.events }

func (c *Client) readLoop(ctx context.Context) {
	defer close(c.events)
	for {
		var raw json.RawMessage
		if err := wsjson.Read(ctx, c.conn, &raw); err != nil {
			c.events <- Disconnected{Err: err}
			return
		}
		ev, err := decode(raw)
		if err != nil || ev == nil {
			continue
		}
		c.events <- ev
	}
}

func decode(raw json.RawMessage) (any, error) {
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return nil, err
	}
	var v any
	switch head.Type {
	case "auth_state":
		v = &AuthState{}
	case "playback_state":
		v = &PlaybackState{}
	case "track_changed":
		v = &TrackChanged{}
	case "playback_changed":
		v = &StatusChanged{}
	case "volume_changed":
		v = &VolumeChanged{}
	case "device_changed":
		v = &DeviceChanged{}
	case "context_changed":
		v = &ContextChanged{}
	case "options_changed":
		v = &OptionsChanged{}
	case "position_sync":
		v = &PositionSync{}
	case "queue_changed":
		v = &QueueChanged{}
	case "error":
		v = &APIError{}
	default: // command_result ve bilinmeyenler
		return nil, nil
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return nil, err
	}
	// İşaretçiyi değere çevir: alıcılar türe göre switch yapabilsin.
	switch t := v.(type) {
	case *AuthState:
		return *t, nil
	case *PlaybackState:
		return *t, nil
	case *TrackChanged:
		return *t, nil
	case *StatusChanged:
		return *t, nil
	case *VolumeChanged:
		return *t, nil
	case *DeviceChanged:
		return *t, nil
	case *ContextChanged:
		return *t, nil
	case *OptionsChanged:
		return *t, nil
	case *PositionSync:
		return *t, nil
	case *QueueChanged:
		return *t, nil
	case *APIError:
		return *t, nil
	}
	return nil, nil
}

// Close, bağlantıyı kapatır.
func (c *Client) Close() {
	c.once.Do(func() {
		c.cancel()
		_ = c.conn.Close(websocket.StatusNormalClosure, "")
	})
}

func (c *Client) send(ctx context.Context, cmd string, fields map[string]any) error {
	msg := map[string]any{"type": "command", "command": cmd}
	for k, v := range fields {
		msg[k] = v
	}
	return wsjson.Write(ctx, c.conn, msg)
}

// ---- Sorgular ----

func (c *Client) RequestAuthState(ctx context.Context) error {
	return c.send(ctx, "get_auth_state", nil)
}
func (c *Client) RequestState(ctx context.Context) error { return c.send(ctx, "get_state", nil) }
func (c *Client) RequestQueue(ctx context.Context, limit int) error {
	return c.send(ctx, "get_queue", map[string]any{"limit": limit})
}

// ---- Komutlar ----

// Play, çalmayı sürdürür; uri verilirse o şarkıyı/listeyi/albümü başlatır.
func (c *Client) Play(ctx context.Context, uri string) error {
	if uri == "" {
		return c.send(ctx, "play", nil)
	}
	return c.send(ctx, "play", map[string]any{"uri": uri})
}

func (c *Client) Pause(ctx context.Context) error    { return c.send(ctx, "pause", nil) }
func (c *Client) Next(ctx context.Context) error     { return c.send(ctx, "skip_next", nil) }
func (c *Client) Previous(ctx context.Context) error { return c.send(ctx, "skip_prev", nil) }
func (c *Client) Activate(ctx context.Context) error { return c.send(ctx, "activate", nil) }

func (c *Client) Seek(ctx context.Context, positionMS int64) error {
	if positionMS < 0 {
		positionMS = 0
	}
	return c.send(ctx, "seek", map[string]any{"position_ms": positionMS})
}

func (c *Client) SetVolume(ctx context.Context, volume int) error {
	volume = max(0, min(100, volume))
	return c.send(ctx, "set_volume", map[string]any{"volume": volume})
}

func (c *Client) SetShuffle(ctx context.Context, on bool) error {
	return c.send(ctx, "set_shuffle", map[string]any{"enabled": on})
}

// SetRepeat, "off", "context" ya da "track". Belgede önerilen sırayla iki komut gönderir.
func (c *Client) SetRepeat(ctx context.Context, mode string) error {
	switch mode {
	case "track":
		if err := c.send(ctx, "set_repeat_context", map[string]any{"enabled": false}); err != nil {
			return err
		}
		return c.send(ctx, "set_repeat_track", map[string]any{"enabled": true})
	case "context":
		if err := c.send(ctx, "set_repeat_track", map[string]any{"enabled": false}); err != nil {
			return err
		}
		return c.send(ctx, "set_repeat_context", map[string]any{"enabled": true})
	default:
		if err := c.send(ctx, "set_repeat_track", map[string]any{"enabled": false}); err != nil {
			return err
		}
		return c.send(ctx, "set_repeat_context", map[string]any{"enabled": false})
	}
}

func (c *Client) AddToQueue(ctx context.Context, trackURI string) error {
	return c.send(ctx, "add_to_queue", map[string]any{"uri": trackURI})
}
