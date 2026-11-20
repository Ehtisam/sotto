package cli

import "testing"

func TestMuteTarget(t *testing.T) {
	cases := []struct {
		cur, saved int
		want       string
		vol, save  int
		muted      bool
	}{
		{40, 0, "", 0, 40, true},
		{0, 40, "", 40, 0, false},
		{0, 0, "", 50, 0, false},
		{40, 0, "off", 40, 0, false},
		{0, 30, "on", 0, 30, true},
		{60, 30, "on", 0, 60, true},
	}
	for _, c := range cases {
		v, s, mu := MuteTarget(c.cur, c.saved, c.want)
		if v != c.vol || s != c.save || mu != c.muted {
			t.Errorf("MuteTarget(%d,%d,%q) = %d,%d,%v", c.cur, c.saved, c.want, v, s, mu)
		}
	}
}

func TestMutedVolumeFile(t *testing.T) {
	dir := t.TempDir()
	SaveMuted(dir, 35)
	if MutedVolume(dir) != 35 {
		t.Fatal("not saved")
	}
	SaveMuted(dir, 0)
	if MutedVolume(dir) != 0 {
		t.Fatal("not cleared")
	}
}
