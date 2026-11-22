package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Ehtisam/sotto/internal/config"
	"github.com/Ehtisam/sotto/internal/ipc"
)

// MCP: el sıkışma, araç listesi, araç çağrısı komut satırı isteğine
// dönüşür, bildirime yanıt verilmez, bilinmeyen yöntem hata döner.
func TestMCP(t *testing.T) {
	var got []ipc.Request
	old := mcpRun
	mcpRun = func(_ config.Config, req ipc.Request) ipc.Response {
		got = append(got, req)
		if req.Cmd == "pause" {
			return ipc.Fail(errorString("nothing is playing"))
		}
		return ipc.Done("Playing One More Time — Daft Punk", map[string]string{"uri": "spotify:track:x"})
	}
	defer func() { mcpRun = old }()

	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"play","arguments":{"query":"daft punk one more time","type":"track"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"pause","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"nope"}`,
		`not json`,
	}, "\n")
	var out bytes.Buffer
	if code := MCP(config.Config{LocalDir: t.TempDir()}, strings.NewReader(in), &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 6 { // 5 istek + ayrıştırma hatası; bildirime yanıt yok
		t.Fatalf("%d replies:\n%s", len(lines), out.String())
	}
	var r []map[string]any
	for _, l := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("bad reply %q", l)
		}
		r = append(r, m)
	}
	init := r[0]["result"].(map[string]any)
	if init["protocolVersion"] != "2025-03-26" || init["serverInfo"].(map[string]any)["name"] != "sotto" {
		t.Fatalf("initialize %v", init)
	}
	tools := r[1]["result"].(map[string]any)["tools"].([]any)
	if len(tools) != len(mcpTools) || len(tools) < 15 {
		t.Fatalf("%d tools", len(tools))
	}
	for _, tl := range tools {
		m := tl.(map[string]any)
		if m["name"] == "" || m["description"] == "" || m["inputSchema"].(map[string]any)["type"] != "object" {
			t.Fatalf("tool %v", m)
		}
	}
	play := r[2]["result"].(map[string]any)
	if play["isError"] != false || !strings.Contains(play["content"].([]any)[0].(map[string]any)["text"].(string), "spotify:track:x") {
		t.Fatalf("play %v", play)
	}
	if len(got) < 1 || got[0].Cmd != "play" || strings.Join(got[0].Args, " ") != "daft punk one more time --type track" {
		t.Fatalf("request %+v", got)
	}
	if pause := r[3]["result"].(map[string]any); pause["isError"] != true {
		t.Fatalf("pause should be an error result: %v", pause)
	}
	if e := r[4]["error"].(map[string]any); e["code"].(float64) != -32601 {
		t.Fatalf("unknown method %v", e)
	}
	if e := r[5]["error"].(map[string]any); e["code"].(float64) != -32700 {
		t.Fatalf("parse error %v", e)
	}
}

type errorString string

func (e errorString) Error() string { return string(e) }
