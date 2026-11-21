package spclient

// SetBaseForTest, iç uçların adresini testlerde sahte sunucuya çevirir.
func SetBaseForTest(u string) (restore func()) {
	old := base
	base = u
	return func() { base = old }
}
