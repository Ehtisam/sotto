package cli

import (
	"errors"
	"strings"

	"github.com/Ehtisam/sotto/internal/spotify"
)

// Spotify'ın 403 yanıtlarının anlaşılır karşılıkları. Bunlar sotto'nun değil
// cihazın kısıtı: telefonlar sesini uzaktan değiştirtmiyor, bazı durumlarda
// karıştırma/tekrar da kapalı.

// Friendly, Spotify'ın kısıt hatalarını açıklamalı hataya çevirir; başka
// hataları olduğu gibi bırakır.
func Friendly(err error, device string) error {
	if err == nil || !spotify.IsStatus(err, 403) {
		return err
	}
	if device == "" {
		device = "This device"
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "volume"):
		return errors.New(RefusalText("volume", device))
	case strings.Contains(msg, "Restriction violated"):
		return errors.New(RefusalText("", device))
	}
	return err
}

// Disallowed, komutun Spotify'ın bildirdiği kısıtlara takılıp takılmadığını söyler.
// action: "volume", "shuffle", "repeat:context", "repeat:track", "repeat:off", "seek", "next", "prev".
func Disallowed(st *spotify.PlayerState, action string, currentRepeat string) bool {
	if st == nil {
		return false
	}
	d := st.Actions.Disallows
	switch action {
	case "volume":
		return st.Device.SupportsVolume != nil && !*st.Device.SupportsVolume
	case "shuffle":
		return d["toggling_shuffle"]
	case "repeat:context":
		return d["toggling_repeat_context"]
	case "repeat:track":
		return d["toggling_repeat_track"]
	case "repeat:off":
		return (currentRepeat == "context" && d["toggling_repeat_context"]) || (currentRepeat == "track" && d["toggling_repeat_track"])
	case "seek":
		return d["seeking"]
	case "next":
		return d["skipping_next"]
	case "prev":
		return d["skipping_prev"]
	}
	return false
}

// RefusalText, kısıt için gösterilecek ileti.
func RefusalText(action, device string) string {
	if device == "" {
		device = "This device"
	}
	if action == "volume" {
		return device + " doesn't let other devices change its volume; change it on the device itself"
	}
	return device + " doesn't allow that from here right now"
}
