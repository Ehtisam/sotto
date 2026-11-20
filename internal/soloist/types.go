// Package soloist, Spotify Soloist'i (Spotify'ın resmî, arayüzsüz Connect
// motoru) başlatır, kapatır ve yerel WebSocket API'si üzerinden yönetir.
//
// API belgesi: https://developer.spotify.com/documentation/soloist/reference/websocket-api
package soloist

import (
	"strings"
	"time"
)

// Entity, Soloist'in şarkı, bölüm, albüm, liste gibi her şey için kullandığı ortak zarf.
type Entity struct {
	URI         string      `json:"uri"`
	EntityType  string      `json:"entity_type"`
	Decorations Decorations `json:"decorations"`
}

type Decorations struct {
	Identity struct {
		Name string `json:"name"`
	} `json:"identity"`
	VisualIdentity struct {
		Cover []Image `json:"cover"`
	} `json:"visual_identity"`
	Parent *struct {
		Entity Entity `json:"entity"`
	} `json:"parent"`
	Creators []struct {
		Entity Entity `json:"entity"`
	} `json:"creators"`
	Playback struct {
		DurationMS     int64    `json:"duration_ms"`
		ContentRatings []string `json:"content_ratings"`
	} `json:"playback"`
}

type Image struct {
	URL  string `json:"url"`
	Size string `json:"size"`
}

// Name, öğenin görünen adı ("" olabilir).
func (e *Entity) Name() string {
	if e == nil {
		return ""
	}
	return e.Decorations.Identity.Name
}

// URIOrEmpty, öğe yoksa "" döner.
func (e *Entity) URIOrEmpty() string {
	if e == nil {
		return ""
	}
	return e.URI
}

// Creators, sanatçı ya da yayıncı adlarını virgülle birleştirir.
func (e *Entity) Creators() string {
	if e == nil {
		return ""
	}
	names := make([]string, 0, len(e.Decorations.Creators))
	for _, c := range e.Decorations.Creators {
		if n := c.Entity.Name(); n != "" {
			names = append(names, n)
		}
	}
	return strings.Join(names, ", ")
}

// ParentName, şarkının albümü ya da bölümün podcast'i.
func (e *Entity) ParentName() string {
	if e == nil || e.Decorations.Parent == nil {
		return ""
	}
	return e.Decorations.Parent.Entity.Name()
}

func (e *Entity) Duration() time.Duration {
	if e == nil {
		return 0
	}
	return time.Duration(e.Decorations.Playback.DurationMS) * time.Millisecond
}

func (e *Entity) Explicit() bool {
	if e == nil {
		return false
	}
	for _, r := range e.Decorations.Playback.ContentRatings {
		if r == "explicit" {
			return true
		}
	}
	return false
}

// Position, çalma konumunun bir andaki çapası. Aradaki zaman yerelde hesaplanır.
type Position struct {
	PositionMS  int64   `json:"position_ms"`
	TimestampMS int64   `json:"timestamp_ms"`
	Speed       float64 `json:"speed"`
}

// At, verilen andaki tahmini konum.
func (p Position) At(now time.Time) time.Duration {
	ms := float64(p.PositionMS)
	if p.Speed > 0 && p.TimestampMS > 0 {
		ms += float64(now.UnixMilli()-p.TimestampMS) * p.Speed
	}
	if ms < 0 {
		ms = 0
	}
	return time.Duration(ms) * time.Millisecond
}

type PlaybackOptions struct {
	Shuffle bool   `json:"shuffle"`
	Repeat  string `json:"repeat"` // off, context, track
}

// Durum değerleri.
const (
	StatusIdle      = "idle"
	StatusPlaying   = "playing"
	StatusPaused    = "paused"
	StatusBuffering = "buffering"
)

// ---- Olaylar (sunucudan gelen) ----

type AuthState struct {
	LoggedIn   bool   `json:"logged_in"`
	IsActive   bool   `json:"is_active"`
	DeviceName string `json:"device_name"`
}

type PlaybackState struct {
	Status   string          `json:"status"`
	Item     *Entity         `json:"item"`
	Context  *Entity         `json:"context"`
	Position Position        `json:"position"`
	Volume   int             `json:"volume"`
	IsActive bool            `json:"is_active"`
	Options  PlaybackOptions `json:"options"`
}

type TrackChanged struct {
	Item *Entity `json:"item"`
}

type StatusChanged struct {
	Status string `json:"status"`
}

type VolumeChanged struct {
	Volume int `json:"volume"`
}

type DeviceChanged struct {
	IsActive   bool   `json:"is_active"`
	DeviceName string `json:"device_name"`
}

type ContextChanged struct {
	Context *Entity `json:"context"`
}

type OptionsChanged struct {
	Options PlaybackOptions `json:"options"`
}

type PositionSync struct {
	Position Position `json:"position"`
}

type QueueEntry struct {
	UID    string  `json:"uid"`
	Source string  `json:"source"` // context, queue, autoplay
	Item   *Entity `json:"item"`
}

type QueueChanged struct {
	Previous []QueueEntry `json:"previous"`
	Upcoming []QueueEntry `json:"upcoming"`
}

// APIError, Soloist'in bir komutu reddettiğini bildirir.
type APIError struct {
	Message string `json:"message"`
}

func (e APIError) Error() string { return "soloist: " + e.Message }

// Disconnected, WebSocket bağlantısı kapanınca gelir.
type Disconnected struct {
	Err error
}
