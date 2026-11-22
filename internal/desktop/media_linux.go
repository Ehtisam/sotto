//go:build linux

package desktop

import (
	"errors"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
)

// Media, MPRIS ile masaüstünün medya kartı ve medya tuşları (mpris.go).
type Media struct {
	conn   *dbus.Conn
	events chan Event

	mu     sync.Mutex
	state  mprisState
	closed bool
}

// OpenMedia, oturum veri yoluna bağlanır ve org.mpris.MediaPlayer2.sotto
// adını alır. Veri yolu yoksa (masaüstüsüz sistem) ErrUnsupported.
func OpenMedia() (*Media, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, ErrUnsupported
	}
	m := &Media{conn: conn, events: make(chan Event, 16)}
	root, player, props := mprisRootObj{m}, mprisPlayerObj{m}, mprisPropsObj{m}
	// Go'da Seek adı io.Seeker'a ayrılmış (go vet); D-Bus'ta yine Seek.
	rename := map[string]string{"SeekBy": "Seek"}
	for _, e := range []struct {
		v     any
		iface string
	}{{root, mprisRoot}, {player, mprisPlayer}, {props, "org.freedesktop.DBus.Properties"}} {
		if err := conn.ExportWithMap(e.v, rename, mprisPath, e.iface); err != nil {
			conn.Close()
			return nil, err
		}
	}
	node := &introspect.Node{Name: string(mprisPath), Interfaces: []introspect.Interface{
		introspect.IntrospectData,
		prop.IntrospectData,
		{Name: mprisRoot, Methods: introspect.Methods(root), Properties: readOnly(m.state.props(mprisRoot, time.Now()))},
		{Name: mprisPlayer, Methods: renamed(introspect.Methods(player), rename), Properties: readOnly(m.state.props(mprisPlayer, time.Now())),
			Signals: []introspect.Signal{{Name: "Seeked", Args: []introspect.Arg{{Name: "Position", Type: "x"}}}}},
	}}
	if err := conn.Export(introspect.NewIntrospectable(node), mprisPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		conn.Close()
		return nil, err
	}
	reply, err := conn.RequestName(mprisName, dbus.NameFlagDoNotQueue)
	if err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		conn.Close()
		return nil, errors.New("another sotto already shows the media card")
	}
	return m, nil
}

func renamed(ms []introspect.Method, to map[string]string) []introspect.Method {
	for i := range ms {
		if n, ok := to[ms[i].Name]; ok {
			ms[i].Name = n
		}
	}
	return ms
}

func readOnly(p map[string]dbus.Variant) []introspect.Property {
	out := make([]introspect.Property, 0, len(p))
	for name, v := range p {
		out = append(out, introspect.Property{Name: name, Type: v.Signature().String(), Access: "read"})
	}
	return out
}

func (m *Media) Events() <-chan Event { return m.events }

func (m *Media) send(ev Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return // kapalı kanala yazılmasın
	}
	select {
	case m.events <- ev:
	default: // sotto meşgulse basış düşer; bir sonraki gelir
	}
}

// changed, PropertiesChanged sinyalini gönderir.
func (m *Media) changed(names ...string) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	all := m.state.props(mprisPlayer, time.Now())
	m.mu.Unlock()
	out := map[string]dbus.Variant{}
	for _, n := range names {
		out[n] = all[n]
	}
	return m.conn.Emit(mprisPath, "org.freedesktop.DBus.Properties.PropertiesChanged", mprisPlayer, out, []string{})
}

func (m *Media) SetTrack(t Track) error {
	m.mu.Lock()
	m.state.track = t
	m.mu.Unlock()
	return m.changed("Metadata", "CanSeek")
}

func (m *Media) SetStatus(s Status) error {
	m.mu.Lock()
	now := time.Now()
	m.state.pos, m.state.at = m.state.position(now), now // duraklayınca konum donsun
	m.state.status = s
	m.mu.Unlock()
	return m.changed("PlaybackStatus")
}

func (m *Media) SetPosition(pos, dur time.Duration) error {
	m.mu.Lock()
	now := time.Now()
	jumped := m.state.jumped(pos, now)
	m.state.pos, m.state.at = pos, now
	if dur > 0 {
		m.state.track.Duration = dur
	}
	closed := m.closed
	m.mu.Unlock()
	if jumped && !closed {
		return m.conn.Emit(mprisPath, mprisPlayer+".Seeked", usec(pos))
	}
	return nil
}

func (m *Media) Close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	close(m.events)
	m.mu.Unlock()
	_, _ = m.conn.ReleaseName(mprisName)
	m.conn.Close()
}

// ---- D-Bus'a açılan nesneler ----

type mprisRootObj struct{ m *Media }

func (mprisRootObj) Raise() *dbus.Error { return nil }
func (mprisRootObj) Quit() *dbus.Error  { return nil }

type mprisPlayerObj struct{ m *Media }

func (p mprisPlayerObj) Next() *dbus.Error      { p.m.send(Event{Button: Next}); return nil }
func (p mprisPlayerObj) Previous() *dbus.Error  { p.m.send(Event{Button: Previous}); return nil }
func (p mprisPlayerObj) Pause() *dbus.Error     { p.m.send(Event{Button: Pause}); return nil }
func (p mprisPlayerObj) PlayPause() *dbus.Error { p.m.send(Event{Button: Toggle}); return nil }
func (p mprisPlayerObj) Stop() *dbus.Error      { p.m.send(Event{Button: Stop}); return nil }
func (p mprisPlayerObj) Play() *dbus.Error      { p.m.send(Event{Button: Play}); return nil }

func (p mprisPlayerObj) SeekBy(offset int64) *dbus.Error {
	p.m.mu.Lock()
	ev := p.m.state.seekBy(time.Duration(offset)*time.Microsecond, time.Now())
	p.m.mu.Unlock()
	p.m.send(ev)
	return nil
}

func (p mprisPlayerObj) SetPosition(id dbus.ObjectPath, pos int64) *dbus.Error {
	p.m.mu.Lock()
	cur, dur := p.m.state.trackID(), p.m.state.track.Duration
	p.m.mu.Unlock()
	d := time.Duration(pos) * time.Microsecond
	if id != cur || d < 0 || (dur > 0 && d > dur) {
		return nil // MPRIS: eski şarkı ya da aralık dışı istek yok sayılır
	}
	p.m.send(Event{Seek: d, IsSeek: true})
	return nil
}

func (mprisPlayerObj) OpenUri(string) *dbus.Error { return nil }

// mprisPropsObj: org.freedesktop.DBus.Properties. Position okunduğu anda
// hesaplandığı için hazır prop paketi yerine elle.
type mprisPropsObj struct{ m *Media }

func (p mprisPropsObj) Get(iface, name string) (dbus.Variant, *dbus.Error) {
	all, err := p.GetAll(iface)
	if err != nil {
		return dbus.Variant{}, err
	}
	v, ok := all[name]
	if !ok {
		return dbus.Variant{}, prop.ErrPropNotFound
	}
	return v, nil
}

func (p mprisPropsObj) GetAll(iface string) (map[string]dbus.Variant, *dbus.Error) {
	p.m.mu.Lock()
	all := p.m.state.props(iface, time.Now())
	p.m.mu.Unlock()
	if all == nil {
		return nil, prop.ErrIfaceNotFound
	}
	return all, nil
}

func (mprisPropsObj) Set(string, string, dbus.Variant) *dbus.Error {
	return prop.ErrReadOnly
}
