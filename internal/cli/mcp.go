package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/Ehtisam/sotto/internal/config"
	"github.com/Ehtisam/sotto/internal/ipc"
)

// MCP sunucusu (sotto mcp): Claude gibi yapay zekâ asistanları sotto'yu
// Model Context Protocol ile kullanır ("şunu çal", "bu şarkıyı listeye
// ekle"). Standart giriş/çıkışta satır satır JSON-RPC 2.0. Araçlar komut
// satırının aynısını çalıştırır (Run): açık bir sotto varsa çalma komutları
// ona gider, yoksa doğrudan Spotify'a.
//
// Asistana eklemek için (örneğin Claude Code):
//
//	claude mcp add sotto -- sotto mcp

// mcpVersions: bilinen protokol sürümleri, yeniden eskiye.
var mcpVersions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`

	// args: araç argümanlarından komut satırı sözcükleri.
	args func(a map[string]any) []string `json:"-"`
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

func enum(desc string, vals ...string) map[string]any {
	return map[string]any{"type": "string", "description": desc, "enum": vals}
}

func schema(required []string, props map[string]any) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

// argStr: argümanı dize olarak (sayı da gelebilir).
func argStr(a map[string]any, k string) string {
	switch v := a[k].(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	}
	return ""
}

func argWords(a map[string]any, k string) []string { return strings.Fields(argStr(a, k)) }

// withOpt: değer varsa "--name değer" ekler.
func withOpt(args []string, a map[string]any, k, flag string) []string {
	if v := argStr(a, k); v != "" {
		return append(args, flag, v)
	}
	return args
}

func fixed(cmd ...string) func(map[string]any) []string {
	return func(map[string]any) []string { return cmd }
}

var mcpTools = []mcpTool{
	{Name: "play", Description: "Resume playback, or play something: a Spotify link/URI, or words to search for (the best match plays). Leave query empty to resume.",
		InputSchema: schema(nil, map[string]any{
			"query": str("Spotify link or URI, or search words such as 'daft punk one more time'"),
			"type":  enum("What to search for (default: track)", "track", "album", "artist", "playlist", "show", "episode"),
		}),
		args: func(a map[string]any) []string {
			return withOpt(append([]string{"play"}, argWords(a, "query")...), a, "type", "--type")
		}},
	{Name: "pause", Description: "Pause playback.", InputSchema: schema(nil, map[string]any{}), args: fixed("pause")},
	{Name: "toggle", Description: "Play if paused, pause if playing.", InputSchema: schema(nil, map[string]any{}), args: fixed("toggle")},
	{Name: "next", Description: "Skip to the next song.", InputSchema: schema(nil, map[string]any{}), args: fixed("next")},
	{Name: "previous", Description: "Go back to the previous song.", InputSchema: schema(nil, map[string]any{}), args: fixed("prev")},
	{Name: "seek", Description: "Jump within the song: seconds (90), minutes:seconds (1:30), or relative (+10, -10).",
		InputSchema: schema([]string{"position"}, map[string]any{"position": str("90, 1:30, +10 or -10")}),
		args:        func(a map[string]any) []string { return []string{"seek", argStr(a, "position")} }},
	{Name: "volume", Description: "Show the volume, or set it: a number 0-100, or relative like +10 / -10.",
		InputSchema: schema(nil, map[string]any{"level": str("0-100, +n or -n; empty shows the volume")}),
		args:        func(a map[string]any) []string { return append([]string{"vol"}, argWords(a, "level")...) }},
	{Name: "shuffle", Description: "Turn shuffle on or off (toggles when state is empty).",
		InputSchema: schema(nil, map[string]any{"state": enum("on or off", "on", "off")}),
		args:        func(a map[string]any) []string { return append([]string{"shuffle"}, argWords(a, "state")...) }},
	{Name: "repeat", Description: "Set repeat: off, context (the album or playlist) or track.",
		InputSchema: schema([]string{"mode"}, map[string]any{"mode": enum("Repeat mode", "off", "context", "track")}),
		args:        func(a map[string]any) []string { return []string{"repeat", argStr(a, "mode")} }},
	{Name: "status", Description: "What is playing now: song, artist, album, position, device, volume, shuffle and repeat.",
		InputSchema: schema(nil, map[string]any{}), args: fixed("status")},
	{Name: "like", Description: "Save the current song to Liked Songs.", InputSchema: schema(nil, map[string]any{}), args: fixed("like")},
	{Name: "unlike", Description: "Remove the current song from Liked Songs.", InputSchema: schema(nil, map[string]any{}), args: fixed("unlike")},
	{Name: "devices", Description: "List the Spotify Connect devices (speakers) this account can play on.",
		InputSchema: schema(nil, map[string]any{}), args: fixed("devices")},
	{Name: "connect", Description: "Move playback to another device, by (part of) its name.",
		InputSchema: schema([]string{"device"}, map[string]any{"device": str("Device name, as listed by devices")}),
		args:        func(a map[string]any) []string { return append([]string{"connect"}, argWords(a, "device")...) }},
	{Name: "search", Description: "Search Spotify. Returns names with their Spotify URIs, which play and queue_add accept.",
		InputSchema: schema([]string{"query"}, map[string]any{
			"query": str("Search words"),
			"type":  str("Comma-separated: track, album, artist, playlist, show, episode (default: all)"),
			"limit": map[string]any{"type": "integer", "description": "Results per type (default 5)"},
		}),
		args: func(a map[string]any) []string {
			return withOpt(withOpt(append([]string{"search"}, argWords(a, "query")...), a, "type", "--type"), a, "limit", "--limit")
		}},
	{Name: "queue", Description: "Show the upcoming songs.", InputSchema: schema(nil, map[string]any{}), args: fixed("queue")},
	{Name: "queue_add", Description: "Add a song to the queue: a Spotify link/URI, or words to search for.",
		InputSchema: schema([]string{"query"}, map[string]any{"query": str("Spotify link or URI, or search words")}),
		args:        func(a map[string]any) []string { return append([]string{"queue"}, argWords(a, "query")...) }},
	{Name: "playlists", Description: "List your playlists.", InputSchema: schema(nil, map[string]any{}), args: fixed("playlist", "list")},
	{Name: "playlist_create", Description: "Create a new, empty playlist.",
		InputSchema: schema([]string{"name"}, map[string]any{
			"name":        str("Playlist name"),
			"description": str("Optional description"),
			"public":      map[string]any{"type": "boolean", "description": "Show it on your profile (default: private)"},
		}),
		args: func(a map[string]any) []string {
			args := withOpt([]string{"playlist", "new", argStr(a, "name")}, a, "description", "--desc")
			if argStr(a, "public") == "true" {
				args = append(args, "--public")
			}
			return args
		}},
	{Name: "history", Description: "What was listened to while sotto was open, with the most played songs and artists.",
		InputSchema: schema(nil, map[string]any{"range": enum("Time range (default: week)", "week", "month", "year", "all")})},
}

type rpcMsg struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCP, sunucuyu stdin kapanana dek çalıştırır.
func MCP(cfg config.Config, stdin io.Reader, stdout, stderr io.Writer) int {
	StateDir = cfg.LocalDir
	in := bufio.NewScanner(stdin)
	in.Buffer(make([]byte, 0, 64*1024), 8<<20)
	enc := json.NewEncoder(stdout) // her yanıt tek satır
	for in.Scan() {
		line := bytes.TrimSpace(in.Bytes())
		if len(line) == 0 {
			continue
		}
		var msg rpcMsg
		if err := json.Unmarshal(line, &msg); err != nil {
			_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": nil, "error": rpcError{-32700, "parse error"}})
			continue
		}
		result, rerr := mcpHandle(cfg, msg)
		if len(msg.ID) == 0 || string(msg.ID) == "null" {
			continue // bildirim: yanıt yok
		}
		out := map[string]any{"jsonrpc": "2.0", "id": msg.ID}
		if rerr != nil {
			out["error"] = rerr
		} else {
			out["result"] = result
		}
		if err := enc.Encode(out); err != nil {
			fmt.Fprintln(stderr, "sotto mcp:", err)
			return 1
		}
	}
	return 0
}

func mcpHandle(cfg config.Config, msg rpcMsg) (any, *rpcError) {
	switch msg.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(msg.Params, &p)
		version := mcpVersions[0]
		for _, v := range mcpVersions {
			if v == p.ProtocolVersion {
				version = v
			}
		}
		return map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "sotto", "version": Version},
			"instructions": "sotto controls the user's Spotify. Use search to find Spotify URIs, then play or queue_add them. " +
				"Playback commands go through the open sotto window when there is one, otherwise to the active Spotify device.",
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": mcpTools}, nil
	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return nil, &rpcError{-32602, "invalid params"}
		}
		return mcpCall(cfg, p.Name, p.Arguments)
	}
	if strings.HasPrefix(msg.Method, "notifications/") {
		return nil, nil
	}
	return nil, &rpcError{-32601, "method not found: " + msg.Method}
}

// mcpRun: testler değiştirir (gerçek Spotify'a gitmesin).
var mcpRun = func(cfg config.Config, req ipc.Request) ipc.Response {
	return Run(cfg, req, strings.NewReader(""), io.Discard)
}

func mcpCall(cfg config.Config, name string, a map[string]any) (any, *rpcError) {
	if a == nil {
		a = map[string]any{}
	}
	var tool *mcpTool
	for i := range mcpTools {
		if mcpTools[i].Name == name {
			tool = &mcpTools[i]
		}
	}
	if tool == nil {
		return nil, &rpcError{-32602, "unknown tool: " + name}
	}
	if name == "history" {
		var out, errb bytes.Buffer
		args := argWords(a, "range")
		if History(cfg, args, &out, &errb, false) != 0 {
			return mcpResult(strings.TrimSpace(errb.String()), true), nil
		}
		return mcpResult(strings.TrimSpace(out.String()), false), nil
	}
	args := tool.args(a)
	r := mcpRun(cfg, ipc.Request{Cmd: args[0], Args: args[1:]})
	if !r.OK {
		return mcpResult(r.Error, true), nil
	}
	text := r.Text
	if len(r.Data) > 0 { // asistan ayrıntıyı (adresler, süreler) da görsün
		if text != "" {
			text += "\n\n"
		}
		text += string(r.Data)
	}
	if text == "" {
		text = "Done."
	}
	return mcpResult(text, false), nil
}

func mcpResult(text string, isError bool) map[string]any {
	return map[string]any{"content": []map[string]any{{"type": "text", "text": text}}, "isError": isError}
}
