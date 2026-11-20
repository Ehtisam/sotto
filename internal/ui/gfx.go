package ui

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/Ehtisam/sotto/internal/cover"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// Gerçek kapak (Sixel). Bubble Tea ekranı hücre hücre çizer; resmi o
// çizimin dışından, ham çıktı olarak göndeririz:
//
//  1. Resim modunda coverBox kapak yerine aynı boyutta boş bir alan çizer.
//     Alanın ilk ve son satırının ilk hücresine zeminden ayırt edilemeyen
//     bir işaret rengi konur.
//  2. render, çizilen ekranda bu işaretleri arayıp resmin yerini bulur.
//  3. Ekran çizildikten bir kare sonra (gfxCheckMsg) resim o yere gönderilir.
//     Bubble Tea ham çıktıyı kareden önce yazar; beklemezsek kare resmi siler.
//     Değişmeyen hücreleri yeniden yazmadığı için resim yerinde kalır.
//  4. Bir resim kalkar ya da yer değiştirirse ekran bir kez temizlenip
//     yeniden çizilir (yoksa eski resim boş hücrelerde kalırdı).
//  5. Bubble Tea geniş karakterli (雨, 「…) satırları her değişen karede
//     baştan siler ve yazar; resmin yanındaki böyle bir satır resmin o
//     şeridini siler. render bu satırları bulur, Output (output.go) o
//     satırların şeritlerini her karenin içine, silmenin hemen arkasına
//     ekler (yalnızca o satır, birkaç KB). Titreme olmaz.

// Terminal soruları: Sixel desteği (DA1 cevabında 4) ve hücre piksel boyutu.
const gfxQuery = ansi.RequestPrimaryDeviceAttributes + "\x1b[16t"

// gfxDelay: ekranın çizilmesine tanınan süre (Bubble Tea saniyede 60 kare).
const gfxDelay = 30 * time.Millisecond

type gfxState struct {
	sixel        bool // terminal Sixel destekliyor
	cellW, cellH int  // hücre piksel boyutu (Sixel için)
}

// gfxSlot, bir çizimde coverBox'un resim için ayırdığı alan.
type gfxSlot struct {
	url        string
	cols, rows int
}

// gfxPlace, resmin ekrandaki yeri (0'dan başlayan sütun ve satır).
type gfxPlace struct {
	gfxSlot
	x, y int
}

// gfxShared, çizimler arasında paylaşılan resim durumu (shared içinde).
type gfxShared struct {
	slots   []gfxSlot  // son çizimde ayrılan alanlar (sıra = işaret numarası)
	placed  []gfxPlace // son çizimde bulunan yerler
	drawn   []gfxPlace // ekranda şu an duranlar
	pending bool       // bir gfxCheckMsg yolda
}

type gfxCheckMsg struct{}

type gfxBuiltMsg struct{}

// gfxRedrawMsg: ekrandakiler silinmiş sayılır, hepsi yeniden gönderilir.
type gfxRedrawMsg struct{}

// imagesOn: kapaklar gerçek resim olarak çizilsin mi.
func (m Model) imagesOn() bool { return m.cfg.Look.Covers == "image" && m.gfx.sixel }

// cellSize, Sixel'in hücre başına piksel sayısı.
func (m Model) cellSize() (int, int) {
	if m.gfx.cellW > 0 && m.gfx.cellH > 0 {
		return m.gfx.cellW, m.gfx.cellH
	}
	return 10, 20
}

// cellRatio, hücrenin ekrandaki gerçek en/boy oranı. Çoğu terminal hücre
// boyutunu doğru bildirir; Windows Terminal ise Sixel için hep 10x20 der,
// gerçek yazı tiplerinde oran ~0.45'tir (Cascadia Mono, Consolas…).
func (m Model) cellRatio() float64 {
	if r := m.cfg.Look.CellRatio; r > 0.2 && r < 1 {
		return r // ayar dosyasından elle
	}
	w, h := m.gfx.cellW, m.gfx.cellH
	if w > 0 && h > 0 && !(w == 10 && h == 20 && os.Getenv("WT_SESSION") != "") {
		return float64(w) / float64(h)
	}
	return 0.45
}

func groundRGB() [3]uint8 {
	r, g, b, _ := theme.Ground.RGBA()
	return [3]uint8{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)}
}

// spec, bir yerdeki resmin Sixel tarifi: kutu dolusu zemin, ortasında
// gerçekte kare görünen kapak.
func (m Model) spec(s gfxSlot) cover.Spec {
	cw, ch := m.cellSize()
	w, h := s.cols*cw, s.rows*ch
	// Sixel pikseli ekranda (gerçek hücre / Sixel hücresi) kadar büyür.
	pixelRatio := m.cellRatio() * float64(ch) / float64(cw)
	iw, ih := cover.FitSquare(w, h, pixelRatio)
	return cover.Spec{URL: s.url, W: w, H: h, IW: iw, IH: ih, RowH: ch, BG: groundRGB()}
}

// updateGfx, terminal cevaplarını ve resim mesajlarını işler. ok=false: mesaj bu dosyanın değil.
func (m Model) updateGfx(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case uv.PrimaryDeviceAttributesEvent:
		m.gfx.sixel = slices.Contains([]int(msg), 4)
		return m, nil, true
	case uv.CellSizeEvent:
		m.gfx.cellW, m.gfx.cellH = msg.Width, msg.Height
		return m, nil, true
	case gfxRedrawMsg:
		m.rt.gfx.drawn = nil
		m.rt.gfx.pending = false
		return m, m.gfxCheck(), true
	case gfxCheckMsg, gfxBuiltMsg:
		m.rt.gfx.pending = false
		return m, m.gfxCheck(), true
	}
	return m, nil, false
}

// afterResize, boyut değişince resimleri iki kez yeniden gönderir: Bubble
// Tea ekranı baştan çizer, terminal de kendi yeniden çizimini geciktirebilir.
func (m Model) afterResize() tea.Cmd {
	if !m.imagesOn() {
		return nil
	}
	redraw := func(time.Time) tea.Msg { return gfxRedrawMsg{} }
	return tea.Batch(tea.Tick(150*time.Millisecond, redraw), tea.Tick(500*time.Millisecond, redraw))
}

// scheduleGfx, resim modundaysa bir sonraki karede yerleri kontrol ettirir.
func (m Model) scheduleGfx() tea.Cmd {
	g := &m.rt.gfx
	if g.pending || (!m.imagesOn() && len(g.drawn) == 0) {
		return nil
	}
	g.pending = true
	return tea.Tick(gfxDelay, func(time.Time) tea.Msg { return gfxCheckMsg{} })
}

// gfxCheck, ekrandaki resimleri son çizimle karşılaştırır ve gerekeni gönderir.
func (m Model) gfxCheck() tea.Cmd {
	g := &m.rt.gfx
	placed := g.placed
	if !m.imagesOn() {
		placed = nil
	}
	// Kalkan ya da yeri değişen bir resim varsa ekranı temizlet; temizlik
	// bütün resimleri siler, sonraki kontrol hepsini yeniden çizer.
	for _, d := range g.drawn {
		if !slices.ContainsFunc(placed, func(p gfxPlace) bool { return sameRect(p, d) }) {
			g.drawn = nil
			g.pending = true
			return tea.Sequence(tea.ClearScreen, tea.Tick(gfxDelay, func(time.Time) tea.Msg { return gfxCheckMsg{} }))
		}
	}
	// Çevrilmemiş kapak varsa önce arka planda çevir.
	var missing []cover.Spec
	for _, p := range placed {
		if _, _, ok := m.rt.covers.SixelReady(m.spec(p.gfxSlot)); !ok {
			missing = append(missing, m.spec(p.gfxSlot))
		}
	}
	if len(missing) > 0 {
		store := m.rt.covers
		g.pending = true
		return func() tea.Msg {
			for _, sp := range missing {
				_ = store.BuildSixel(sp)
			}
			return gfxBuiltMsg{}
		}
	}
	var b strings.Builder
	at := func(x, y int, seq string) {
		b.WriteString("\x1b7" + ansi.CursorPosition(x+1, y+1) + seq + "\x1b8")
	}
	// Yeni ya da değişen resimler bütün olarak.
	for _, p := range placed {
		if slices.Contains(g.drawn, p) {
			continue
		}
		full, _, _ := m.rt.covers.SixelReady(m.spec(p.gfxSlot))
		at(p.x, p.y, full)
	}
	g.drawn = slices.Clone(placed)
	if b.Len() == 0 {
		return nil
	}
	return tea.Raw(b.String())
}

// quit, sotto'yu kapatır. Resim modundaysa önce şerit eklemeyi durdurur ve
// ekranı (resimlerle birlikte) sildirir: yoksa Windows Terminal son çizilen
// şeridi kabuğun ekranında bırakabiliyor. Silmeyi Bubble Tea'ye yaptırıyoruz
// (ClearScreen son karede, alternatif ekrandan çıkmadan önce uygulanır); ham
// ESC[2J ise çıkıştan sonra yazılıp kabuğun ekranını siliyordu.
func (m Model) quit() tea.Cmd {
	if !m.imagesOn() && len(m.rt.gfx.drawn) == 0 {
		return tea.Quit
	}
	m.rt.out.stop()
	m.rt.gfx.drawn = nil
	return tea.Sequence(tea.ClearScreen, tea.Quit)
}

func sameRect(a, b gfxPlace) bool {
	return a.x == b.x && a.y == b.y && a.cols == b.cols && a.rows == b.rows
}

// stripsAfter, bu karede Bubble Tea'nin baştan yazacağı (silip yazacağı)
// satırlardan ekrandaki bir resmin üstüne düşenlerin şeritleri. Output
// bunları karenin içine ekler.
func (m Model) stripsAfter(screen string) string {
	g := &m.rt.gfx
	if len(g.drawn) == 0 {
		return ""
	}
	lines := strings.Split(screen, "\n")
	var b strings.Builder
	for _, p := range g.drawn {
		if !slices.Contains(g.placed, p) {
			continue // bu karede yeri değişti; gfxCheck ilgilenir
		}
		_, strips, ok := m.rt.covers.SixelReady(m.spec(p.gfxSlot))
		if !ok {
			continue
		}
		for k := 0; k < p.rows && k < len(strips); k++ {
			if y := p.y + k; y < len(lines) && driftLine(lines[y]) {
				b.WriteString("\x1b7" + ansi.CursorPosition(p.x+1, y+1) + strips[k] + "\x1b8")
			}
		}
	}
	return b.String()
}

// driftLine: Bubble Tea'nin her karede baştan yazdığı türden bir satır mı
// (genişliği 1 olmayan bir karakter içeriyor: geniş harf, birleşen işaret…).
func driftLine(line string) bool {
	for _, r := range ansi.Strip(line) {
		if r >= 0x80 && ansi.StringWidth(string(r)) != 1 {
			return true
		}
	}
	return false
}

// ---- yer tutucu ve işaretler ----

// gfxPlaceholder, resmin yerine çizilen boş alan; ilk ve son satırın ilk
// hücresinde bu alanın işaretleri var.
func (m Model) gfxPlaceholder(url string, cols, rows int) string {
	g := &m.rt.gfx
	id := len(g.slots)
	g.slots = append(g.slots, gfxSlot{url, cols, rows})
	ground := lipgloss.NewStyle().Background(theme.Ground)
	blank := ground.Render(strings.Repeat(" ", cols-1))
	lines := make([]string, rows)
	for y := range lines {
		mark := ground.Render(" ")
		switch {
		case y == 0:
			mark = lipgloss.NewStyle().Background(lipgloss.Color(sentinel(id, 0))).Render(" ")
		case y == rows-1:
			mark = lipgloss.NewStyle().Background(lipgloss.Color(sentinel(id, 1))).Render(" ")
		}
		lines[y] = mark + blank
	}
	return strings.Join(lines, "\n")
}

// sentinel, zemin renginin son bitleriyle oynanmış, gözle fark edilmeyen bir
// renk: hangi alanın (id) hangi köşesi (which: 0 üst, 1 alt).
func sentinel(id, which int) string {
	r, g, b, _ := theme.Ground.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, (b>>8)^uint32(1+id*2+which))
}

// sentinelSGR, işaret renginin çizilmiş metindeki karşılığı.
func sentinelSGR(id, which int) string {
	var r, g, b int
	_, _ = fmt.Sscanf(sentinel(id, which), "#%02x%02x%02x", &r, &g, &b)
	return fmt.Sprintf("48;2;%d;%d;%d", r, g, b)
}

// findPlaces, çizilmiş ekranda işaretleri arayıp resimlerin yerini bulur.
// İki köşesi de beklenen yerde görünmeyen alan (kırpılmışsa) atlanır.
func findPlaces(screen string, slots []gfxSlot) []gfxPlace {
	if len(slots) == 0 {
		return nil
	}
	type pos struct{ x, y int }
	found := map[string]pos{}
	for y, line := range strings.Split(screen, "\n") {
		for id := range slots {
			for which := 0; which < 2; which++ {
				sgr := sentinelSGR(id, which)
				i := strings.Index(line, sgr)
				if i < 0 {
					continue
				}
				start := strings.LastIndex(line[:i], "\x1b[")
				if start < 0 {
					continue
				}
				found[fmt.Sprint(id, which)] = pos{ansi.StringWidth(line[:start]), y}
			}
		}
	}
	var out []gfxPlace
	for id, s := range slots {
		top, ok1 := found[fmt.Sprint(id, 0)]
		bottom, ok2 := found[fmt.Sprint(id, 1)]
		if !ok1 || (s.rows > 1 && (!ok2 || bottom.x != top.x || bottom.y-top.y != s.rows-1)) {
			continue
		}
		out = append(out, gfxPlace{s, top.x, top.y})
	}
	return out
}
