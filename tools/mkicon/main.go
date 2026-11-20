// mkicon, sotto simgesini assets/ altına PNG ve ICO olarak, Windows için de
// exe'ye gömülen kaynak dosyalarını (cmd/sotto/rsrc_windows_*.syso: simge ve
// "sotto" adı) yazar. Simge internal/desktop/icon.go'da çizilir; değişince:
//
//	go run ./tools/mkicon
package main

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/Ehtisam/sotto/internal/desktop"
	"github.com/Ehtisam/sotto/internal/winres"
)

// version, exe'nin sürüm bilgisine yazılır.
const version = "0.1.0"

func main() {
	write := func(name string, data []byte) {
		if err := os.WriteFile(name, data, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("wrote", name)
	}
	write("assets/sotto.png", desktop.IconPNG(256))
	ico := desktop.IconICO()
	write("assets/sotto.ico", ico)

	// Windows simgeyi uygulama adına göre önbelleğe aldığı için ad simgeyle değişir.
	// Bildirim simgesi (IconUri PNG'si) ve kısayol simgesi (ICO) birlikte.
	sum := sha1.Sum(append(append([]byte{}, ico...), desktop.NotifyIconPNG()...))
	write("internal/desktop/appid_gen.go", []byte(`// go run ./tools/mkicon yazar; elle değiştirme.

package desktop

// AppID, simgenin parmak iziyle (bkz. desktop.go).
const AppID = "Ehtisam.sotto.`+hex.EncodeToString(sum[:4])+`"
`))

	info := winres.Info{Name: "sotto", Description: "sotto", Version: version, Filename: "sotto.exe"}
	for _, n := range desktop.IconSizes {
		info.Icons = append(info.Icons, desktop.IconPNG(n))
		info.IconSizes = append(info.IconSizes, n)
	}
	for _, arch := range []winres.Arch{winres.AMD64, winres.ARM64} {
		b, err := winres.Syso(arch, info)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		write("cmd/sotto/rsrc_windows_"+string(arch)+".syso", b)
	}
}
