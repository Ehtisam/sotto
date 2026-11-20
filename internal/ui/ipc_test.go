package ui

import (
	"strings"
	"testing"

	"github.com/Ehtisam/sotto/internal/cli"
	"github.com/Ehtisam/sotto/internal/ipc"
)

func ask(t *testing.T, m Model, cmd string, args ...string) (Model, ipc.Response) {
	t.Helper()
	reply := make(chan ipc.Response, 1)
	out, c := m.Update(IPCMsg{Req: ipc.Request{Cmd: cmd, Args: args}, Reply: reply})
	runAll(c)
	select {
	case r := <-reply:
		return out.(Model), r
	default:
		t.Fatalf("%s: no reply", cmd)
	}
	return m, ipc.Response{}
}

func TestIPCStatusFromModel(t *testing.T) {
	m := eventsModel(t)
	m.state.Volume = 35
	_, r := ask(t, m, "status")
	if !r.OK || !strings.Contains(r.Text, "now") || !strings.Contains(string(r.Data), `"volume":35`) || !strings.Contains(r.Text, "KaBoom.2") {
		t.Fatalf("%+v", r)
	}
	_, r = ask(t, m, "vol")
	if r.Text != "35" {
		t.Fatalf("vol: %+v", r)
	}
}

func TestIPCBadArgsFail(t *testing.T) {
	m := eventsModel(t)
	for _, c := range [][]string{{"seek", "abc"}, {"vol", "loud"}, {"repeat", "sometimes"}, {"play", "not a link"}} {
		_, r := ask(t, m, c[0], c[1:]...)
		if r.OK || r.Error == "" {
			t.Errorf("%v accepted: %+v", c, r)
		}
	}
	m.state.Item = nil
	if _, r := ask(t, m, "seek", "10"); r.OK {
		t.Error("seek with nothing playing")
	}
	if s := m.cliStatus(); s != (cli.Status{}) {
		t.Errorf("empty status = %+v", s)
	}
}
