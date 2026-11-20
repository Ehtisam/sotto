//go:build windows

package desktop

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Başlat menüsü kısayolu. Windows medya kartında ve bildirimlerde uygulamanın
// adını ve simgesini Başlat menüsündeki kısayoldan okur: kısayolun
// System.AppUserModel.ID özelliği sotto'nun kimliğiyle (AppID) aynı olmalı.
// Kısayol yoksa kart "Bilinmeyen uygulama" der. Kısayol yalnızca bu kullanıcı
// için, yönetici izni gerekmeden %APPDATA%\Microsoft\Windows\Start Menu\Programs
// altına yazılır.

var (
	ole32                = windows.NewLazySystemDLL("ole32.dll")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")
	procPropVariantClear = ole32.NewProc("PropVariantClear")
)

var (
	clsidShellLink    = guid("00021401-0000-0000-C000-000000000046")
	iidShellLinkW     = guid("000214F9-0000-0000-C000-000000000046")
	iidPersistFile    = guid("0000010b-0000-0000-C000-000000000046")
	iidPropertyStore  = guid("886d8eeb-8cf2-4446-8d02-cdba1dbdcf99")
	fmtidAppUserModel = guid("9F4C2855-9F79-4B39-A8D0-E1D42DE1D5F3")
)

type propertyKey struct {
	fmtid windows.GUID
	pid   uint32
}

var pkeyAppUserModelID = propertyKey{fmtidAppUserModel, 5}

// propVariant, PROPVARIANT'ın VT_LPWSTR için gereken kısmı (amd64/arm64'te 24 bayt).
type propVariant struct {
	vt  uint16
	_   [3]uint16
	ptr uintptr
	_   uintptr
}

const vtLPWSTR = 31

// ShortcutPath, sotto'nun Başlat menüsü kısayolunun yeri.
func ShortcutPath() (string, error) {
	dir, err := windows.KnownFolderPath(windows.FOLDERID_Programs, 0)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sotto.lnk"), nil
}

func newShellLink() (com, error) {
	var link com
	r, _, _ := procCoCreateInstance.Call(uintptrOf(&clsidShellLink), 0, 1 /* CLSCTX_INPROC_SERVER */, uintptrOf(&iidShellLinkW), uintptrOf(&link))
	return link, check(r)
}

func wstr(s string) uintptr {
	p, _ := windows.UTF16PtrFromString(s)
	return uintptr(unsafe.Pointer(p))
}

// InstallShortcut, Başlat menüsüne exe'yi açan, sotto'nun kimliğini taşıyan
// kısayolu yazar (varsa yeniler).
func InstallShortcut(exe string) error {
	path, err := ShortcutPath()
	if err != nil {
		return err
	}
	return installShortcutAs(path, exe, AppID)
}

// installShortcutAs, kısayolu verilen yere verilen kimlikle yazar.
func installShortcutAs(path, exe, appID string) error {
	icon := ""
	if iconDir != "" {
		icon = filepath.Join(iconDir, "sotto.ico")
		if _, err := writeICO(icon); err != nil {
			icon = ""
		}
	}
	w, err := getWorker()
	if err != nil {
		return err
	}
	return w.do(func() error {
		link, err := newShellLink()
		if err != nil {
			return err
		}
		defer link.release()
		home, _ := os.UserHomeDir()
		_ = link.call(20, wstr(exe))                     // SetPath
		if err := link.call(9, wstr(home)); err != nil { // SetWorkingDirectory
			return err
		}
		_ = link.call(7, wstr("A Spotify player for the terminal")) // SetDescription
		if icon != "" {
			_ = link.call(17, wstr(icon), 0) // SetIconLocation
		}

		ps, err := link.query(&iidPropertyStore)
		if err != nil {
			return err
		}
		defer ps.release()
		id, _ := windows.UTF16PtrFromString(appID)
		pv := propVariant{vt: vtLPWSTR, ptr: uintptr(unsafe.Pointer(id))}
		if err := ps.call(6, uintptrOf(&pkeyAppUserModelID), uintptrOf(&pv)); err != nil { // SetValue
			return err
		}
		if err := ps.call(7); err != nil { // Commit
			return err
		}

		pf, err := link.query(&iidPersistFile)
		if err != nil {
			return err
		}
		defer pf.release()
		_ = os.MkdirAll(filepath.Dir(path), 0o700)
		return pf.call(6, wstr(path), 1) // Save
	})
}

// RemoveShortcut, kısayolu siler (yoksa bir şey yapmaz).
func RemoveShortcut() error {
	path, err := ShortcutPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// ShortcutInfo, kısayolun hedefi ve kimliği ("" : kısayol yok).
func ShortcutInfo() (target, appID string, err error) {
	path, err := ShortcutPath()
	if err != nil {
		return "", "", err
	}
	if _, err := os.Stat(path); err != nil {
		return "", "", nil
	}
	w, err := getWorker()
	if err != nil {
		return "", "", err
	}
	err = w.do(func() error {
		link, err := newShellLink()
		if err != nil {
			return err
		}
		defer link.release()
		pf, err := link.query(&iidPersistFile)
		if err != nil {
			return err
		}
		defer pf.release()
		if err := pf.call(5, wstr(path), 0); err != nil { // Load
			return err
		}
		buf := make([]uint16, windows.MAX_PATH*2)
		if err := link.call(3, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0, 0x4 /* SLGP_RAWPATH */); err == nil { // GetPath
			target = windows.UTF16ToString(buf)
		}
		ps, err := link.query(&iidPropertyStore)
		if err != nil {
			return err
		}
		defer ps.release()
		var pv propVariant
		if err := ps.call(5, uintptrOf(&pkeyAppUserModelID), uintptrOf(&pv)); err != nil { // GetValue
			return err
		}
		if pv.vt == vtLPWSTR && pv.ptr != 0 {
			appID = windows.UTF16PtrToString((*uint16)(unsafe.Pointer(pv.ptr)))
		}
		procPropVariantClear.Call(uintptrOf(&pv))
		return nil
	})
	return target, appID, err
}

// ShortcutOK: kısayol var, bu exe'yi açıyor ve sotto'nun kimliğini taşıyor.
func ShortcutOK(exe string) bool {
	target, id, err := ShortcutInfo()
	return err == nil && id == AppID && strings.EqualFold(filepath.Clean(target), filepath.Clean(exe))
}

// CleanupOld, eski uygulama kimliklerinin kayıtlarını ve deneme kısayolunu siler.
func CleanupOld() {
	for _, parent := range []string{`Software\Classes\AppUserModelId`, `Software\Microsoft\Windows\CurrentVersion\Notifications\Settings`} {
		k, err := registry.OpenKey(registry.CURRENT_USER, parent, registry.ENUMERATE_SUB_KEYS)
		if err != nil {
			continue
		}
		names, _ := k.ReadSubKeyNames(-1)
		k.Close()
		for _, n := range names {
			if strings.HasPrefix(strings.ToLower(n), strings.ToLower(appIDPrefix)) && !strings.EqualFold(n, AppID) {
				_ = registry.DeleteKey(registry.CURRENT_USER, parent+`\`+n)
			}
		}
	}
	if p, err := ShortcutPath(); err == nil {
		_ = os.Remove(filepath.Join(filepath.Dir(p), "sotto v2 deneme.lnk"))
	}
}
