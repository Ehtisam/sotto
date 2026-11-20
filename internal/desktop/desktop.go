// Package desktop, sotto'yu işletim sistemine bağlar: şarkı değişince
// bildirim, medya tuşları ve Windows'un medya denetimi (ses kutusundaki şarkı
// kartı, kilit ekranı, kulaklık düğmeleri).
//
// Windows'ta hepsi WinRT ile doğrudan yapılır (PowerShell açılmaz). Başka
// sistemlerde bildirim notify-send ile gönderilir; medya denetimi yoktur.
package desktop

import (
	"errors"
	"time"
)

// AppID (appid_gen.go), Windows'un bildirimlerde ve medya kartında sotto'yu
// tanıdığı ad: "Ehtisam.sotto." + simgenin parmak izi. Windows her ad için
// ilk gördüğü simgeyi önbelleğe alıp bir daha okumuyor (ilk ad ters renkli bir
// simgeye takıldı); simge değişince ad da değişsin diye go run ./tools/mkicon
// adı yeniden yazar. Eski adların kayıtları açılışta silinir (CleanupOld).

// appIDPrefix: sotto'nun bütün eski ve yeni adları bununla başlar.
const appIDPrefix = "Ehtisam.sotto"

// ErrUnsupported: bu sistemde yok.
var ErrUnsupported = errors.New("not supported on this system")

// Button, medya tuşlarından ya da Windows'un medya kartından gelen istek.
type Button int

const (
	Play Button = iota
	Pause
	Stop
	Next
	Previous
	Toggle // tek tuşlu çal/durdur (Windows bunu Play ya da Pause olarak yollar; başka sistemler için)
)

// Event, medya denetiminden gelen bir istek: bir düğme ya da sarma.
type Event struct {
	Button Button
	Seek   time.Duration // Seek >= 0 ve IsSeek: bu konuma sar
	IsSeek bool
}

// Track, medya kartında gösterilen şarkı.
type Track struct {
	Title, Artist, Album string
	CoverURL             string // https adresi
	Duration             time.Duration
}

// Status, çalma durumu.
type Status int

const (
	Closed Status = iota // kart gizli
	Stopped
	Playing
	Paused
)

// Note, bir bildirim.
type Note struct {
	Title, Body string
	Image       string // yerel dosya (jpg/png); "" olabilir
}
