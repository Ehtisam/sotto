//go:build windows

package desktop

import (
	"net/url"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sys/windows/registry"
)

// Paketlenmemiş bir programın bildirim gösterebilmesi için Windows'un onu
// tanıması gerekir: HKCU\Software\Classes\AppUserModelId\<AppID> altında ad
// ve simge. Bu kullanıcıya özel, yönetici izni istemeyen bir kayıttır.

var (
	regMu   sync.Mutex
	regDone bool
	iconDir string

	notifiers = map[string]com{} // uygulama kimliği → IToastNotifier (WinRT iş parçacığında kullanılır)
)

// SetDataDir, simgenin yazılacağı klasörü verir (%LOCALAPPDATA%\sotto).
func SetDataDir(dir string) { iconDir = dir }

const appKey = `Software\Classes\AppUserModelId\` + AppID

func register() error {
	regMu.Lock()
	defer regMu.Unlock()
	if regDone {
		return nil
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, appKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.SetStringValue("DisplayName", "sotto"); err != nil {
		return err
	}
	// Bildirim simgesi yalnızca buradan (IconUri) okunuyor; yoksa Windows genel
	// bir yer tutucu koyuyor. Medya kartı ise Başlat menüsü kısayolunun simgesini kullanıyor.
	if iconDir != "" {
		icon := filepath.Join(iconDir, "sotto.png")
		if _, err := writeIfChanged(icon, NotifyIconPNG()); err == nil {
			_ = k.SetStringValue("IconUri", icon)
		}
	}
	regDone = true
	return nil
}

// Registered, sotto'nun Windows'ta bildirim için kayıtlı olup olmadığını söyler (sotto doctor).
func Registered() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, appKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetStringValue("DisplayName")
	return err == nil && v != ""
}

// Notify, Windows bildirimi gösterir. Bildirimler hep aynı etiketi taşır:
// yenisi eskisinin yerine geçer, bildirim merkezinde yığılmaz.
func Notify(n Note) error {
	if err := register(); err != nil {
		return err
	}
	w, err := getWorker()
	if err != nil {
		return err
	}
	return w.do(func() error { return showToast(AppID, n) })
}

func showToast(appID string, n Note) error {
	doc, err := activate("Windows.Data.Xml.Dom.XmlDocument")
	if err != nil {
		return err
	}
	defer doc.release()
	io, err := doc.query(&iidXMLDocIO)
	if err != nil {
		return err
	}
	defer io.release()
	if err := io.putString(6, toastXML(n)); err != nil { // LoadXml
		return err
	}
	xdoc, err := doc.query(&iidXMLDoc)
	if err != nil {
		return err
	}
	defer xdoc.release()

	fac, err := factory("Windows.UI.Notifications.ToastNotification", &iidToastFac)
	if err != nil {
		return err
	}
	defer fac.release()
	var toast com
	if err := fac.call(6, uintptr(xdoc), uintptrOf(&toast)); err != nil { // CreateToastNotification
		return err
	}
	defer toast.release()
	if t2, err := toast.query(&iidToast2); err == nil {
		_ = t2.putString(6, "now")   // put_Tag
		_ = t2.putString(8, "sotto") // put_Group
		t2.release()
	}

	notifier := notifiers[appID]
	if notifier == 0 {
		mgr, err := factory("Windows.UI.Notifications.ToastNotificationManager", &iidToastMgr)
		if err != nil {
			return err
		}
		defer mgr.release()
		h, err := newHString(appID)
		if err != nil {
			return err
		}
		defer h.free()
		if err := mgr.call(7, uintptr(h), uintptrOf(&notifier)); err != nil { // CreateToastNotifierWithId
			return err
		}
		notifiers[appID] = notifier
	}
	return notifier.call(6, uintptr(toast)) // Show
}

func toastXML(n Note) string {
	var b strings.Builder
	b.WriteString(`<toast duration="short"><visual><binding template="ToastGeneric">`)
	b.WriteString("<text>" + xmlEscape(n.Title) + "</text>")
	if n.Body != "" {
		b.WriteString("<text>" + xmlEscape(n.Body) + "</text>")
	}
	if n.Image != "" {
		u := url.URL{Scheme: "file", Path: "/" + filepath.ToSlash(n.Image)}
		b.WriteString(`<image placement="appLogoOverride" src="` + xmlEscape(u.String()) + `"/>`)
	}
	b.WriteString(`</binding></visual><audio silent="true"/></toast>`)
	return b.String()
}

// RefreshIcons, kısayolun simgesini (sotto.ico) yeni çizimle günceller;
// değiştiyse true (o zaman kısayol yeniden yazılır).
func RefreshIcons() bool {
	if iconDir == "" {
		return false
	}
	_, _ = writeIfChanged(filepath.Join(iconDir, "sotto.png"), NotifyIconPNG())
	changed, _ := writeICO(filepath.Join(iconDir, "sotto.ico"))
	return changed
}
