package history

import (
	"os"
	"testing"
	"time"
)

func TestAppendLoadCompute(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 21, 20, 0, 0, 0, time.Local) // Cumartesi
	add := func(ago time.Duration, uri, title, artist string, ms int64) {
		if err := Append(dir, Play{At: now.Add(-ago), URI: uri, Title: title, Artist: artist, MS: ms}); err != nil {
			t.Fatal(err)
		}
	}
	add(time.Hour, "spotify:track:a", "A", "X", 180000)
	add(2*time.Hour, "spotify:track:a", "A", "X", 120000)
	add(3*time.Hour, "spotify:track:b", "B", "X, Y", 200000)
	add(20*24*time.Hour, "spotify:track:c", "C", "Z", 100000)  // ayda, haftada değil
	add(400*24*time.Hour, "spotify:track:d", "D", "Z", 100000) // yalnız hepsinde
	// Bozuk satır atlanır.
	f, _ := os.OpenFile(File(dir), os.O_APPEND|os.O_WRONLY, 0o600)
	f.WriteString("{broken\n")
	f.Close()

	plays, err := Load(dir)
	if err != nil || len(plays) != 5 {
		t.Fatalf("load %d %v", len(plays), err)
	}
	w := Compute(plays, Week, now, 10)
	if w.Plays != 3 || w.Songs != 2 || w.Artists != 2 || w.MS != 500000 || w.Days != 1 {
		t.Fatalf("week %+v", w)
	}
	if w.Top[0].Name != "A" || w.Top[0].Plays != 2 {
		t.Fatalf("top %+v", w.Top)
	}
	if w.TopArt[0].Name != "X" || w.TopArt[0].Plays != 3 {
		t.Fatalf("top artists %+v", w.TopArt)
	}
	if w.ByHour[19] != 1 || w.ByHour[18] != 1 || w.ByHour[17] != 1 || w.ByDay[5] != 3 {
		t.Fatalf("hours %v days %v", w.ByHour, w.ByDay)
	}
	if w.Recent[0].Title != "A" || w.Recent[2].Title != "B" {
		t.Fatalf("recent order %v", w.Recent)
	}
	if m := Compute(plays, Month, now, 10); m.Plays != 4 {
		t.Fatalf("month %d", m.Plays)
	}
	if a := Compute(plays, All, now, 10); a.Plays != 5 {
		t.Fatalf("all %d", a.Plays)
	}
}

func TestCounts(t *testing.T) {
	if Counts(20*time.Second, 3*time.Minute) || !Counts(31*time.Second, 3*time.Minute) {
		t.Fatal("30 s rule")
	}
	if !Counts(25*time.Second, 40*time.Second) || Counts(3*time.Second, 5*time.Second) {
		t.Fatal("half-of-a-short-song rule")
	}
}
