// sotto: terminal için sade ama kapsamlı bir Spotify arayüzü.
package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/config"
	"github.com/Ehtisam/sotto/internal/i18n"
	"github.com/Ehtisam/sotto/internal/ui"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "sotto: could not read settings:", err)
		os.Exit(1)
	}

	i18n.Set(cfg.Language)

	out := ui.NewOutput(os.Stdout)
	m := ui.New(cfg).WithOutput(out)
	if len(os.Args) > 1 && os.Args[1] == "setup" { // kurulumu yeniden aç
		m = m.StartSetup()
	}
	// Soloist sotto açıkken çalışır; nasıl çıkılırsa çıkılsın kapatılır.
	defer m.Shutdown()

	if _, err := tea.NewProgram(m, tea.WithOutput(out)).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "sotto:", err)
		m.Shutdown()
		os.Exit(1)
	}
}
