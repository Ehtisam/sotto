// Package cli, sotto'nun komut satırı: "sotto next", "sotto status --json",
// "sotto search …", "sotto playlist …", "sotto doctor".
//
// Çalma komutları açık bir sotto varsa ona gider (Soloist'e yerel bağlantıyla
// ulaşır, arayüz hemen güncellenir); yoksa doğrudan Web API ile etkin
// cihaza. Arama ve liste işleri hep doğrudan Web API ile yapılır.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/Ehtisam/sotto/internal/config"
	"github.com/Ehtisam/sotto/internal/ipc"
	"github.com/Ehtisam/sotto/internal/spotify"
)

// Remote: açık sotto'nun kendisi yürüttüğü komutlar.
var Remote = map[string]bool{
	"play": true, "pause": true, "toggle": true, "next": true, "prev": true,
	"seek": true, "vol": true, "mute": true, "shuffle": true, "repeat": true,
	"status": true, "like": true, "unlike": true, "connect": true,
}

// commands: bilinen bütün komutlar (sotto <komut>).
var commands = map[string]bool{
	"search": true, "devices": true, "queue": true, "playlist": true,
	"doctor": true, "help": true, "version": true, "theme": true, "history": true,
}

// IsCommand, ilk argüman bir komut mu (değilse arayüz açılır).
func IsCommand(arg string) bool {
	switch arg {
	case "-h", "--help", "-v", "--version", "previous", "volume", "playlists":
		return true
	}
	return Remote[arg] || commands[arg]
}

// Version, derlemede -ldflags ile verilir.
var Version = "0.1.0"

// Main, komutu çalıştırır ve çıkış kodunu döner.
func Main(cfg config.Config, args []string, stdout, stderr io.Writer, stdin io.Reader) int {
	asJSON := false
	var rest []string
	for _, a := range args {
		if a == "--json" {
			asJSON = true
			continue
		}
		rest = append(rest, a)
	}
	if len(rest) == 0 {
		rest = []string{"help"}
	}
	cmd, cargs := rest[0], rest[1:]
	StateDir = cfg.LocalDir
	switch cmd {
	case "-h", "--help", "help":
		fmt.Fprint(stdout, help)
		return 0
	case "-v", "--version", "version":
		fmt.Fprintln(stdout, "sotto", Version)
		return 0
	case "previous":
		cmd = "prev"
	case "volume":
		cmd = "vol"
	case "playlists":
		cmd, cargs = "playlist", append([]string{"list"}, cargs...)
	case "doctor":
		return Doctor(cfg, stdout, asJSON)
	case "theme":
		return Theme(cfg, cargs, stdout, stderr)
	case "history":
		return History(cfg, cargs, stdout, stderr, asJSON)
	}
	if !IsCommand(cmd) {
		fmt.Fprintf(stderr, "sotto: unknown command %q (sotto help lists them)\n", cmd)
		return 2
	}
	req := ipc.Request{Cmd: cmd, Args: cargs}

	var resp ipc.Response
	handled := false
	if Remote[cmd] {
		r, err := ipc.Call(ipc.Path(cfg.LocalDir), req, 2*time.Minute)
		if err == nil {
			resp, handled = r, true
		} else if !errors.Is(err, ipc.ErrNotRunning) {
			fmt.Fprintln(stderr, "sotto:", err)
			return 1
		}
	}
	if !handled {
		sp := spotify.NewClient(cfg.Spotify.ClientID, cfg.TokenFile())
		switch {
		case !sp.HasClientID():
			resp = ipc.Fail(errors.New("sotto is not set up yet; run sotto setup"))
		case !sp.HasLogin():
			resp = ipc.Fail(errors.New("not logged in to Spotify; open sotto and log in"))
		default:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			resp = Direct(ctx, sp, req, stdin, stderr)
			cancel()
		}
	}
	return print(resp, asJSON, stdout, stderr)
}

func print(r ipc.Response, asJSON bool, stdout, stderr io.Writer) int {
	if asJSON {
		out := map[string]any{"ok": r.OK}
		if r.Error != "" {
			out["error"] = r.Error
		}
		if len(r.Data) > 0 {
			out["data"] = r.Data
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Fprintln(stdout, string(b))
	} else if r.OK {
		if r.Text != "" {
			fmt.Fprintln(stdout, r.Text)
		}
	} else {
		fmt.Fprintln(stderr, "sotto:", r.Error)
	}
	if !r.OK {
		return 1
	}
	return 0
}

// flags, argümanları konumsal olanlar ve --ad değer bayrakları olarak ayırır.
type flags struct {
	pos  []string
	vals map[string]string
}

// valued: değer alan bayraklar.
var valued = map[string]bool{"type": true, "limit": true, "desc": true, "name": true, "into": true}

func parse(args []string) flags {
	f := flags{vals: map[string]string{}}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "--") && len(a) > 2 {
			name, val, hasVal := strings.Cut(a[2:], "=")
			if !hasVal && valued[name] && i+1 < len(args) {
				val, i = args[i+1], i+1
			} else if !hasVal {
				val = "true"
			}
			f.vals[name] = val
			continue
		}
		f.pos = append(f.pos, a)
	}
	return f
}

func (f flags) has(name string) bool { return f.vals[name] == "true" }

func (f flags) int(name string, def int) int {
	if v, err := strconv.Atoi(f.vals[name]); err == nil {
		return v
	}
	return def
}

func (f flags) arg(i int) string {
	if i < len(f.pos) {
		return f.pos[i]
	}
	return ""
}

// rest: i'inci konumsal argümandan sonrası tek metin olarak.
func (f flags) rest(i int) string {
	if i >= len(f.pos) {
		return ""
	}
	return strings.Join(f.pos[i:], " ")
}

const help = `sotto: a Spotify player for the terminal.

  sotto                     open sotto
  sotto setup               run the setup again

Playback (goes through the open sotto if there is one, else the active device):
  sotto play [link|words]   resume, play a link, or search and play the best match:
                            sotto play daft punk one more time
                            sotto play random access memories --type album
                            (--type track, album, artist, playlist, show, episode)
  sotto pause | toggle
  sotto next | prev
  sotto seek <pos>          90, 1:30, +10 or -10
  sotto vol [n|+n|-n]       show or set the volume
  sotto mute [on|off]       mute or unmute (the old volume comes back)
  sotto shuffle [on|off]    toggle or set shuffle
  sotto repeat [off|context|track]
  sotto status              what is playing
  sotto like | unlike       save the current song to Liked Songs, or remove it
  sotto connect [device]    list devices, or move playback to one

Library:
  sotto search <words> [--type track,album,artist,playlist,show,episode] [--limit n]
  sotto queue [link|words]  show the queue, or add a song to it (searched like play)
  sotto devices
  sotto playlist list
  sotto playlist new <name> [--public] [--collab] [--desc text]
  sotto playlist delete <name|link> [--yes]
  sotto playlist duplicate <name|link> [new name]
  sotto playlist import <file|-> [--name name | --into playlist]
        one Spotify link or uri per line; other lines are searched as "artist - title"

  sotto theme list          the themes you can pick
  sotto theme import <file> [--name name] [--use]
        make a theme from a terminal colour scheme: iTerm2 .itermcolors,
        Alacritty .toml or .yml, or a Windows Terminal scheme .json

  sotto history [week|month|year|all]
                            what you listened to while sotto was open, with
                            the most played songs and artists (--json too)
  sotto doctor              check the setup
  --json                    machine-readable output for any command
`
