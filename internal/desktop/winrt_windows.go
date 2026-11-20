//go:build windows

package desktop

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// WinRT'ye cgo'suz, doğrudan COM vtable çağrılarıyla erişilir. Bütün WinRT
// işleri tek bir iş parçacığında yapılır: bu iş parçacığı Windows'un medya
// denetiminin istediği gizli pencereyi tutar ve mesaj döngüsünü çevirir.

var (
	combase                    = windows.NewLazySystemDLL("combase.dll")
	procRoInitialize           = combase.NewProc("RoInitialize")
	procRoGetActivationFactory = combase.NewProc("RoGetActivationFactory")
	procRoActivateInstance     = combase.NewProc("RoActivateInstance")
	procWindowsCreateString    = combase.NewProc("WindowsCreateString")
	procWindowsDeleteString    = combase.NewProc("WindowsDeleteString")
	user32                     = windows.NewLazySystemDLL("user32.dll")
	procCreateWindowExW        = user32.NewProc("CreateWindowExW")
	procGetMessageW            = user32.NewProc("GetMessageW")
	procTranslateMessage       = user32.NewProc("TranslateMessage")
	procDispatchMessageW       = user32.NewProc("DispatchMessageW")
	procPostMessageW           = user32.NewProc("PostMessageW")
	shell32                    = windows.NewLazySystemDLL("shell32.dll")
	procSetCurrentProcessAppID = shell32.NewProc("SetCurrentProcessExplicitAppUserModelID")
)

func guid(s string) windows.GUID {
	g, err := windows.GUIDFromString("{" + s + "}")
	if err != nil {
		panic(err)
	}
	return g
}

var (
	iidIUnknown    = guid("00000000-0000-0000-C000-000000000046")
	iidIAgile      = guid("94ea2b94-e9cc-49e0-c0ff-ee64ca8f5b90")
	iidXMLDoc      = guid("f7f3a506-1e87-42d6-bcfb-b8c809fa5494")
	iidXMLDocIO    = guid("6cd0e74e-ee65-4489-9ebf-ca43e87ba637")
	iidToastFac    = guid("04124b20-82c6-4229-b109-fd9ed4662b53")
	iidToast2      = guid("9dfb9fd1-143a-490e-90bf-b9fba7132de7")
	iidToastMgr    = guid("50ac103f-d235-4598-bbef-98fe4d1a3ad4")
	iidSMTCInterop = guid("ddb0472d-c911-4a1f-86d9-dc3d71a95f5a")
	iidSMTC        = guid("99fa3ff4-1742-42a6-902e-087d41f965ec")
	iidSMTC2       = guid("ea98d2f6-7f3c-4af2-a586-72889808efb1")
	iidMusic       = guid("6bbf0c59-d0a0-4d26-92a0-f978e1d18e7b")
	iidMusic2      = guid("00368462-97d3-44b9-b00f-008afcefaf18")
	iidTimeline    = guid("5125316a-c3a2-475b-8507-93534dc88f15")
	iidUriFac      = guid("44a9796f-723e-4fdf-a218-033e75b0c084")
	iidStreamRef   = guid("857309dc-3fbf-4e7d-986f-ef3b1a07a964")
	iidButtonArgs  = guid("b7f47116-a56f-4dc8-9e11-92031f4a87c2")
	iidSeekArgs    = guid("b4493f88-eb28-4961-9c14-335e44f3e125")
	// TypedEventHandler<SystemMediaTransportControls, …> temsilcileri.
	iidButtonHandler = guid("0557e996-7b23-5bae-aa81-ea0d671143a4")
	iidSeekHandler   = guid("44e34f15-bdc0-50a7-ace4-39e91fb753f1")
)

// ---- HRESULT, HSTRING, COM nesnesi ----

type hresultError uint32

func (e hresultError) Error() string { return fmt.Sprintf("winrt: HRESULT 0x%08x", uint32(e)) }

func check(r uintptr) error {
	if uint32(r)&0x80000000 != 0 {
		return hresultError(uint32(r))
	}
	return nil
}

// hstring, WinRT dizgisi; 0 boş dizgidir.
type hstring uintptr

func newHString(s string) (hstring, error) {
	if s == "" {
		return 0, nil
	}
	u, err := windows.UTF16FromString(s)
	if err != nil {
		return 0, err
	}
	var h hstring
	r, _, _ := procWindowsCreateString.Call(uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&h)))
	return h, check(r)
}

func (h hstring) free() {
	if h != 0 {
		procWindowsDeleteString.Call(uintptr(h))
	}
}

// com, bir COM arayüz işaretçisi (Windows belleğinde).
type com uintptr

// call, vtable'daki i'inci yöntemi çağırır. IInspectable'da ilk altı yöntem
// (QueryInterface … GetTrustLevel) ortaktır; arayüzün kendi yöntemleri 6'dan başlar.
func (c com) call(i int, args ...uintptr) error {
	if c == 0 {
		return errors.New("winrt: nil object")
	}
	vt := *(*uintptr)(unsafe.Pointer(c))
	fn := *(*uintptr)(unsafe.Pointer(vt + uintptr(i)*unsafe.Sizeof(uintptr(0))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{uintptr(c)}, args...)...)
	return check(r)
}

func (c com) release() {
	if c != 0 {
		_ = c.call(2)
	}
}

func (c com) query(iid *windows.GUID) (com, error) {
	var out com
	err := c.call(0, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out)))
	return out, err
}

// getObj, "get_X(out T)" biçimindeki bir yöntemi çağırır.
func (c com) getObj(i int) (com, error) {
	var out com
	err := c.call(i, uintptr(unsafe.Pointer(&out)))
	return out, err
}

// putString, "put_X(HSTRING)" biçimindeki bir yöntemi çağırır.
func (c com) putString(i int, s string) error {
	h, err := newHString(s)
	if err != nil {
		return err
	}
	defer h.free()
	return c.call(i, uintptr(h))
}

func factory(class string, iid *windows.GUID) (com, error) {
	h, err := newHString(class)
	if err != nil {
		return 0, err
	}
	defer h.free()
	var out com
	r, _, _ := procRoGetActivationFactory.Call(uintptr(h), uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out)))
	return out, check(r)
}

func activate(class string) (com, error) {
	h, err := newHString(class)
	if err != nil {
		return 0, err
	}
	defer h.free()
	var out com
	r, _, _ := procRoActivateInstance.Call(uintptr(h), uintptr(unsafe.Pointer(&out)))
	return out, check(r)
}

// ---- olay temsilcisi (Go'da yazılmış COM nesnesi) ----

type delegateVtbl struct {
	QueryInterface, AddRef, Release, Invoke uintptr
}

type delegate struct {
	vtbl *delegateVtbl
	refs int32
	iid  windows.GUID
	fn   func(sender, args com)
}

var (
	delegateVT = &delegateVtbl{
		QueryInterface: syscall.NewCallback(func(this, riid, ppv uintptr) uintptr {
			d := (*delegate)(unsafe.Pointer(this))
			id := *(*windows.GUID)(unsafe.Pointer(riid))
			if id == d.iid || id == iidIUnknown || id == iidIAgile {
				*(*uintptr)(unsafe.Pointer(ppv)) = this
				atomic.AddInt32(&d.refs, 1)
				return 0
			}
			*(*uintptr)(unsafe.Pointer(ppv)) = 0
			return 0x80004002 // E_NOINTERFACE
		}),
		AddRef: syscall.NewCallback(func(this uintptr) uintptr {
			return uintptr(atomic.AddInt32(&(*delegate)(unsafe.Pointer(this)).refs, 1))
		}),
		Release: syscall.NewCallback(func(this uintptr) uintptr {
			// Nesne Go belleğinde; keep listesi yaşattığı için serbest bırakılmaz.
			return uintptr(atomic.AddInt32(&(*delegate)(unsafe.Pointer(this)).refs, -1))
		}),
		Invoke: syscall.NewCallback(func(this, sender, args uintptr) uintptr {
			d := (*delegate)(unsafe.Pointer(this))
			if d.fn != nil {
				d.fn(com(sender), com(args))
			}
			return 0
		}),
	}
	keepMu sync.Mutex
	keep   []*delegate // Windows'a verilen temsilciler çöp toplayıcıdan korunur
)

func newDelegate(iid windows.GUID, fn func(sender, args com)) *delegate {
	d := &delegate{vtbl: delegateVT, refs: 1, iid: iid, fn: fn}
	keepMu.Lock()
	keep = append(keep, d)
	keepMu.Unlock()
	return d
}

// ---- WinRT iş parçacığı ----

const wmJob = 0x8000 + 1 // WM_APP + 1

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
	private uint32
}

type worker struct {
	hwnd uintptr
	jobs chan func()
}

var (
	wkOnce sync.Once
	wk     *worker
	wkErr  error
)

// getWorker, WinRT iş parçacığını (ilk çağrıda) başlatır.
func getWorker() (*worker, error) {
	wkOnce.Do(func() {
		ready := make(chan error, 1)
		w := &worker{jobs: make(chan func(), 64)}
		go w.loop(ready)
		if wkErr = <-ready; wkErr == nil {
			wk = w
		}
	})
	return wk, wkErr
}

func (w *worker) loop(ready chan<- error) {
	runtime.LockOSThread() // pencere ve STA bu iş parçacığına bağlı; bırakılmaz

	if r, _, _ := procRoInitialize.Call(0); check(r) != nil && uint32(r) != 0x80010106 { // RPC_E_CHANGED_MODE
		ready <- check(r)
		return
	}
	if p, err := windows.UTF16PtrFromString(AppID); err == nil {
		procSetCurrentProcessAppID.Call(uintptr(unsafe.Pointer(p)))
	}
	cls, _ := windows.UTF16PtrFromString("STATIC")
	title, _ := windows.UTF16PtrFromString("sotto")
	hwnd, _, err := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(title)),
		0, 0, 0, 0, 0, 0, 0, 0, 0)
	if hwnd == 0 {
		ready <- fmt.Errorf("winrt: window: %w", err)
		return
	}
	w.hwnd = hwnd
	ready <- nil

	var m msg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		if m.message == wmJob {
			w.drain()
			continue
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func (w *worker) drain() {
	for {
		select {
		case f := <-w.jobs:
			f()
		default:
			return
		}
	}
}

// do, f'yi WinRT iş parçacığında çalıştırır ve bitmesini bekler.
func (w *worker) do(f func() error) error {
	done := make(chan error, 1)
	job := func() {
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("winrt: %v", r)
			}
		}()
		done <- f()
	}
	select {
	case w.jobs <- job:
	case <-time.After(5 * time.Second):
		return errors.New("winrt: busy")
	}
	procPostMessageW.Call(w.hwnd, wmJob, 0, 0)
	select {
	case err := <-done:
		return err
	case <-time.After(15 * time.Second):
		return errors.New("winrt: timed out")
	}
}
