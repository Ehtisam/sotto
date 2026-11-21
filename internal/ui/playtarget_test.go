package ui

import (
	"context"
	"errors"
	"testing"

	"github.com/Ehtisam/sotto/internal/spotify"
)

// Müzik telefonda çalarken sotto'dan şarkı seçmek onu telefonda bırakmalı,
// Soloist açık diye bilgisayara çekmemeli. Hiçbir cihaz çalmıyorsa Soloist.
func TestPlayTargetKeepsActiveDevice(t *testing.T) {
	f, m := newFakeWeb(t)
	ctx := context.Background()
	f.on("GET me/player/devices", `{"devices":[{"id":"so","name":"sotto","is_active":false},{"id":"ph","name":"Phone","is_active":true}]}`)
	if id, el := playTarget(ctx, m.sp, "sotto"); id != "ph" || el != "" {
		t.Fatalf("phone playing, Soloist on: id=%q elsewhere=%q", id, el)
	}
	if id, el := playTarget(ctx, m.sp, ""); id != "ph" || el != "Phone" {
		t.Fatalf("phone playing, Soloist off: id=%q elsewhere=%q", id, el)
	}
	f.on("GET me/player/devices", `{"devices":[{"id":"so","name":"sotto","is_active":false},{"id":"ph","name":"Phone","is_active":false}]}`)
	if id, _ := playTarget(ctx, m.sp, "sotto"); id != "so" {
		t.Fatalf("nothing playing: id=%q, want Soloist", id)
	}
	if id, _ := playTarget(ctx, m.sp, ""); id != "" {
		t.Fatalf("nothing playing, Soloist off: id=%q", id)
	}
}

func TestTransferRetryable(t *testing.T) {
	for code, want := range map[int]bool{500: true, 502: true, 503: true, 404: true, 403: false, 401: false} {
		if got := transferRetryable(&spotify.HTTPError{Status: code}); got != want {
			t.Errorf("%d: %v", code, got)
		}
	}
	if transferRetryable(errors.New("x")) {
		t.Error("plain error retried")
	}
}
