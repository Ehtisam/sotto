//go:build windows

package desktop

import (
	"errors"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	clipKernel32     = windows.NewLazySystemDLL("kernel32.dll")
	openClipboard    = user32.NewProc("OpenClipboard")
	closeClipboard   = user32.NewProc("CloseClipboard")
	getClipboardData = user32.NewProc("GetClipboardData")
	isFormatAvail    = user32.NewProc("IsClipboardFormatAvailable")
	globalLock       = clipKernel32.NewProc("GlobalLock")
	globalUnlock     = clipKernel32.NewProc("GlobalUnlock")
)

const cfUnicodeText = 13

// ReadClipboard, panodaki metni okur. Pano başka bir programda açıksa kısa
// bir süre yeniden dener.
func ReadClipboard() (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if r, _, _ := isFormatAvail.Call(cfUnicodeText); r == 0 {
		return "", nil
	}
	opened := false
	for try := 0; try < 10; try++ {
		if r, _, _ := openClipboard.Call(0); r != 0 {
			opened = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !opened {
		return "", errors.New("the clipboard is busy")
	}
	defer closeClipboard.Call()
	h, _, _ := getClipboardData.Call(cfUnicodeText)
	if h == 0 {
		return "", nil
	}
	p, _, _ := globalLock.Call(h)
	if p == 0 {
		return "", errors.New("could not read the clipboard")
	}
	defer globalUnlock.Call(h)
	return windows.UTF16PtrToString((*uint16)(unsafe.Pointer(p))), nil
}
