package ui

// Sabitlenmiş öğeler (✱): Spotify uygulamalarında Kitaplık'ın üstüne
// iğnelenenler sotto'da da üstte, ✦ işaretiyle görünür (sıralamadan bağımsız,
// en son sabitlenen önce). Kitaplık'ın kökünde klasör içindekiler de üste
// çıkar (Spotify'daki gibi). Sabitleme Spotify'ın uygulamalarında yapılır:
// bu küme dışarıdan yazılamıyor.

// pinKey, girişin sabit kümesindeki adresi (Beğenilenler "spotify:collection").
func pinKey(e libEntry) string {
	if e.kind == kindLiked {
		return "spotify:collection"
	}
	return e.uri
}

// withPins, sabitlenenleri başa alır ve işaretler. Sayfa içi arama açıkken
// sıra aramanın olduğundan dokunulmaz (yalnız işaret).
func (m Model) withPins(vis []libEntry) []libEntry {
	if len(m.lib.pins) == 0 || !m.cfg.Extras.Enabled {
		return vis
	}
	rank := make(map[string]int, len(m.lib.pins))
	for i, u := range m.lib.pins {
		rank[u] = i
	}
	var pinned, rest []libEntry
	seen := map[string]bool{}
	for _, e := range vis {
		if _, ok := rank[pinKey(e)]; ok {
			e.pinned = true
			pinned = append(pinned, e)
			seen[pinKey(e)] = true
		} else {
			rest = append(rest, e)
		}
	}
	if m.lib.find.query != "" {
		out := make([]libEntry, 0, len(vis))
		for _, e := range vis {
			_, e.pinned = rank[pinKey(e)]
			out = append(out, e)
		}
		return out
	}
	// Kökte (hepsi süzgeci), klasörlerin içindeki sabitler de üste.
	if m.lib.filter == libAll && m.lib.folder == "" {
		for _, e := range append(append([]libEntry(nil), m.lib.entries...), m.lib.saved...) {
			if _, ok := rank[pinKey(e)]; ok && !seen[pinKey(e)] {
				e.pinned = true
				pinned = append(pinned, e)
				seen[pinKey(e)] = true
			}
		}
	}
	// Sabitlenme sırası (en son önce).
	for i := 1; i < len(pinned); i++ {
		for j := i; j > 0 && rank[pinKey(pinned[j])] < rank[pinKey(pinned[j-1])]; j-- {
			pinned[j], pinned[j-1] = pinned[j-1], pinned[j]
		}
	}
	return append(pinned, rest...)
}
