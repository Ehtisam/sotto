package desktop

import (
	"fmt"
	"hash/fnv"
	"time"

	"github.com/godbus/dbus/v5"
)

// MPRIS: Linux'ta medya tuşları, masaüstünün medya kartı ve playerctl gibi
// araçlar oynatıcıları D-Bus'taki org.mpris.MediaPlayer2 arayüzünden bulur.
// Buradaki parçalar işletim sisteminden bağımsız (Windows'ta da sınanır);
// D-Bus bağlantısı media_linux.go'da.

const (
	mprisName   = "org.mpris.MediaPlayer2.sotto"
	mprisPath   = dbus.ObjectPath("/org/mpris/MediaPlayer2")
	mprisRoot   = "org.mpris.MediaPlayer2"
	mprisPlayer = "org.mpris.MediaPlayer2.Player"
	noTrack     = dbus.ObjectPath("/org/mpris/MediaPlayer2/TrackList/NoTrack")
)

// mprisState: kartta gösterilen şey. Konum, at anındaki pos'tur; çalarken
// o andan beri geçen süre eklenir (istemciler Position'ı arada bir okur).
type mprisState struct {
	track  Track
	status Status
	pos    time.Duration
	at     time.Time
}

func usec(d time.Duration) int64 { return d.Microseconds() }

// trackID: şarkının D-Bus nesne yolu (MPRIS her şarkı için bir yol ister).
func (s mprisState) trackID() dbus.ObjectPath {
	if s.track.Title == "" {
		return noTrack
	}
	h := fnv.New64a()
	fmt.Fprint(h, s.track.Title, "\x00", s.track.Artist, "\x00", s.track.Album)
	return dbus.ObjectPath(fmt.Sprintf("/org/sotto/track/%x", h.Sum64()))
}

// position: şu anki konum (çalarken ilerler, şarkı süresini geçmez).
func (s mprisState) position(now time.Time) time.Duration {
	p := s.pos
	if s.status == Playing && !s.at.IsZero() {
		p += now.Sub(s.at)
	}
	if d := s.track.Duration; d > 0 && p > d {
		p = d
	}
	return max(p, 0)
}

func (s mprisState) playbackStatus() string {
	switch s.status {
	case Playing:
		return "Playing"
	case Paused:
		return "Paused"
	}
	return "Stopped"
}

func (s mprisState) metadata() map[string]dbus.Variant {
	m := map[string]dbus.Variant{"mpris:trackid": dbus.MakeVariant(s.trackID())}
	t := s.track
	if t.Title == "" {
		return m
	}
	m["xesam:title"] = dbus.MakeVariant(t.Title)
	if t.Artist != "" {
		m["xesam:artist"] = dbus.MakeVariant([]string{t.Artist})
	}
	if t.Album != "" {
		m["xesam:album"] = dbus.MakeVariant(t.Album)
	}
	if t.Duration > 0 {
		m["mpris:length"] = dbus.MakeVariant(usec(t.Duration))
	}
	if t.CoverURL != "" {
		m["mpris:artUrl"] = dbus.MakeVariant(t.CoverURL)
	}
	return m
}

// props: bir arayüzün bütün özellikleri (GetAll).
func (s mprisState) props(iface string, now time.Time) map[string]dbus.Variant {
	v := dbus.MakeVariant
	switch iface {
	case mprisRoot:
		return map[string]dbus.Variant{
			"CanQuit": v(false), "CanRaise": v(false), "HasTrackList": v(false),
			"Identity": v("sotto"), "DesktopEntry": v("sotto"),
			"SupportedUriSchemes": v([]string{}), "SupportedMimeTypes": v([]string{}),
		}
	case mprisPlayer:
		has := s.track.Title != ""
		return map[string]dbus.Variant{
			"PlaybackStatus": v(s.playbackStatus()),
			"Rate":           v(1.0), "MinimumRate": v(1.0), "MaximumRate": v(1.0),
			"Volume":    v(1.0),
			"Metadata":  v(s.metadata()),
			"Position":  v(usec(s.position(now))),
			"CanGoNext": v(true), "CanGoPrevious": v(true),
			"CanPlay": v(true), "CanPause": v(true),
			"CanSeek": v(has && s.track.Duration > 0), "CanControl": v(true),
		}
	}
	return nil
}

// seekBy: Seek(offset) isteğinin sonucu. MPRIS'e göre şarkının sonunu
// geçen ileri sarma sonraki şarkıya geçer, başından önceye sarma başa döner.
func (s mprisState) seekBy(offset time.Duration, now time.Time) Event {
	target := s.position(now) + offset
	if d := s.track.Duration; d > 0 && target >= d {
		return Event{Button: Next}
	}
	return Event{Seek: max(target, 0), IsSeek: true}
}

// jumped: yeni konum, beklenenden belirgin biçimde farklıysa (sarıldıysa)
// MPRIS'in Seeked sinyali gönderilir.
func (s mprisState) jumped(pos time.Duration, now time.Time) bool {
	d := pos - s.position(now)
	return d > 1500*time.Millisecond || d < -1500*time.Millisecond
}
