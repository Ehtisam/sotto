package ui

import (
	"strings"
	"testing"
)

// Sırayı temizle: Sıra sayfasında shift+C gerçek programda clearQueue'ya
// ulaşır (menüdeyken de, listenin içindeyken de). Çevrimdışı ekstralar
// kapalı: istek gitmez, ne gerektiği söylenir; sıra olduğu gibi kalır.
func TestDriveQueueClear(t *testing.T) {
	d, f := driveOffline(t)
	f.on("GET me/player?additional_types", `{"device":{"id":"ph","name":"Phone","volume_percent":50},"is_playing":true,"progress_ms":1000,
		"shuffle_state":false,"repeat_state":"off",
		"item":{"name":"Song 01","uri":"spotify:track:t1","type":"track","duration_ms":200000,"artists":[{"name":"Artist 1"}],"album":{"name":"Album"}}}`)
	f.on("GET me/player/queue", `{"currently_playing":{"name":"Song 01","uri":"spotify:track:t1","type":"track","duration_ms":200000,"artists":[{"name":"Artist 1"}]},
		"queue":[{"name":"Song 02","uri":"spotify:track:t2","type":"track","duration_ms":200000,"artists":[{"name":"Artist 2"}]}]}`)
	d.do("<wait:1500>gq<wait:800>")
	if m := d.model(); m.page != pageQueue {
		t.Fatalf("g q → page %v", m.page)
	}
	if s := d.screen(); !strings.Contains(s, "Song 02") {
		t.Fatalf("queue page:\n%s", s)
	}
	d.do("j<wait:100>C<wait:300>")
	if m := d.model(); !strings.Contains(m.flash, "extras") || len(m.queue.Upcoming) != 1 || m.qs.editing {
		t.Fatalf("flash %q, upcoming %d, editing %v", m.flash, len(m.queue.Upcoming), m.qs.editing)
	}
}
