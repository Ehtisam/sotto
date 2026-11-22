//go:build !windows

package config

import "os"

// systemLocale: Unix'te dil ortam değişkenlerinden (öncelik sırasıyla).
func systemLocale() string {
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(k); v != "" && v != "C" && v != "POSIX" {
			return v
		}
	}
	return ""
}
