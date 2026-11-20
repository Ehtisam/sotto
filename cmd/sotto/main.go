// sotto: terminal için sade ama kapsamlı bir Spotify arayüzü.
package main

import (
	"context"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/cli"
	"github.com/Ehtisam/sotto/internal/config"
	"github.com/Ehtisam/sotto/internal/i18n"
	"github.com/Ehtisam/sotto/internal/ipc"
	"github.com/Ehtisam/sotto/internal/ui"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "sotto: could not read settings:", err)
		os.Exit(1)
	}

	// sotto next, sotto status --json, sotto doctor …: arayüz açılmaz.
	if len(os.Args) > 1 && cli.IsCommand(os.Args[1]) {
		os.Exit(cli.Main(cfg, os.Args[1:], os.Stdout, os.Stderr, os.Stdin))
	}
	if len(os.Args) > 1 && os.Args[1] != "setup" {
		fmt.Fprintf(os.Stderr, "sotto: unknown command %q (sotto help lists them)\n", os.Args[1])
		os.Exit(2)
	}

	i18n.Set(cfg.Language)

	out := ui.NewOutput(os.Stdout)
	m := ui.New(cfg).WithOutput(out)
	if len(os.Args) > 1 && os.Args[1] == "setup" { // kurulumu yeniden aç
		m = m.StartSetup()
	}
	// Windows medya kartında ve bildirimlerde "sotto" adı için Başlat menüsü kısayolu.
	go func() { _ = ui.SyncStartMenu(cfg.Desktop.StartMenu) }()

	// Soloist sotto açıkken çalışır; nasıl çıkılırsa çıkılsın kapatılır.
	defer m.Shutdown()

	p := tea.NewProgram(m, tea.WithOutput(out))

	// Komut satırındaki sotto'lar bu pencereye bağlanır. Başka bir sotto
	// zaten dinliyorsa bu pencere dinlemez.
	srv, err := ipc.Listen(ipc.Path(cfg.LocalDir), func(ctx context.Context, req ipc.Request) ipc.Response {
		reply := make(chan ipc.Response, 1)
		p.Send(ui.IPCMsg{Req: req, Reply: reply})
		select {
		case r := <-reply:
			return r
		case <-ctx.Done():
			return ipc.Fail(ctx.Err())
		}
	})
	if err != nil {
		srv = nil
	}
	defer srv.Close()

	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "sotto:", err)
		srv.Close()
		m.Shutdown()
		os.Exit(1)
	}
}
