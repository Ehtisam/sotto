package lyrics

import (
	"testing"
	"time"
)

func TestParseLRC(t *testing.T) {
	l := ParseLRC("[ar:x]\n[00:14.98] \n[00:10.98] I am Iron Man\n[01:02.5] Has he lost his mind?\n")
	if !l.Synced || len(l.Lines) != 3 {
		t.Fatalf("got %+v", l)
	}
	if l.Lines[0].Text != "I am Iron Man" || l.Lines[0].Start != 10980*time.Millisecond {
		t.Fatalf("first line = %+v", l.Lines[0])
	}
	if l.Lines[1].Text != "♪" {
		t.Fatalf("empty line = %q", l.Lines[1].Text)
	}
	if l.Lines[2].Start != 62500*time.Millisecond {
		t.Fatalf("minutes: %v", l.Lines[2].Start)
	}
}

func TestAt(t *testing.T) {
	l := ParseLRC("[00:10.00] a\n[00:20.00] b\n[00:30.00] c")
	cases := map[time.Duration]int{5 * time.Second: -1, 10 * time.Second: 0, 25 * time.Second: 1, 99 * time.Second: 2}
	for pos, want := range cases {
		if got := l.At(pos); got != want {
			t.Errorf("At(%v) = %d, want %d", pos, got, want)
		}
	}
	if (&Lyrics{Lines: l.Lines}).At(25*time.Second) != -1 {
		t.Error("unsynced lyrics have no current line")
	}
}
