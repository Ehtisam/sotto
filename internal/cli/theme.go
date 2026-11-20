package cli

import (
	"fmt"
	"io"

	"github.com/Ehtisam/sotto/internal/config"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// Theme, "sotto theme list | import <dosya> [--name ad] [--use]".
func Theme(cfg config.Config, args []string, stdout, stderr io.Writer) int {
	f := parse(args)
	_ = theme.LoadUser(cfg.ConfigDir)
	switch f.arg(0) {
	case "", "list":
		for _, p := range theme.All() {
			mark := "  "
			if p.Name == cfg.Look.Theme {
				mark = "* "
			}
			kind := ""
			if p.User {
				kind = "  (yours)"
			}
			fmt.Fprintf(stdout, "%s%s%s\n", mark, p.Name, kind)
		}
		fmt.Fprintf(stdout, "\nYour themes live in %s\n", theme.UserDir(cfg.ConfigDir))
		return 0
	case "import":
		file := f.arg(1)
		if file == "" {
			fmt.Fprintln(stderr, "sotto: theme import needs a file (.itermcolors, Alacritty .toml/.yml or a Windows Terminal scheme .json)")
			return 2
		}
		name, path, err := theme.Import(cfg.ConfigDir, file, f.vals["name"])
		if err != nil {
			fmt.Fprintln(stderr, "sotto:", err)
			return 1
		}
		fmt.Fprintf(stdout, "Saved the theme %q to %s\n", name, path)
		if f.has("use") {
			cfg.Look.Theme = name
			if err := cfg.Save(); err != nil {
				fmt.Fprintln(stderr, "sotto:", err)
				return 1
			}
			fmt.Fprintln(stdout, "sotto uses it from the next start.")
		} else {
			fmt.Fprintln(stdout, "Pick it in Settings → Theme, or run with --use.")
		}
		return 0
	}
	fmt.Fprintf(stderr, "sotto: unknown theme command %q (list or import)\n", f.arg(0))
	return 2
}
