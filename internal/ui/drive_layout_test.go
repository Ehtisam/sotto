package ui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Kutulu düzen ve menünün yeri: Ayarlar'dan değişir, ekran pencereye tam
// sığar, fareyle menüye ve satırlara tıklama yeni yerlerde de doğru çalışır.
func TestDriveLayoutBoxesAndMenu(t *testing.T) {
	d, _ := driveOffline(t)
	click := func(x, y int) {
		d.raw(fmt.Sprintf("\x1b[<0;%d;%dM\x1b[<0;%d;%dm", x+1, y+1, x+1, y+1))
		d.do("<wait:250>")
	}
	fits := func(what string) []string {
		lines := strings.Split(d.screen(), "\n")
		if len(lines) != 32 {
			t.Fatalf("%s: %d lines, want 32\n%s", what, len(lines), d.screen())
		}
		for _, l := range lines {
			if lipgloss.Width(l) > 110 {
				t.Fatalf("%s: line too wide (%d): %q", what, lipgloss.Width(l), l)
			}
		}
		return lines
	}
	goTo := func(id settingID) {
		index := func(x settingID) int {
			for i, s := range settingIDs() {
				if s == x {
					return i
				}
			}
			return -1
		}
		for i := 0; i < 40 && d.model().settingSelected() != id; i++ {
			if index(d.model().settingSelected()) > index(id) {
				d.do("<up><wait:50>")
			} else {
				d.do("<down><wait:50>")
			}
		}
		if d.model().settingSelected() != id {
			m := d.model()
			t.Fatalf("row %v not reachable (page %v focus %v selected %v)\n%s", id, m.page, m.focus, m.settingSelected(), d.screen())
		}
	}
	// clickPage, menüde verilen sayfaya tıklar (menü konumu layout'tan).
	clickPage := func(p page) {
		l := d.model().layout()
		click(l.menuX+3, l.bodyTop+2+int(p))
	}

	d.do("8<wait:400>")
	goTo(setLayout)
	d.do("<right><wait:300>")
	lines := fits("boxed")
	s := strings.Join(lines, "\n")
	if !strings.Contains(s, "╭") || !strings.Contains(s, "╰") || !strings.Contains(s, "│") {
		t.Fatalf("no boxes:\n%s", s)
	}
	if !strings.Contains(s, "Layout") || !strings.Contains(s, "boxes") {
		t.Fatalf("layout row not updated:\n%s", s)
	}
	// Kutulu düzende fare: menüde Kitaplık'a tıkla.
	clickPage(pageLibrary)
	if m := d.model(); m.page != pageLibrary {
		t.Fatalf("boxed: menu click went to page %v\n%s", m.page, d.screen())
	}
	// Liste açılıp ikinci satıra tıklayınca o seçilir.
	d.do("j<enter><wait:400>")
	l := d.model().layout()
	start := d.model().lib.open.pos()
	row := -1
	for i, line := range strings.Split(d.screen(), "\n") {
		if strings.Contains(ansi.Strip(line), "Song 03") {
			row = i
		}
	}
	if row < 0 {
		t.Fatalf("Song 03 not on screen:\n%s", d.screen())
	}
	click(l.contentX+8, row)
	if m := d.model(); m.lib.open.tracks[m.lib.open.cursor].Name != "Song 03" {
		t.Fatalf("boxed: row click selected %q (start %d)\n%s", m.lib.open.tracks[m.lib.open.cursor].Name, start, d.screen())
	}
	d.t.Log("\n" + d.screen())

	// Menü sağda: içerik solda başlar, menü tıklaması yine doğru sayfaya gider.
	d.do("<esc><wait:200>g,<wait:300>")
	goTo(setMenuPos)
	d.do("<right><wait:300>")
	if m := d.model(); m.cfg.Look.Menu != "right" || !m.layout().menuRight {
		t.Fatalf("menu = %q", m.cfg.Look.Menu)
	}
	fits("menu right")
	for _, line := range strings.Split(ansi.Strip(d.screen()), "\n") {
		if strings.Contains(line, "VIII Settings") && strings.Index(line, "VIII") < 60 {
			t.Fatalf("menu not on the right: %q", line)
		}
	}
	clickPage(pageQueue)
	if d.model().page != pageQueue {
		t.Fatalf("right menu click: page %v\n%s", d.model().page, d.screen())
	}
	// Gizli: menü yok, rakamlar yine çalışır.
	d.do("g,<wait:300>")
	goTo(setMenuPos)
	d.do("<right><wait:300>")
	fits("menu hidden")
	if s := d.screen(); strings.Contains(s, "VIII Settings") {
		t.Fatalf("menu still drawn:\n%s", s)
	}
	d.do("<esc><wait:100><esc><wait:100>4<wait:300>")
	if d.model().page != pageLibrary {
		t.Fatal("4 should still open the Library")
	}
	// Şerit üstte + kutular: yine sığar.
	d.do("g,<wait:300>")
	goTo(setStrip)
	d.do("<right><wait:300>")
	fits("strip top boxed")
	d.t.Log("\n" + d.screen())
}
