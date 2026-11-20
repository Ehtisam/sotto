//go:build windows

package audio

import (
	"encoding/binary"
	"fmt"
	"math"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// WASAPI loopback, cgo'suz COM vtable çağrılarıyla: varsayılan hoparlörün
// sesi paylaşımlı modda dinlenir (başka programları etkilemez).

var (
	ole32              = windows.NewLazySystemDLL("ole32.dll")
	procCoInitializeEx = ole32.NewProc("CoInitializeEx")
	procCoUninitialize = ole32.NewProc("CoUninitialize")
	procCoCreateInst   = ole32.NewProc("CoCreateInstance")
	procCoTaskMemFree  = ole32.NewProc("CoTaskMemFree")
)

func guid(s string) windows.GUID {
	g, err := windows.GUIDFromString("{" + s + "}")
	if err != nil {
		panic(err)
	}
	return g
}

var (
	clsidMMDeviceEnumerator = guid("BCDE0395-E52F-467C-8E3D-C4579291692E")
	iidIMMDeviceEnumerator  = guid("A95664D2-9614-4F35-A746-DE8DB63617E6")
	iidIAudioClient         = guid("1CB9AD4C-DBFA-4c32-B178-C2F568A703B2")
	iidIAudioCaptureClient  = guid("C8ADBD64-E71E-48a0-A4DE-185C395CD317")
	subtypeFloat            = guid("00000003-0000-0010-8000-00aa00389b71")
	subtypePCM              = guid("00000001-0000-0010-8000-00aa00389b71")
)

const (
	clsctxAll              = 0x17
	eRender                = 0
	eConsole               = 0
	sharedMode             = 0
	streamFlagsLoopback    = 0x00020000
	bufferFlagsSilent      = 0x2
	waveFormatPCM          = 1
	waveFormatIEEEFloat    = 3
	waveFormatExtensible   = 0xFFFE
	coinitMultithreaded    = 0x0
	rpcEChangedMode        = 0x80010106
	sFalse                 = 1
	bufferDuration100ns    = 2000000 // 200 ms
	pollEvery              = 15 * time.Millisecond
	hresultDeviceInvalided = 0x88890004
)

type hresult uint32

func (h hresult) Error() string {
	if h == hresultDeviceInvalided {
		return "audio device changed"
	}
	return fmt.Sprintf("wasapi: HRESULT 0x%08x", uint32(h))
}

func check(r uintptr) error {
	if uint32(r)&0x80000000 != 0 {
		return hresult(uint32(r))
	}
	return nil
}

type com uintptr

func (c com) call(i int, args ...uintptr) error {
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

// waveFormat, WAVEFORMATEX (+ WAVEFORMATEXTENSIBLE).
type waveFormat struct {
	Tag           uint16
	Channels      uint16
	SamplesPerSec uint32
	AvgBytes      uint32
	BlockAlign    uint16
	Bits          uint16
	Size          uint16
	ValidBits     uint16
	ChannelMask   uint32
	SubFormat     windows.GUID
}

// capture, bir yakalama oturumu: hazır olunca ready(nil), kurulamazsa
// ready(hata) çağırır; durdurulana ya da aygıt değişene kadar döner.
func capture(v *Visualizer, ready func(error)) (err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	fail := func(e error) error { ready(e); return e }

	r, _, _ := procCoInitializeEx.Call(0, coinitMultithreaded)
	if r != 0 && r != sFalse && uint32(r) != rpcEChangedMode {
		return fail(hresult(uint32(r)))
	}
	if r == 0 || r == sFalse {
		defer procCoUninitialize.Call()
	}

	var enum com
	r, _, _ = procCoCreateInst.Call(uintptr(unsafe.Pointer(&clsidMMDeviceEnumerator)), 0, clsctxAll,
		uintptr(unsafe.Pointer(&iidIMMDeviceEnumerator)), uintptr(unsafe.Pointer(&enum)))
	if e := check(r); e != nil {
		return fail(e)
	}
	defer enum.release()

	var dev com
	if e := enum.call(4, eRender, eConsole, uintptr(unsafe.Pointer(&dev))); e != nil { // GetDefaultAudioEndpoint
		return fail(e)
	}
	defer dev.release()

	var client com
	if e := dev.call(3, uintptr(unsafe.Pointer(&iidIAudioClient)), clsctxAll, 0, uintptr(unsafe.Pointer(&client))); e != nil { // Activate
		return fail(e)
	}
	defer client.release()

	var pf uintptr
	if e := client.call(8, uintptr(unsafe.Pointer(&pf))); e != nil { // GetMixFormat
		return fail(e)
	}
	defer procCoTaskMemFree.Call(pf)
	wf := *(*waveFormat)(unsafe.Pointer(pf))
	isFloat := wf.Tag == waveFormatIEEEFloat || (wf.Tag == waveFormatExtensible && wf.SubFormat == subtypeFloat)
	isPCM := wf.Tag == waveFormatPCM || (wf.Tag == waveFormatExtensible && wf.SubFormat == subtypePCM)
	if !(isFloat && wf.Bits == 32) && !(isPCM && (wf.Bits == 16 || wf.Bits == 32 || wf.Bits == 24)) {
		return fail(fmt.Errorf("unsupported mix format (tag %#x, %d bits)", wf.Tag, wf.Bits))
	}

	if e := client.call(3, sharedMode, streamFlagsLoopback, bufferDuration100ns, 0, pf, 0); e != nil { // Initialize
		return fail(e)
	}
	var cap com
	if e := client.call(14, uintptr(unsafe.Pointer(&iidIAudioCaptureClient)), uintptr(unsafe.Pointer(&cap))); e != nil { // GetService
		return fail(e)
	}
	defer cap.release()
	if e := client.call(10); e != nil { // Start
		return fail(e)
	}
	defer client.call(11) // Stop
	ready(nil)

	ch := int(wf.Channels)
	block := int(wf.BlockAlign)
	bytesPer := int(wf.Bits) / 8
	mono := make([]float64, 0, 4096)
	tick := time.NewTicker(pollEvery)
	defer tick.Stop()
	for {
		select {
		case <-v.stop:
			return nil
		case <-tick.C:
		}
		for {
			var packet uint32
			if e := cap.call(5, uintptr(unsafe.Pointer(&packet))); e != nil { // GetNextPacketSize
				return e
			}
			if packet == 0 {
				break
			}
			var data uintptr
			var frames, flags uint32
			if e := cap.call(3, uintptr(unsafe.Pointer(&data)), uintptr(unsafe.Pointer(&frames)), uintptr(unsafe.Pointer(&flags)), 0, 0); e != nil { // GetBuffer
				return e
			}
			mono = mono[:0]
			if flags&bufferFlagsSilent != 0 || data == 0 {
				for i := 0; i < int(frames); i++ {
					mono = append(mono, 0)
				}
			} else {
				raw := unsafe.Slice((*byte)(unsafe.Pointer(data)), int(frames)*block)
				for i := 0; i < int(frames); i++ {
					sum := 0.0
					for c := 0; c < ch; c++ {
						sum += sample(raw[i*block+c*bytesPer:], bytesPer, isFloat)
					}
					mono = append(mono, sum/float64(ch))
				}
			}
			_ = cap.call(4, uintptr(frames)) // ReleaseBuffer
			v.Push(mono, int(wf.SamplesPerSec))
		}
	}
}

func sample(b []byte, n int, isFloat bool) float64 {
	switch {
	case isFloat:
		return float64(math.Float32frombits(binary.LittleEndian.Uint32(b)))
	case n == 2:
		return float64(int16(binary.LittleEndian.Uint16(b))) / 32768
	case n == 3:
		v := int32(b[0]) | int32(b[1])<<8 | int32(int8(b[2]))<<16
		return float64(v) / 8388608
	default:
		return float64(int32(binary.LittleEndian.Uint32(b))) / 2147483648
	}
}
