// sotto: terminal için sade ama kapsamlı bir Spotify arayüzü.
package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/config"
	"github.com/Ehtisam/sotto/internal/ui"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "sotto: ayarlar okunamadı:", err)
		os.Exit(1)
	}

	m := ui.New(cfg)
	// Soloist sotto açıkken çalışır; nasıl çıkılırsa çıkılsın kapatılır.
	defer m.Shutdown()

	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "sotto:", err)
		m.Shutdown()
		os.Exit(1)
	}
}
