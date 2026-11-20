package spotify

import "time"

// Başka paketlerin testleri için: Web API adresini sahte bir sunucuya
// çevirir ve geçerli görünen bir giriş yazar.

// SetAPIBaseForTest, Web API adresini değiştirir; dönen işlev eskisine döner.
func SetAPIBaseForTest(u string) (restore func()) {
	old := apiBase
	apiBase = u
	return func() { apiBase = old }
}

// WriteTokenForTest, path'e bir saat geçerli sahte bir giriş yazar.
func WriteTokenForTest(path string) error {
	return saveToken(path, Token{AccessToken: "test", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)})
}
