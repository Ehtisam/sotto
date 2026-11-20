package ui

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/cli"
	"github.com/Ehtisam/sotto/internal/ipc"
	"github.com/Ehtisam/sotto/internal/soloist"
	"github.com/Ehtisam/sotto/internal/spotify"
)

// Komut satırından gelen çalma komutları: açık sotto onları tuşlarla aynı
// yoldan yürütür (Soloist etkinse ona, değilse Web API ile etkin cihaza).

// IPCMsg, komut satırından gelen bir istek; yanıt Reply'a bir kez yazılır.
type IPCMsg struct {
	Req   ipc.Request
	Reply chan<- ipc.Response
}

// Spotify, modelin Web API istemcisi (komut satırı sunucusu için).
func (m Model) Spotify() *spotify.Client { return m.sp }

// status, şimdiki durumun komut satırı hali.
func (m Model) cliStatus() cli.Status {
	if m.state.Item == nil {
		return cli.Status{}
	}
	name, artist, album := m.describe(m.state.Item)
	device := m.remote.device
	if m.soloistActive() {
		device = m.auth.DeviceName
	}
	return cli.Status{
		Playing: m.playing(), Title: name, Artist: artist, Album: album,
		URI: m.state.Item.URI, Link: spotify.WebLink(m.state.Item.URI),
		Context: m.state.Context.Name(), ContextURI: m.state.Context.URIOrEmpty(), Device: device,
		PositionMS: m.position().Milliseconds(), DurationMS: m.state.Item.Duration().Milliseconds(),
		Volume: m.state.Volume, Shuffle: m.state.Options.Shuffle, Repeat: m.state.Options.Repeat,
	}
}

// replyAfter, komut bitince sonucunu yanıtlar: flashMsg hata demektir.
func replyAfter(cmd tea.Cmd, reply chan<- ipc.Response, text string) tea.Cmd {
	return func() tea.Msg {
		var msg tea.Msg
		if cmd != nil {
			msg = cmd()
		}
		if f, ok := msg.(flashMsg); ok {
			reply <- ipc.Fail(errors.New(string(f)))
		} else {
			reply <- ipc.Done(text, nil)
		}
		return msg
	}
}

func (m Model) updateIPC(msg IPCMsg) (tea.Model, tea.Cmd) {
	reply, args := msg.Reply, msg.Req.Args
	arg := ""
	if len(args) > 0 {
		arg = args[0]
	}
	fail := func(err error) (tea.Model, tea.Cmd) {
		reply <- ipc.Fail(err)
		return m, nil
	}
	cur := m.cliStatus()
	needItem := func() error {
		if cur.URI == "" {
			return errors.New("nothing is playing")
		}
		return nil
	}

	switch msg.Req.Cmd {
	case "status":
		reply <- ipc.Done(cur.Text(), cur)
		return m, nil
	case "play":
		if arg == "" {
			return m, replyAfter(m.resumeOrPlay(), reply, "")
		}
		if len(args) == 1 {
			if uri, err := cli.ToURI(arg); err == nil {
				return m, m.playFound(uri, "", reply)
			}
		}
		// Sözcükler: önce ara, bulunanı aynı yoldan çal.
		if m.spState != spReady {
			return fail(errors.New(T("flash.noaccount")))
		}
		sp := m.sp
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			uri, label, err := cli.Target(ctx, sp, args)
			if err != nil {
				reply <- ipc.Fail(err)
				return nil
			}
			return ipcFoundMsg{uri: uri, label: label, reply: reply}
		}
	case "pause":
		return m, replyAfter(m.pause(), reply, "")
	case "toggle":
		return m, replyAfter(m.playPause(), reply, "")
	case "next":
		return m, replyAfter(m.next(), reply, "")
	case "prev":
		return m, replyAfter(m.previous(), reply, "")
	case "seek":
		if err := needItem(); err != nil {
			return fail(err)
		}
		ms, err := cli.ParseSeek(arg, cur.PositionMS)
		if err != nil {
			return fail(err)
		}
		return m, replyAfter(m.seekTo(time.Duration(ms)*time.Millisecond), reply, "")
	case "vol":
		if arg == "" {
			reply <- ipc.Done(strconv.Itoa(m.state.Volume), map[string]int{"volume": m.state.Volume})
			return m, nil
		}
		v, err := cli.ParseVolume(arg, m.state.Volume)
		if err != nil {
			return fail(err)
		}
		return m, replyAfter(m.setVolume(v), reply, "")
	case "shuffle":
		on, err := cli.ParseOnOff(arg, m.state.Options.Shuffle)
		if err != nil {
			return fail(err)
		}
		text := "Shuffle off."
		if on {
			text = "Shuffle on."
		}
		return m, replyAfter(m.setShuffle(on), reply, text)
	case "repeat":
		mode, err := cli.NextRepeat(arg, m.state.Options.Repeat)
		if err != nil {
			return fail(err)
		}
		return m, replyAfter(m.setRepeat(mode), reply, "Repeat "+mode+".")
	case "like", "unlike":
		if m.spState != spReady {
			return fail(errors.New(T("flash.noaccount")))
		}
		sp, like := m.sp, msg.Req.Cmd == "like"
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			reply <- cli.Like(ctx, sp, cur, like)
			return nil
		}
	case "connect":
		name := strings.Join(args, " ")
		if name == "" {
			return m, m.cliDirect(msg)
		}
		// Soloist kapalıysa ve ona geçilmek isteniyorsa önce aç.
		if strings.EqualFold(name, m.cfg.Soloist.DeviceName) && !m.soloistLive() && m.cfg.SoloistAPIKey() != "" {
			mm, cmd := m.startSoloistAndUse()
			reply <- ipc.Done("Starting Soloist; playback moves to it when it is ready.", nil)
			return mm, cmd
		}
		return m, m.cliDirect(msg)
	}
	return m, m.cliDirect(msg)
}

// ipcFoundMsg: "sotto play <sözcükler>" araması bitti; bulunan çalınır.
type ipcFoundMsg struct {
	uri, label string
	reply      chan<- ipc.Response
}

// playFound, bir adresi çalar: Soloist etkinse ona, değilse etkin cihaza.
func (m Model) playFound(uri, label string, reply chan<- ipc.Response) tea.Cmd {
	return replyAfter(m.control(
		func(ctx context.Context, c *soloist.Client) error { return c.Play(ctx, uri) },
		func(ctx context.Context, s *spotify.Client) error { return cli.PlayURI(ctx, s, "", uri) }), reply, cli.Playing(label))
}

// cliDirect, isteği Web API ile (komut satırındaki gibi) yürütür.
func (m Model) cliDirect(msg IPCMsg) tea.Cmd {
	sp := m.sp
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		msg.Reply <- cli.Direct(ctx, sp, msg.Req, strings.NewReader(""), nopWriter{})
		return pollSoonMsg{}
	}
}

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }
