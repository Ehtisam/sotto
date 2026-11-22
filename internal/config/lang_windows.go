//go:build windows

package config

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var getUserDefaultLocaleName = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetUserDefaultLocaleName")

// systemLocale: kullanıcının Windows yerel ayarı ("tr-TR" gibi).
func systemLocale() string {
	buf := make([]uint16, 85) // LOCALE_NAME_MAX_LENGTH
	if r, _, _ := getUserDefaultLocaleName.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))); r == 0 {
		return ""
	}
	return windows.UTF16ToString(buf)
}
