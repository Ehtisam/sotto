package desktop

import (
	"strings"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

func TestMprisState(t *testing.T) {
	now := time.Now()
	s := mprisState{}
	if s.trackID() != noTrack || s.playbackStatus() != "Stopped" {
		t.Fatal("empty state")
	}
	if len(s.metadata()) != 1 {
		t.Fatalf("empty metadata %v", s.metadata())
	}
	s.track = Track{Title: "Olmaz", Artist: "Malt", Album: "Arıza", Duration: 230 * time.Second, CoverURL: "https://i.scdn.co/x"}
	md := s.metadata()
	if md["xesam:title"].Value() != "Olmaz" || md["mpris:length"].Value() != int64(230_000_000) ||
		md["mpris:artUrl"].Value() != "https://i.scdn.co/x" {
		t.Fatalf("metadata %v", md)
	}
	if a, _ := md["xesam:artist"].Value().([]string); len(a) != 1 || a[0] != "Malt" {
		t.Fatalf("artist %v", md["xesam:artist"])
	}
	id := s.trackID()
	if !id.IsValid() || !strings.HasPrefix(string(id), "/org/sotto/track/") {
		t.Fatalf("track id %q", id)
	}
	// Çalarken konum ilerler, duraklayınca durur, süreyi geçmez.
	s.status, s.pos, s.at = Playing, 10*time.Second, now.Add(-5*time.Second)
	if p := s.position(now); p < 14900*time.Millisecond || p > 15100*time.Millisecond {
		t.Fatalf("playing position %v", p)
	}
	s.status = Paused
	if s.position(now) != 10*time.Second {
		t.Fatal("paused position moved")
	}
	s.status, s.at = Playing, now.Add(-time.Hour)
	if s.position(now) != 230*time.Second {
		t.Fatal("position past the end")
	}
	// Sarma: ileri, geri (başa kadar), sonu geçince sonraki şarkı.
	s.status, s.pos, s.at = Paused, 60*time.Second, now
	if ev := s.seekBy(10*time.Second, now); !ev.IsSeek || ev.Seek != 70*time.Second {
		t.Fatalf("seek +10 %v", ev)
	}
	if ev := s.seekBy(-90*time.Second, now); !ev.IsSeek || ev.Seek != 0 {
		t.Fatalf("seek -90 %v", ev)
	}
	if ev := s.seekBy(200*time.Second, now); ev.IsSeek || ev.Button != Next {
		t.Fatalf("seek past end %v", ev)
	}
	if !s.jumped(90*time.Second, now) || s.jumped(61*time.Second, now) {
		t.Fatal("jump detection")
	}
	// Özellikler: istemcilerin beklediği adlar ve D-Bus türleri.
	p := s.props(mprisPlayer, now)
	want := map[string]string{"PlaybackStatus": "s", "Metadata": "a{sv}", "Position": "x", "Rate": "d",
		"Volume": "d", "CanSeek": "b", "CanControl": "b", "CanGoNext": "b"}
	for name, sig := range want {
		if got := p[name].Signature().String(); got != sig {
			t.Errorf("%s: %q, want %q", name, got, sig)
		}
	}
	r := s.props(mprisRoot, now)
	if r["Identity"].Value() != "sotto" || r["DesktopEntry"].Value() != "sotto" {
		t.Fatalf("root %v", r)
	}
	if s.props("x", now) != nil {
		t.Fatal("unknown interface")
	}
	_ = dbus.ObjectPath("")
}

func TestDesktopEntry(t *testing.T) {
	e := desktopEntry("/home/sols/sotto")
	for _, line := range []string{"[Desktop Entry]", "Exec=/home/sols/sotto", "Icon=sotto", "Terminal=true", "Name=sotto"} {
		if !strings.Contains(e, line+"\n") {
			t.Errorf("missing %q in\n%s", line, e)
		}
	}
	for _, p := range []string{"/home/sols/sotto", "/home/a b/my $dir/so`t\"to", `/opt/x\y/sotto`} {
		q := execQuote(p)
		if back := execTarget("Exec=" + q); back != p {
			t.Errorf("%q → %q → %q", p, q, back)
		}
	}
	if q := execQuote("/home/a b/sotto"); q != `"/home/a b/sotto"` {
		t.Errorf("space: %s", q)
	}
}
