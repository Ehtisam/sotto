package cli

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Status, "sotto status" çıktısı (hem açık sotto'dan hem Web API'den).
type Status struct {
	Playing    bool   `json:"playing"`
	Title      string `json:"title"`
	Artist     string `json:"artist"`
	Album      string `json:"album"`
	URI        string `json:"uri"`
	Link       string `json:"link,omitempty"`
	Context    string `json:"context,omitempty"`
	ContextURI string `json:"context_uri,omitempty"`
	Device     string `json:"device,omitempty"`
	PositionMS int64  `json:"position_ms"`
	DurationMS int64  `json:"duration_ms"`
	Volume     int    `json:"volume"`
	Shuffle    bool   `json:"shuffle"`
	Repeat     string `json:"repeat"`
}

// Text, durumun insanlar için hali.
func (s Status) Text() string {
	if s.URI == "" {
		return "Nothing is playing."
	}
	icon := "❚❚"
	if s.Playing {
		icon = "▶"
	}
	line := icon + " " + s.Title
	if s.Artist != "" {
		line += " — " + s.Artist
	}
	var parts []string
	if s.Album != "" {
		parts = append(parts, s.Album)
	}
	parts = append(parts, Clock(s.PositionMS)+" / "+Clock(s.DurationMS))
	if s.Device != "" {
		parts = append(parts, s.Device)
	}
	parts = append(parts, fmt.Sprintf("vol %d%%", s.Volume))
	if s.Shuffle {
		parts = append(parts, "shuffle")
	}
	if s.Repeat != "" && s.Repeat != "off" {
		parts = append(parts, "repeat "+s.Repeat)
	}
	out := line + "\n  " + strings.Join(parts, " · ")
	if s.Context != "" {
		out += "\n  from " + s.Context
	}
	return out
}

// Clock, milisaniyeyi "3:07" biçiminde yazar.
func Clock(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	return fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}

// ParseSeek, sarma hedefini okur: "90", "1:30" (mutlak) ya da "+10", "-10"
// (saniye, göreli). Dönen değer milisaniye cinsinden mutlak konumdur.
func ParseSeek(arg string, currentMS int64) (int64, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return 0, errors.New("seek needs a position, like 90, 1:30, +10 or -10")
	}
	rel := arg[0] == '+' || arg[0] == '-'
	body := strings.TrimLeft(arg, "+-")
	var secs float64
	if i := strings.IndexByte(body, ':'); i >= 0 {
		m, err1 := strconv.Atoi(body[:i])
		s, err2 := strconv.ParseFloat(body[i+1:], 64)
		if err1 != nil || err2 != nil {
			return 0, fmt.Errorf("bad position %q", arg)
		}
		secs = float64(m)*60 + s
	} else {
		v, err := strconv.ParseFloat(body, 64)
		if err != nil {
			return 0, fmt.Errorf("bad position %q", arg)
		}
		secs = v
	}
	ms := int64(secs * 1000)
	if rel {
		if arg[0] == '-' {
			ms = -ms
		}
		ms += currentMS
	}
	return max(ms, 0), nil
}

// ParseVolume, ses hedefini okur: "50" ya da "+5", "-10".
func ParseVolume(arg string, current int) (int, error) {
	arg = strings.TrimSpace(arg)
	v, err := strconv.Atoi(strings.TrimPrefix(arg, "+"))
	if err != nil {
		return 0, fmt.Errorf("bad volume %q (0-100, +5, -5)", arg)
	}
	if arg[0] == '+' || arg[0] == '-' {
		v += current
	}
	return max(0, min(100, v)), nil
}

// NextRepeat, tekrar kipini okur ya da (boşsa) sıradakine geçer.
func NextRepeat(arg, current string) (string, error) {
	switch arg {
	case "":
		return map[string]string{"off": "context", "context": "track", "track": "off"}[orOff(current)], nil
	case "off", "context", "track":
		return arg, nil
	case "all", "on", "playlist":
		return "context", nil
	case "one", "song":
		return "track", nil
	}
	return "", fmt.Errorf("repeat takes off, context or track")
}

func orOff(s string) string {
	if s == "" {
		return "off"
	}
	return s
}

// ParseOnOff: "on"/"off"; boşsa current'ın tersi.
func ParseOnOff(arg string, current bool) (bool, error) {
	switch strings.ToLower(arg) {
	case "":
		return !current, nil
	case "on", "true", "1", "yes":
		return true, nil
	case "off", "false", "0", "no":
		return false, nil
	}
	return false, fmt.Errorf("expected on or off, got %q", arg)
}

// ToURI, bir Spotify bağlantısını ya da adresini "spotify:tür:id" biçimine
// çevirir: https://open.spotify.com/intl-tr/track/ID?si=… → spotify:track:ID.
func ToURI(s string) (string, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "spotify:") {
		return s, nil
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || !strings.HasSuffix(u.Host, "spotify.com") {
		return "", fmt.Errorf("not a Spotify link: %s", s)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) > 0 && strings.HasPrefix(parts[0], "intl-") {
		parts = parts[1:]
	}
	if len(parts) == 4 && parts[0] == "user" && parts[2] == "playlist" { // eski biçim
		parts = parts[2:]
	}
	if len(parts) != 2 || parts[1] == "" {
		return "", fmt.Errorf("not a Spotify link: %s", s)
	}
	switch parts[0] {
	case "track", "album", "artist", "playlist", "show", "episode":
		return "spotify:" + parts[0] + ":" + parts[1], nil
	}
	return "", fmt.Errorf("unsupported Spotify link: %s", s)
}

// Kind, bir adresin türü (track, album, …).
func Kind(uri string) string {
	p := strings.Split(uri, ":")
	if len(p) >= 3 {
		return p[len(p)-2]
	}
	return ""
}
