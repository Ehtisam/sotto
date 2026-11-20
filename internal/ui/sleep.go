package ui

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/soloist"
	"github.com/Ehtisam/sotto/internal/spotify"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// Uyku zamanlayıcısı: z her basışta bir sonraki süreye geçer (15, 30, 45,
// 60, 90 dakika, şarkı bitince, kapalı). Süre dolunca müzik durur. Kalan
// süre alt şeritte görünür.

var sleepSteps = []time.Duration{15 * time.Minute, 30 * time.Minute, 45 * time.Minute, 60 * time.Minute, 90 * time.Minute, -1}

// Şarkı bitince modunda, şarkının bitmesine bu kadar kala durdurulur
// (sonrakine geçmeden).
const sleepEndMargin = 1500 * time.Millisecond

type sleepState struct {
	step  int       // sleepSteps'te sıradaki; 0 kapalı, i>0: sleepSteps[i-1] seçili
	at    time.Time // süreli modda duracağı an
	track string    // şarkı bitince modunda: o an çalan şarkı
}

func (s sleepState) on() bool { return s.step > 0 }

// cycleSleep, z tuşu: bir sonraki süreye geçer.
func (m Model) cycleSleep() Model {
	sl := &m.sleep
	sl.step = (sl.step + 1) % (len(sleepSteps) + 1)
	sl.at, sl.track = time.Time{}, ""
	if !sl.on() {
		return m.note(T("sleep.off"))
	}
	d := sleepSteps[sl.step-1]
	if d < 0 {
		if m.state.Item == nil || m.state.Item.URI == "" {
			sl.step = 0
			return m.note(T("sleep.nothing"))
		}
		sl.track = m.state.Item.URI
		return m.note(T("sleep.track"))
	}
	sl.at = m.now.Add(d)
	return m.note(F("sleep.in", int(d.Minutes())))
}

// sleepTick, her tikte zamanın dolup dolmadığına bakar; dolduysa durdurur.
func (m Model) sleepTick() (Model, tea.Cmd) {
	sl := m.sleep
	if !sl.on() {
		return m, nil
	}
	due := false
	if sl.track != "" {
		cur := ""
		if m.state.Item != nil {
			cur = m.state.Item.URI
		}
		dur := m.state.Item.Duration()
		// Şarkının sonuna gelindi ya da (atlanmış/kaçırılmışsa) şarkı değişti.
		due = cur != sl.track || (dur > 0 && dur-m.position() <= sleepEndMargin)
	} else {
		due = !m.now.Before(sl.at)
	}
	if !due {
		return m, nil
	}
	m.sleep = sleepState{}
	if m.state.Status != soloist.StatusPlaying && m.state.Status != soloist.StatusBuffering {
		return m, nil
	}
	m = m.note(T("sleep.done"))
	return m, m.control(
		func(ctx context.Context, c *soloist.Client) error { return c.Pause(ctx) },
		func(ctx context.Context, s *spotify.Client) error { return s.Pause(ctx) })
}

// sleepMark, alt şeritteki ☾ ve kalan süre.
func (m Model) sleepMark() string {
	sl := m.sleep
	if !sl.on() {
		return ""
	}
	if sl.track != "" {
		return theme.Hi.Render(theme.Icon.Moon + " " + T("sleep.mark.track"))
	}
	left := max(sl.at.Sub(m.now), 0).Round(time.Second)
	return theme.Hi.Render(theme.Icon.Moon + fmt.Sprintf(" %d:%02d", int(left.Minutes()), int(left.Seconds())%60))
}
