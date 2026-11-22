package config

import "strings"

// languageFrom, "tr_TR.UTF-8" ya da "tr-TR" gibi bir yerel ayar adından
// sotto'nun bildiği dili çıkarır; bilmediği her şey İngilizce.
func languageFrom(locale string) string {
	l := strings.ToLower(strings.TrimSpace(locale))
	if l == "tr" || strings.HasPrefix(l, "tr_") || strings.HasPrefix(l, "tr-") || strings.HasPrefix(l, "tr.") {
		return "tr"
	}
	return "en"
}

// SystemLanguage: sistemin dili (ilk açılışta varsayılan dil olarak).
func SystemLanguage() string { return languageFrom(systemLocale()) }
