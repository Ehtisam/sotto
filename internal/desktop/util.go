package desktop

import "strings"

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;").Replace(s)
}

// writeICO, Başlat menüsü kısayolunun simgesini (değiştiyse) yazar.
func writeICO(path string) (bool, error) { return writeIfChanged(path, IconICO()) }
