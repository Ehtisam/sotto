package ui

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/Ehtisam/sotto/internal/soloist"
)

const testCover = "https://example.test/cover.jpg"

func gfxModel(t *testing.T) Model {
	t.Helper()
	m := homeModel(t)
	m.width, m.height = 110, 30
	m.cfg.Look.Covers = "image"
	m.gfx = gfxState{sixel: true, cellW: 10, cellH: 20}
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 4), uint8(y * 4), 120, 255})
		}
	}
	m.rt.covers.Put(testCover, img)
	it := entity("song")
	it.Decorations.VisualIdentity.Cover = []soloist.Image{{URL: testCover, Size: "default"}}
	m.state.Item = it
	m.page, m.focus = pageNow, focusMenu
	return m
}

// run, bir komutu çalıştırıp çıkan mesajları (Batch/Sequence açılarak) toplar.
func run(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if b, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range b {
			out = append(out, run(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func TestGfxQueryAnswers(t *testing.T) {
	m := homeModel(t)
	mm, _ := m.Update(uv.PrimaryDeviceAttributesEvent{61, 4, 6})
	mm, _ = mm.(Model).Update(uv.CellSizeEvent{Width: 9, Height: 18})
	g := mm.(Model).gfx
	if !g.sixel || g.cellW != 9 || g.cellH != 18 {
		t.Fatalf("gfx = %+v", g)
	}
	mm, _ = mm.(Model).Update(uv.PrimaryDeviceAttributesEvent{61, 6, 22})
	if mm.(Model).gfx.sixel {
		t.Fatal("no 4 in the answer means no Sixel")
	}
}

func TestGfxFindsTheNowPlayingCover(t *testing.T) {
	m := gfxModel(t)
	screen := m.render()
	placed := m.rt.gfx.placed
	if len(placed) != 1 {
		t.Fatalf("placed = %+v", placed)
	}
	p := placed[0]
	// İçerik sütunu: kenar + menü + boşluk; satır: başlık, çizgi, sayfa adı, boş.
	if p.url != testCover || p.x != margin+menuWidth+gutter || p.y != 4 || p.cols != 2*p.rows || p.rows < 5 {
		t.Fatalf("place = %+v", p)
	}
	if strings.Contains(screen, "▀") {
		t.Fatal("image mode should not draw pixel blocks for the cover")
	}
}

func TestGfxSendsImageAfterAFrame(t *testing.T) {
	m := gfxModel(t)
	_ = m.render()
	// İlk kontrol: kapak henüz Sixel'e çevrilmedi → arka planda çevir.
	cmd := m.gfxCheck()
	msgs := run(cmd)
	if len(msgs) != 1 {
		t.Fatalf("first check = %v", msgs)
	}
	if _, ok := msgs[0].(gfxBuiltMsg); !ok {
		t.Fatalf("first check = %T", msgs[0])
	}
	// İkinci kontrol: resim yerine gönderilir.
	mm, cmd := m.Update(gfxBuiltMsg{})
	m = mm.(Model)
	msgs = run(cmd)
	if len(msgs) != 1 {
		t.Fatalf("second check = %v", msgs)
	}
	raw, ok := msgs[0].(tea.RawMsg)
	if !ok {
		t.Fatalf("second check = %T", msgs[0])
	}
	s := fmt.Sprint(raw.Msg)
	p := m.rt.gfx.placed[0]
	if !strings.HasPrefix(s, "\x1b7"+fmt.Sprintf("\x1b[%d;%dH", p.y+1, p.x+1)+"\x1bP") || !strings.HasSuffix(s, "\x1b\\\x1b8") {
		t.Fatalf("raw = %q…", s[:min(40, len(s))])
	}
	// Değişiklik yoksa bir şey gönderilmez.
	if cmd := m.gfxCheck(); cmd != nil {
		t.Fatal("nothing changed, nothing should be sent")
	}
	// Sayfa değişince kapak menüye taşınır: önce ekran temizlenir.
	m.page = pageHome
	_ = m.render()
	if cmd := m.gfxCheck(); cmd == nil || len(m.rt.gfx.drawn) != 0 {
		t.Fatal("a moved image should clear the screen first")
	}
}

func TestGfxOffWhenNoSixelOrPixelMode(t *testing.T) {
	m := gfxModel(t)
	m.gfx.sixel = false
	if s := m.render(); !strings.Contains(s, "▀") || len(m.rt.gfx.placed) != 0 {
		t.Fatal("without Sixel the cover falls back to pixel blocks")
	}
	m = gfxModel(t)
	m.cfg.Look.Covers = "pixel"
	if s := m.render(); !strings.Contains(s, "▀") || len(m.rt.gfx.placed) != 0 {
		t.Fatal("pixel mode draws blocks")
	}
}

// Kırpılan alan (alt köşesi görünmeyen) çizilmez; yoksa resim şeride taşar.
func TestGfxSkipsClippedSlots(t *testing.T) {
	slots := []gfxSlot{{url: "u", cols: 4, rows: 3}}
	m := homeModel(t)
	box := m.gfxPlaceholder("u", 4, 3)
	lines := strings.Split(box, "\n")
	m.rt.gfx.slots = nil
	if got := findPlaces(strings.Join(lines[:2], "\n"), slots); len(got) != 0 {
		t.Fatalf("clipped slot placed: %+v", got)
	}
	if got := findPlaces("ab"+lines[0]+"\n  "+lines[1]+"\n  "+lines[2], slots); len(got) != 1 || got[0].x != 2 || got[0].y != 0 {
		t.Fatalf("full slot = %+v", got)
	}
}

// Geniş karakterli satır (Bubble Tea onu her değişen karede silip baştan
// yazar) resmin yanındaysa, o satırın şeridi her karenin içine eklenir.
func TestGfxStripsRideAlongWithFrames(t *testing.T) {
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	out := NewOutput(f)
	m := gfxModel(t).WithOutput(out)
	m.state.Item.Decorations.Identity.Name = "雨の歌" // şarkı adı kapağın yanında
	_ = m.render()
	if out.after != "" {
		t.Fatal("nothing is on screen yet, nothing to add")
	}
	run(m.gfxCheck()) // çevir
	m.gfxCheck()      // resmi gönder
	_ = m.render()
	p := m.rt.gfx.drawn[0]
	// Geniş satır kapağın ikinci satırında (bağlam, boş, ad…): yalnızca o şerit.
	var rows []int
	for k := 0; k < p.rows; k++ {
		if strings.Contains(out.after, fmt.Sprintf("\x1b7\x1b[%d;%dH\x1bP", p.y+k+1, p.x+1)) {
			rows = append(rows, k)
		}
	}
	if len(rows) != 1 {
		t.Fatalf("strips for rows %v, want exactly the wide line's", rows)
	}
	// Eşzamanlı çıktı varsa şerit kapanıştan önce gider.
	frame := []byte("\x1b[?2026hFRAME\x1b[?2026l")
	got := string(withAfter(frame, "STRIP"))
	if got != "\x1b[?2026hFRAMESTRIP\x1b[?2026l" {
		t.Fatalf("withAfter = %q", got)
	}
	if got := string(withAfter([]byte("X"), "S")); got != "XS" {
		t.Fatalf("withAfter plain = %q", got)
	}
	// Yazı: kare + şeritler.
	if n, err := out.Write([]byte("FRAME")); n != 5 || err != nil {
		t.Fatalf("write = %d %v", n, err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "out"))
	if !strings.HasPrefix(string(b), "FRAME\x1b7") {
		t.Fatalf("written = %q", string(b)[:20])
	}
}

func TestGfxSquareOnWindowsTerminal(t *testing.T) {
	t.Setenv("WT_SESSION", "x")
	m := gfxModel(t)
	if r := m.cellRatio(); r != 0.45 {
		t.Fatalf("ratio = %v, want 0.45 on Windows Terminal's 10x20", r)
	}
	sp := m.spec(gfxSlot{url: testCover, cols: 18, rows: 9})
	// 18 sütun x 0.45 = 8.1 satır yüksekliğinde kare: 180 x 162 piksel, 180 x 180 kutuda.
	if sp.W != 180 || sp.H != 180 || sp.IW != 180 || sp.IH != 162 {
		t.Fatalf("spec = %+v", sp)
	}
	m.cfg.Look.CellRatio = 0.5
	if sp := m.spec(gfxSlot{url: testCover, cols: 18, rows: 9}); sp.IW != 180 || sp.IH != 180 {
		t.Fatalf("ratio 0.5 should fill the box: %+v", sp)
	}
	// Gerçek hücre boyutunu bildiren terminal (WezTerm, kitty…): o kullanılır.
	t.Setenv("WT_SESSION", "")
	m.cfg.Look.CellRatio = 0
	m.gfx.cellW, m.gfx.cellH = 9, 18
	if r := m.cellRatio(); r != 0.5 {
		t.Fatalf("reported cell ratio = %v", r)
	}
}

func TestGfxRedrawsAfterResize(t *testing.T) {
	m := gfxModel(t)
	_ = m.render()
	run(m.gfxCheck())
	m.gfxCheck()
	mm, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 32})
	m = mm.(Model)
	if cmd == nil || m.rt.gfx.drawn != nil {
		t.Fatal("resize should forget what was drawn and schedule redraws")
	}
}

// Çıkışta (alternatif ekrandan çıkış) ve sonrasında şerit eklenmez; yoksa
// resim kabuğun ekranında kalır.
func TestOutputStopsAtExit(t *testing.T) {
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	out := NewOutput(f)
	out.setAfter("STRIP")
	_, _ = out.Write([]byte("frame"))
	_, _ = out.Write([]byte("last frame\x1b[?1049l"))
	_, _ = out.Write([]byte("bye"))
	b, _ := os.ReadFile(filepath.Join(dir, "out"))
	if got := string(b); got != "frameSTRIPlast frame\x1b[?1049lbye" {
		t.Fatalf("written = %q", got)
	}
}

// q ile çıkarken şerit eklemek durur ve ekran (resimlerle) silinir.
func TestQuitStopsStripsAndClearsImages(t *testing.T) {
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	out := NewOutput(f)
	m := gfxModel(t).WithOutput(out)
	out.setAfter("STRIP")
	_, cmd := m.Update(keyPress("q"))
	if !out.off || out.after != "" {
		t.Fatal("quitting should stop the strips at once")
	}
	if cmd == nil {
		t.Fatal("quit should return a command")
	}
	// Pikselde (resim yokken) doğrudan çıkılır.
	m2 := homeModel(t)
	if _, cmd := m2.Update(keyPress("q")); fmt.Sprint(cmd()) != fmt.Sprint(tea.Quit()) {
		t.Fatal("without images q just quits")
	}
}
