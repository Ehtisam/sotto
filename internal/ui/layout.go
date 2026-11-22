package ui

// Yerleşim: ekranın parçalarının yeri tek yerde hesaplanır; çizim (render,
// body), fare, liste yükseklikleri ve içerik genişliği buradan okur.
//
// Sade düzen:   başlık · çizgi · gövde · çizgi · şerit (2) · boş · kısayollar
// Kutulu düzen: başlık · [menü kutusu | içerik kutusu] · şerit kutusu · kısayollar
// Şerit üstte seçiliyse şerit başlığın hemen altına geçer. Kutulu düzende
// her kutunun içinde iki yanda bir sütun boşluk var; gövde bir satır kısalır.

type layout struct {
	boxed          bool
	bodyTop, bodyH int // gövdenin (kutu içinin) ilk satırı ve yüksekliği
	stripY         int // şeridin ilk satırı (ikincisi ilerleme çubuğu)
	stripX         int // şerit yazısının başladığı sütun
	menu           bool
	menuRight      bool
	menuX          int
	contentX       int
	contentW       int
	panel          bool // yan panel (panel.go): içeriğin sağında
	panelX         int
}

func (m Model) layout() layout {
	l := layout{boxed: m.cfg.Look.Layout == "boxed", stripX: margin}
	if l.boxed {
		l.bodyH = max(m.height-8, 1)
		l.bodyTop, l.stripY = 2, 2+l.bodyH+1+2
		if m.stripTop() {
			l.stripY, l.bodyTop = 2, 6
		}
		l.stripX = margin + 1
	} else {
		l.bodyH = max(m.height-7, 1)
		l.bodyTop, l.stripY = 2, 2+l.bodyH+1
		if m.stripTop() {
			l.stripY, l.bodyTop = 2, 5
		}
	}
	cw := m.width - margin - menuWidth - gutter - 1
	// Tam ekran görselleştirme (V) menünün yerini de alır.
	full := m.vizFull && !m.help && !m.pick.open
	l.menu = cw >= 24 && m.cfg.Look.Menu != "hidden" && !full
	right := m.cfg.Look.Menu == "right"
	if l.boxed {
		// Kutu: kenar + boşluk + içerik + boşluk + kenar; kutular arası bir sütun.
		const menuBox = menuWidth + 4
		l.contentX, l.contentW = 2, max(m.width-4, 1)
		if l.menu {
			l.contentW = max(m.width-menuBox-1-4, 1)
			if right {
				l.menuRight = true
				l.menuX = 2 + l.contentW + 2 + 1 + 2
			} else {
				l.menuX = 2
				l.contentX = menuBox + 1 + 2
			}
		}
		// Yan panel kendi kutusunda, içerik kutusunun sağında.
		const panelBox = panelW + 4 + 1
		if m.panelWanted() && l.contentW-panelBox >= panelMinContent {
			l.panel = true
			l.contentW -= panelBox
			l.panelX = l.contentX + l.contentW + 2 + 1 + 2
			if l.menuRight {
				l.menuX = l.panelX + panelW + 2 + 1 + 2
			}
		}
		return l
	}
	l.contentX, l.contentW = margin, max(m.width-margin-1, 1)
	if l.menu {
		l.contentW = cw
		if right {
			l.menuRight = true
			l.menuX = margin + cw + gutter
		} else {
			l.menuX = margin
			l.contentX = margin + menuWidth + gutter
		}
	}
	if m.panelWanted() && l.contentW-panelW-gutter >= panelMinContent {
		l.panel = true
		l.contentW -= panelW + gutter
		l.panelX = l.contentX + l.contentW + gutter
		if l.menuRight {
			l.menuX = l.panelX + panelW + gutter
		}
	}
	return l
}
