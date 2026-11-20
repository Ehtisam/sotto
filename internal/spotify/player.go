package spotify

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// PlayerState, /me/player yanıtı: hangi cihazda ne çalıyor.
type PlayerState struct {
	Device struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Type   string `json:"type"`
		Volume *int   `json:"volume_percent"`
	} `json:"device"`
	ShuffleState bool   `json:"shuffle_state"`
	RepeatState  string `json:"repeat_state"` // off, context, track
	ProgressMS   int64  `json:"progress_ms"`
	IsPlaying    bool   `json:"is_playing"`
	Item         *Track `json:"item"`
	Context      *struct {
		URI  string `json:"uri"`
		Type string `json:"type"`
	} `json:"context"`
}

// Player, şu an etkin olan cihazın durumu. Hiçbir cihaz etkin değilse nil döner.
func (c *Client) Player(ctx context.Context) (*PlayerState, error) {
	var p PlayerState
	if err := c.get(ctx, "me/player?additional_types=track,episode", &p); err != nil {
		return nil, err
	}
	if p.Device.ID == "" && p.Item == nil {
		return nil, nil // 204: etkin cihaz yok
	}
	if p.Item != nil {
		c.remember(*p.Item)
	}
	return &p, nil
}

// Queue, etkin cihazın sırası.
func (c *Client) Queue(ctx context.Context) ([]Track, error) {
	var r struct {
		Queue []Track `json:"queue"`
	}
	err := c.get(ctx, "me/player/queue", &r)
	if err == nil {
		c.remember(r.Queue...)
	}
	return r.Queue, err
}

// ---- etkin cihaza komutlar ----

func (c *Client) Pause(ctx context.Context) error {
	return c.do(ctx, http.MethodPut, "me/player/pause", nil, nil)
}

func (c *Client) Resume(ctx context.Context) error {
	return c.do(ctx, http.MethodPut, "me/player/play", nil, nil)
}

// ResumeOn, verilen cihazda çalmayı sürdürür.
func (c *Client) ResumeOn(ctx context.Context, deviceID string) error {
	return c.do(ctx, http.MethodPut, "me/player/play"+deviceQuery(deviceID), nil, nil)
}

func (c *Client) Next(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "me/player/next", nil, nil)
}

func (c *Client) Previous(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "me/player/previous", nil, nil)
}

func (c *Client) Seek(ctx context.Context, positionMS int64) error {
	return c.do(ctx, http.MethodPut, fmt.Sprintf("me/player/seek?position_ms=%d", max(positionMS, 0)), nil, nil)
}

func (c *Client) SetVolume(ctx context.Context, percent int) error {
	return c.do(ctx, http.MethodPut, fmt.Sprintf("me/player/volume?volume_percent=%d", max(0, min(100, percent))), nil, nil)
}

func (c *Client) SetShuffle(ctx context.Context, on bool) error {
	return c.do(ctx, http.MethodPut, fmt.Sprintf("me/player/shuffle?state=%t", on), nil, nil)
}

func (c *Client) SetRepeat(ctx context.Context, mode string) error {
	return c.do(ctx, http.MethodPut, "me/player/repeat?state="+url.QueryEscape(mode), nil, nil)
}

// Transfer, çalmayı verilen cihaza aktarır.
func (c *Client) Transfer(ctx context.Context, deviceID string, play bool) error {
	return c.do(ctx, http.MethodPut, "me/player", map[string]any{"device_ids": []string{deviceID}, "play": play}, nil)
}
