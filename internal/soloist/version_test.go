package soloist

import (
	"testing"
	"time"
)

func TestParseVersion(t *testing.T) {
	b, err := ParseVersion("soloist 1.3.10.87 build 1795111339 (20261119) (g19412df6ea) (windows/x86_64)\n")
	if err != nil {
		t.Fatal(err)
	}
	if b.Version != "1.3.10.87" || b.Built.UTC().Format("2006-01-02") != "2026-11-19" {
		t.Fatalf("%+v", b)
	}
	if got := b.DaysLeft(b.Built.Add(10 * 24 * time.Hour)); got != 80 {
		t.Fatalf("days left = %d", got)
	}
	if _, err := ParseVersion("hello"); err == nil {
		t.Fatal("parsed nonsense")
	}
}
