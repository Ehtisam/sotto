package ui

import (
	"context"
	"os/exec"

	"github.com/Ehtisam/sotto/internal/proc"
)

// shellCommand, komutu cmd ile çalıştırır. Komut satırı olduğu gibi verilir:
// Go'nun tırnak kaçışı (\") cmd'nin anladığı biçim değil. Önce kod sayfası
// UTF-8'e alınır: yoksa cmd dosyaya eski Türkçe kod sayfasıyla (CP857) yazar,
// "ü" gibi harfler bozulur.
func shellCommand(ctx context.Context, command string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "cmd.exe")
	proc.Detach(cmd)
	// cmd kod sayfasını satırın başında okur; aynı satırdaki chcp sonrakini
	// etkilemez. Bu yüzden komut chcp'den sonra açılan ikinci bir cmd'de çalışır.
	cmd.SysProcAttr.CmdLine = `cmd.exe /S /C "chcp 65001 >nul & cmd.exe /S /C "` + command + `""`
	return cmd
}
