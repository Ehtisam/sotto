package discord

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/Ehtisam/sotto/internal/config"
)

// Açık Discord'la gerçek deneme: sotto'nun uygulama kimliği kabul ediliyor
// mu, SET_ACTIVITY hatasız dönüyor mu. Etkinlik hemen temizlenir.
//
//	SOTTO_LIVE_DISCORD=1 go test ./internal/discord -run Live -v
func TestLiveDiscordAppID(t *testing.T) {
	if os.Getenv("SOTTO_LIVE_DISCORD") == "" {
		t.Skip("SOTTO_LIVE_DISCORD not set")
	}
	c, err := dialIPC()
	if err != nil {
		t.Fatalf("Discord not reachable: %v", err)
	}
	defer c.Close()
	if err := send(c, opHandshake, map[string]any{"v": 1, "client_id": config.DefaultDiscordAppID}); err != nil {
		t.Fatal(err)
	}
	op, body, err := read(c)
	if err != nil || op == opClose {
		t.Fatalf("handshake: op=%d %s %v", op, body, err)
	}
	var ready struct {
		Evt  string `json:"evt"`
		Data struct {
			User struct {
				Username string `json:"username"`
			} `json:"user"`
		} `json:"data"`
	}
	_ = json.Unmarshal(body, &ready)
	t.Logf("handshake: evt=%s", ready.Evt)
	if ready.Evt != "READY" {
		t.Fatalf("no READY: %s", body)
	}
	now := time.Now()
	a := &Activity{Title: "sotto test", Artist: "sotto", Start: now, End: now.Add(3 * time.Minute),
		URL: "https://open.spotify.com/"}
	if err := send(c, opFrame, setActivity(a)); err != nil {
		t.Fatal(err)
	}
	_, body, err = read(c)
	var resp struct {
		Evt  *string        `json:"evt"`
		Data map[string]any `json:"data"`
	}
	_ = json.Unmarshal(body, &resp)
	if err != nil || resp.Evt != nil {
		t.Fatalf("SET_ACTIVITY: %s %v", body, err)
	}
	t.Logf("SET_ACTIVITY accepted: name=%v type=%v application_id=%v", resp.Data["name"], resp.Data["type"], resp.Data["application_id"])
	if err := send(c, opFrame, setActivity(nil)); err != nil {
		t.Fatal(err)
	}
	if _, body, err = read(c); err != nil {
		t.Fatalf("clear: %v", err)
	}
	t.Logf("cleared: %s", body)
}
