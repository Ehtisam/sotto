package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/Ehtisam/sotto/internal/bridge"
	"github.com/Ehtisam/sotto/internal/config"
	"github.com/Ehtisam/sotto/internal/desktop"
	"github.com/Ehtisam/sotto/internal/ipc"
	"github.com/Ehtisam/sotto/internal/soloist"
	"github.com/Ehtisam/sotto/internal/spotify"
)

// Check, "sotto doctor"un bir satırı.
type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"` // ok, warn, fail, info
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

var hex32 = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)

// Doctor, kurulumu baştan sona denetler.
func Doctor(cfg config.Config, out io.Writer, asJSON bool) int {
	checks := RunChecks(cfg)
	failed := false
	for _, c := range checks {
		failed = failed || c.Status == "fail"
	}
	if asJSON {
		b, _ := json.MarshalIndent(checks, "", "  ")
		fmt.Fprintln(out, string(b))
	} else {
		icons := map[string]string{"ok": "✓", "warn": "!", "fail": "✗", "info": "·"}
		for _, c := range checks {
			fmt.Fprintf(out, "%s %-16s %s\n", icons[c.Status], c.Name, c.Detail)
			if c.Fix != "" {
				fmt.Fprintf(out, "  %-16s → %s\n", "", c.Fix)
			}
		}
	}
	if failed {
		return 1
	}
	return 0
}

// RunChecks, denetimleri yapar.
func RunChecks(cfg config.Config) []Check {
	var cs []Check
	add := func(name, status, detail, fix string) { cs = append(cs, Check{name, status, detail, fix}) }
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	add("settings", "ok", cfg.ConfigFile(), "")

	// Spotify uygulaması ve giriş.
	sp := spotify.NewClient(cfg.Spotify.ClientID, cfg.TokenFile())
	loggedIn := false
	switch {
	case cfg.Spotify.ClientID == "":
		add("spotify app", "fail", "no Client ID", "run sotto setup")
	case !hex32.MatchString(cfg.Spotify.ClientID):
		add("spotify app", "fail", "the Client ID should be 32 letters and digits", "copy it again from developer.spotify.com/dashboard")
	default:
		add("spotify app", "ok", "Client ID "+cfg.Spotify.ClientID[:4]+"…"+cfg.Spotify.ClientID[28:], "")
		if !sp.HasLogin() {
			add("spotify login", "fail", "not logged in", "open sotto; it opens the login page")
			break
		}
		me, err := sp.Me(ctx)
		if spotify.IsRateLimited(err) {
			loggedIn = true // giriş var; Spotify yalnızca bir süre bekletiyor
			add("spotify limit", "warn", err.Error(), "wait; sotto shows what it already has meanwhile")
			break
		}
		if err != nil {
			add("spotify login", "fail", err.Error(), "open sotto and log in again (ctrl+l)")
			break
		}
		loggedIn = true
		name := me.DisplayName
		if name == "" {
			name = me.ID
		}
		if me.Product != "" && me.Product != "premium" {
			add("spotify login", "fail", name+" ("+me.Product+")", "Soloist and remote control need Spotify Premium")
		} else {
			add("spotify login", "ok", name+" (premium)", "")
		}
	}

	// Soloist.
	path := cfg.SoloistPath()
	if _, err := os.Stat(path); err != nil {
		st := "warn"
		if cfg.Soloist.Autostart {
			st = "fail"
		}
		add("soloist", st, "not installed", "sotto setup downloads it (light mode plays music on this computer)")
	} else if b, err := soloist.ReadBuild(ctx, path); err != nil {
		add("soloist", "fail", "cannot run "+path+": "+err.Error(), "Settings → Soloist build downloads it again")
	} else {
		left := b.DaysLeft(time.Now())
		detail := fmt.Sprintf("%s, built %s, %d days left", b.Version, b.Built.Format("2 Jan 2006"), left)
		if cfg.Soloist.AutoUpdate {
			detail += " (updates by itself)"
		}
		switch {
		case left <= 0:
			add("soloist", "fail", fmt.Sprintf("%s expired on %s", b.Version, b.Expires().Format("2 Jan 2006")), "Settings → Soloist build downloads the latest")
		case left <= 14 && !cfg.Soloist.AutoUpdate:
			add("soloist", "warn", detail, "Settings → Soloist build downloads the latest")
		default:
			add("soloist", "ok", detail, "")
		}
	}
	switch {
	case cfg.SoloistAPIKey() == "":
		st := "info"
		if cfg.Soloist.Autostart {
			st = "fail"
		}
		add("soloist key", st, "none (remote control only)", "sotto setup asks for it")
	default:
		add("soloist key", "ok", "set", "")
	}
	if cfg.Soloist.Autostart {
		add("speaker", "info", "Soloist starts with sotto (light mode)", "")
	} else {
		add("speaker", "info", "remote control: sotto plays on your other devices", "")
	}

	// Açık sotto.
	if _, err := ipc.Call(ipc.Path(cfg.LocalDir), ipc.Request{Cmd: "status"}, 5*time.Second); err == nil {
		add("sotto", "ok", "open; commands go to it", "")
	} else {
		add("sotto", "info", "not open; commands go straight to Spotify", "")
	}

	// Cihazlar.
	if loggedIn {
		if ds, err := sp.Devices(ctx); err != nil {
			add("devices", "warn", err.Error(), "")
		} else {
			var names []string
			for _, d := range ds {
				n := d.Name
				if d.IsActive {
					n += " (playing)"
				}
				names = append(names, n)
			}
			if len(names) == 0 {
				add("devices", "info", "none online right now", "")
			} else {
				add("devices", "ok", strings.Join(names, ", "), "")
			}
		}
	}

	// Ekstralar ✱.
	br := bridge.New(filepath.Join(cfg.LocalDir, "bridge"))
	switch {
	case !cfg.Extras.Enabled:
		add("extras", "info", "off (official routes only)", "")
	case !br.Installed():
		add("extras", "warn", "on, but go-librespot is not installed", "sotto setup downloads it")
	default:
		add("extras", "ok", "on (go-librespot installed)", "")
	}

	// Masaüstü.
	switch {
	case cfg.Desktop.Notify == "off":
		add("notifications", "info", "off", "")
	case desktop.Registered():
		add("notifications", "ok", cfg.Desktop.Notify, "")
	default:
		add("notifications", "info", cfg.Desktop.Notify+"; Windows learns sotto's name with the first one", "")
	}
	if runtime.GOOS == "windows" {
		exe, _ := os.Executable()
		target, id, _ := desktop.ShortcutInfo()
		switch {
		case !cfg.Desktop.StartMenu:
			add("start menu", "info", "off (the media card says \"Unknown app\")", "")
		case target == "":
			add("start menu", "warn", "no shortcut yet", "open sotto once; it adds one")
		case id != desktop.AppID:
			add("start menu", "warn", "the shortcut lacks sotto's app id", "open sotto once; it fixes it")
		case !strings.EqualFold(filepath.Clean(target), filepath.Clean(exe)):
			add("start menu", "info", "points to "+target, "")
		default:
			add("start menu", "ok", "sotto shows by name on the media card", "")
		}
	}
	if cfg.Desktop.MediaControls {
		add("media keys", "ok", "on", "")
	} else {
		add("media keys", "info", "off", "")
	}
	if c := strings.TrimSpace(cfg.Hooks.OnTrackChange + cfg.Hooks.OnStatusChange); c != "" {
		add("hooks", "info", "set; output goes to "+filepath.Join(cfg.LocalDir, "logs", "hooks.log"), "")
	}
	return cs
}
