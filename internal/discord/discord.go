// Package discord, Discord'un yerel RPC'si üzerinden "şu an dinliyor"
// durumunu ayarlar. Discord masaüstü uygulaması açıksa bir adlandırılmış
// boru (Windows) ya da unix soketi dinler: discord-ipc-0 … 9. Mesajlar
// 8 baytlık bir başlık (işlem, uzunluk; little-endian) ve JSON'dur.
// Durum, Discord geliştirici sitesinde oluşturulan bir uygulamanın kimliğiyle
// gösterilir ("Listening to <uygulamanın adı>").
package discord

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Activity, gösterilecek durum. Boşsa (Title "") durum temizlenir.
type Activity struct {
	Title, Artist, Album string
	Cover                string    // https adresi (Discord dışarıdan resim kabul ediyor)
	Start, End           time.Time // çalıyorsa; ilerleme çubuğu için
	URL                  string    // şarkının open.spotify.com adresi (düğme)
}

const (
	opHandshake = 0
	opFrame     = 1
	opClose     = 2
)

// Presence, arka planda Discord'a bağlı kalan ve son durumu gönderen.
// Discord kapalıysa birkaç saniyede bir yeniden dener; hata göstermez.
type Presence struct {
	appID string
	mu    sync.Mutex
	want  *Activity // gönderilecek son durum (nil: temizle)
	dirty bool
	wake  chan struct{}
	stop  chan struct{}
	done  chan struct{}
	Err   func(error) // isteğe bağlı: günlüğe yazmak için
}

// Start, appID için bir Presence başlatır.
func Start(appID string) *Presence {
	p := &Presence{appID: appID, wake: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{})}
	go p.loop()
	return p
}

// Set, durumu ayarlar (nil: temizle). Bekletmez.
func (p *Presence) Set(a *Activity) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.want, p.dirty = a, true
	p.mu.Unlock()
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// Close, durumu temizler ve bağlantıyı kapatır.
func (p *Presence) Close() {
	if p == nil {
		return
	}
	select {
	case <-p.stop:
	default:
		close(p.stop)
	}
	<-p.done
}

// dial, testlerde değiştirilir.
var dial = dialIPC

func (p *Presence) loop() {
	defer close(p.done)
	var conn io.ReadWriteCloser
	retry := time.NewTimer(0)
	defer retry.Stop()
	for {
		select {
		case <-p.stop:
			if conn != nil {
				// Durumu temizle; Discord'un yanıtını en çok 2 sn bekle.
				c := conn
				cleared := make(chan struct{})
				go func() {
					if send(c, opFrame, setActivity(nil)) == nil {
						_, _, _ = read(c)
					}
					close(cleared)
				}()
				select {
				case <-cleared:
				case <-time.After(2 * time.Second):
				}
				c.Close()
			}
			return
		case <-p.wake:
		case <-retry.C:
		}
		if conn == nil {
			c, err := dial()
			if err != nil {
				retry.Reset(15 * time.Second) // Discord kapalı: sonra yine dene
				continue
			}
			if err := handshake(c, p.appID); err != nil {
				c.Close()
				p.report(err)
				retry.Reset(30 * time.Second)
				continue
			}
			conn = c
			p.mu.Lock()
			p.dirty = true // yeni bağlantıda son durumu yeniden gönder
			p.mu.Unlock()
		}
		p.mu.Lock()
		a, dirty := p.want, p.dirty
		p.dirty = false
		p.mu.Unlock()
		if !dirty {
			continue
		}
		if err := send(conn, opFrame, setActivity(a)); err == nil {
			_, _, err = read(conn)
		} else {
			conn.Close()
			conn = nil
			p.mu.Lock()
			p.dirty = true
			p.mu.Unlock()
			retry.Reset(5 * time.Second)
		}
	}
}

func (p *Presence) report(err error) {
	if p.Err != nil {
		p.Err(err)
	}
}

func handshake(c io.ReadWriter, appID string) error {
	if err := send(c, opHandshake, map[string]any{"v": 1, "client_id": appID}); err != nil {
		return err
	}
	op, body, err := read(c)
	if err != nil {
		return err
	}
	if op == opClose {
		var e struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(body, &e)
		return fmt.Errorf("discord: %s", e.Message)
	}
	return nil
}

// setActivity, SET_ACTIVITY isteği (Activity nil ya da boşsa temizler).
func setActivity(a *Activity) map[string]any {
	var act any
	if a != nil && a.Title != "" {
		v := map[string]any{
			"type":    2, // dinliyor
			"details": clip(a.Title),
		}
		if a.Artist != "" {
			v["state"] = clip(a.Artist)
		}
		if !a.Start.IsZero() {
			ts := map[string]any{"start": a.Start.UnixMilli()}
			if !a.End.IsZero() {
				ts["end"] = a.End.UnixMilli()
			}
			v["timestamps"] = ts
		}
		assets := map[string]any{}
		if a.Cover != "" {
			assets["large_image"] = a.Cover
		}
		if a.Album != "" {
			assets["large_text"] = clip(a.Album)
		}
		if len(assets) > 0 {
			v["assets"] = assets
		}
		if a.URL != "" {
			v["buttons"] = []map[string]string{{"label": "Open in Spotify", "url": a.URL}}
		}
		act = v
	}
	return map[string]any{
		"cmd":   "SET_ACTIVITY",
		"args":  map[string]any{"pid": os.Getpid(), "activity": act},
		"nonce": nonce(),
	}
}

// clip: Discord alanları 2–128 karakter ister.
func clip(s string) string {
	r := []rune(s)
	if len(r) > 128 {
		r = append(r[:127], '…')
	}
	if len(r) < 2 {
		return string(r) + " "
	}
	return string(r)
}

func nonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func send(w io.Writer, op uint32, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	head := make([]byte, 8)
	binary.LittleEndian.PutUint32(head[0:], op)
	binary.LittleEndian.PutUint32(head[4:], uint32(len(body)))
	_, err = w.Write(append(head, body...))
	return err
}

var errFrame = errors.New("discord: bad frame")

func read(r io.Reader) (uint32, []byte, error) {
	head := make([]byte, 8)
	if _, err := io.ReadFull(r, head); err != nil {
		return 0, nil, err
	}
	op, n := binary.LittleEndian.Uint32(head[0:]), binary.LittleEndian.Uint32(head[4:])
	if n > 1<<20 {
		return 0, nil, errFrame
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return 0, nil, err
	}
	return op, body, nil
}
