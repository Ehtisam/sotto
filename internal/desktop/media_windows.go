//go:build windows

package desktop

import (
	"time"
	"unsafe"
)

// Media, Windows'un medya denetimi (System Media Transport Controls): ses
// kutusundaki ve kilit ekranındaki şarkı kartı, klavyenin ve kulaklığın medya
// tuşları. Windows medya tuşlarını en son etkin olan karta yollar.
type Media struct {
	w       *worker
	smtc    com
	smtc2   com
	updater com
	events  chan Event
}

func uintptrOf[T any](p *T) uintptr { return uintptr(unsafe.Pointer(p)) }

// ticks, süreyi WinRT'nin TimeSpan birimine (100 ns) çevirir.
func ticks(d time.Duration) uintptr { return uintptr(d.Nanoseconds() / 100) }

// OpenMedia, sotto'yu Windows'un medya denetimine bağlar.
func OpenMedia() (*Media, error) {
	w, err := getWorker()
	if err != nil {
		return nil, err
	}
	m := &Media{w: w, events: make(chan Event, 16)}
	err = w.do(func() error {
		interop, err := factory("Windows.Media.SystemMediaTransportControls", &iidSMTCInterop)
		if err != nil {
			return err
		}
		defer interop.release()
		if err := interop.call(6, w.hwnd, uintptrOf(&iidSMTC), uintptrOf(&m.smtc)); err != nil { // GetForWindow
			return err
		}
		for _, i := range []int{11, 13, 17, 25, 27} { // IsEnabled, Play, Pause, Previous, Next
			if err := m.smtc.call(i, 1); err != nil {
				return err
			}
		}
		if m.updater, err = m.smtc.getObj(8); err != nil { // get_DisplayUpdater
			return err
		}
		_ = m.updater.call(7, 1) // put_Type(Music)

		buttons := newDelegate(iidButtonHandler, func(_, args com) {
			var b int32
			if args.call(6, uintptrOf(&b)) != nil { // get_Button
				return
			}
			ev := Event{}
			switch b {
			case 0:
				ev.Button = Play
			case 1:
				ev.Button = Pause
			case 2:
				ev.Button = Stop
			case 6:
				ev.Button = Next
			case 7:
				ev.Button = Previous
			default:
				return
			}
			m.send(ev)
		})
		var token int64
		if err := m.smtc.call(32, uintptr(unsafe.Pointer(buttons)), uintptrOf(&token)); err != nil { // add_ButtonPressed
			return err
		}
		if m.smtc2, err = m.smtc.query(&iidSMTC2); err == nil {
			seek := newDelegate(iidSeekHandler, func(_, args com) {
				var t int64
				if args.call(6, uintptrOf(&t)) == nil { // get_RequestedPlaybackPosition
					m.send(Event{IsSeek: true, Seek: time.Duration(t) * 100})
				}
			})
			_ = m.smtc2.call(13, uintptr(unsafe.Pointer(seek)), uintptrOf(&token)) // add_PlaybackPositionChangeRequested
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Media) send(ev Event) {
	select {
	case m.events <- ev:
	default:
	}
}

// Events, medya tuşlarından ve karttan gelen istekler.
func (m *Media) Events() <-chan Event { return m.events }

// SetTrack, kartta gösterilen şarkıyı değiştirir.
func (m *Media) SetTrack(t Track) error {
	return m.w.do(func() error {
		music, err := m.updater.getObj(12) // get_MusicProperties
		if err != nil {
			return err
		}
		defer music.release()
		_ = music.putString(7, t.Title)   // put_Title
		_ = music.putString(11, t.Artist) // put_Artist
		_ = music.putString(9, t.Artist)  // put_AlbumArtist
		if m2, err := music.query(&iidMusic2); err == nil {
			_ = m2.putString(7, t.Album) // put_AlbumTitle
			m2.release()
		}
		_ = m.updater.call(11, 0) // put_Thumbnail(null)
		if t.CoverURL != "" {
			if ref, err := streamRef(t.CoverURL); err == nil {
				_ = m.updater.call(11, uintptr(ref))
				ref.release()
			}
		}
		return m.updater.call(17) // Update
	})
}

// streamRef, bir adresten RandomAccessStreamReference kurar (kapak resmi için).
func streamRef(address string) (com, error) {
	uf, err := factory("Windows.Foundation.Uri", &iidUriFac)
	if err != nil {
		return 0, err
	}
	defer uf.release()
	h, err := newHString(address)
	if err != nil {
		return 0, err
	}
	defer h.free()
	var uri com
	if err := uf.call(6, uintptr(h), uintptrOf(&uri)); err != nil { // CreateUri
		return 0, err
	}
	defer uri.release()
	st, err := factory("Windows.Storage.Streams.RandomAccessStreamReference", &iidStreamRef)
	if err != nil {
		return 0, err
	}
	defer st.release()
	var ref com
	err = st.call(7, uintptr(uri), uintptrOf(&ref)) // CreateFromUri
	return ref, err
}

// SetStatus, kartın çalma durumunu değiştirir.
func (m *Media) SetStatus(s Status) error {
	v := map[Status]uintptr{Closed: 0, Stopped: 2, Playing: 3, Paused: 4}[s]
	return m.w.do(func() error {
		if s == Closed {
			_ = m.updater.call(16) // ClearAll
			_ = m.updater.call(7, 1)
			_ = m.updater.call(17)
		}
		return m.smtc.call(7, v) // put_PlaybackStatus
	})
}

// SetPosition, kartın zaman çizelgesini günceller (sarma çubuğu).
func (m *Media) SetPosition(pos, dur time.Duration) error {
	if m.smtc2 == 0 || dur <= 0 {
		return nil
	}
	return m.w.do(func() error {
		tl, err := activate("Windows.Media.SystemMediaTransportControlsTimelineProperties")
		if err != nil {
			return err
		}
		defer tl.release()
		p, err := tl.query(&iidTimeline)
		if err != nil {
			return err
		}
		defer p.release()
		_ = p.call(7, 0)                    // put_StartTime
		_ = p.call(9, ticks(dur))           // put_EndTime
		_ = p.call(11, 0)                   // put_MinSeekTime
		_ = p.call(13, ticks(dur))          // put_MaxSeekTime
		_ = p.call(15, ticks(pos))          // put_Position
		return m.smtc2.call(12, uintptr(p)) // UpdateTimelineProperties
	})
}

// Close, kartı kapatır.
func (m *Media) Close() {
	if m == nil || m.smtc == 0 {
		return
	}
	_ = m.SetStatus(Closed)
	_ = m.w.do(func() error { return m.smtc.call(11, 0) }) // put_IsEnabled(false)
}
