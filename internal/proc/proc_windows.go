//go:build windows

package proc

import (
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Detach, programı ayrı bir süreç grubunda ve pencere açmadan başlatır.
// Böylece terminalde basılan Ctrl+C doğrudan programa gitmez; sotto onu
// kendisi kapatır.
func Detach(cmd *exec.Cmd) {
	const createNoWindow = 0x08000000
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | createNoWindow,
		HideWindow:    true,
	}
}

var (
	jobOnce sync.Once
	job     windows.Handle
	jobErr  error
)

// BindToParent, programı "sotto kapanınca öldür" ayarlı bir Job Object'e
// koyar. sotto nasıl kapanırsa kapansın (pencere kapatma, çökme) program
// arka planda kalmaz.
func BindToParent(pid int) error {
	jobOnce.Do(func() {
		h, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			jobErr = err
			return
		}
		info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
			BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
				LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
			},
		}
		if _, err := windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
			windows.CloseHandle(h)
			jobErr = err
			return
		}
		job = h // sotto'nun ömrü boyunca açık kalır; kapanınca program da kapanır
	})
	if jobErr != nil {
		return jobErr
	}
	ph, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(ph)
	return windows.AssignProcessToJobObject(job, ph)
}
