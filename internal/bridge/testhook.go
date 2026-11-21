package bridge

import "time"

// SetTokenForTest, testlerde go-librespot'u çalıştırmadan jeton verir.
func (b *Bridge) SetTokenForTest(tok string) {
	b.mu.Lock()
	b.token, b.expiry = tok, time.Now().Add(time.Hour)
	b.mu.Unlock()
}
