package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// Kabuk tamamlama. Mantık burada, tek yerde: kabuk betikleri yazılan
// sözcükleri "sotto __complete c:<yarım sözcük> <önceki sözcükler…>" ile
// sorar, sotto adayları satır satır yazar. Yarım sözcüğün başındaki "c:",
// Windows PowerShell 5.1'in boş argümanı hiç geçirmemesine karşı.

// subcommands: komutların alt komutları ya da sabit değerleri.
var subcommands = map[string][]string{
	"":           {"play", "pause", "toggle", "next", "prev", "seek", "vol", "mute", "shuffle", "repeat", "status", "like", "unlike", "connect", "search", "queue", "devices", "playlist", "theme", "history", "doctor", "setup", "completion", "help", "version"},
	"mute":       {"on", "off"},
	"shuffle":    {"on", "off"},
	"repeat":     {"off", "context", "track"},
	"playlist":   {"list", "new", "delete", "duplicate", "import"},
	"theme":      {"list", "import"},
	"history":    {"week", "month", "year", "all"},
	"completion": {"powershell", "bash", "zsh", "fish"},
}

// flags: komut (ya da "komut alt") başına bayraklar. --json her komutta var.
var cmdFlags = map[string][]string{
	"play":            {"--type"},
	"search":          {"--type", "--limit"},
	"queue":           {"--type"},
	"playlist new":    {"--public", "--collab", "--desc"},
	"playlist delete": {"--yes"},
	"playlist import": {"--name", "--into"},
	"theme import":    {"--name", "--use"},
}

// flagValues: değer alan bayraklar; değerleri biliniyorsa listesi.
var flagValues = map[string][]string{
	"--type":  {"track", "album", "artist", "playlist", "show", "episode"},
	"--limit": nil,
	"--desc":  nil,
	"--name":  nil,
	"--into":  nil,
}

// Complete, before (önceki sözcükler) ve cur (yarım sözcük) için adaylar.
func Complete(before []string, cur string) []string {
	// Değer bekleyen bir bayraktan sonra: onun değerleri.
	if n := len(before); n > 0 {
		if vals, ok := flagValues[before[n-1]]; ok {
			return withPrefix(vals, cur)
		}
	}
	// Konumsal sözcükler (bayraklar ve değerleri hariç).
	var pos []string
	for i := 0; i < len(before); i++ {
		w := before[i]
		if strings.HasPrefix(w, "-") {
			if _, ok := flagValues[w]; ok {
				i++
			}
			continue
		}
		pos = append(pos, w)
	}
	cmd := ""
	if len(pos) > 0 {
		cmd = pos[0]
		switch cmd { // eş adlar
		case "previous":
			cmd = "prev"
		case "volume":
			cmd = "vol"
		case "playlists":
			cmd = "playlist"
		}
	}
	key := cmd
	if len(pos) > 1 && subcommands[cmd] != nil {
		key = cmd + " " + pos[1]
	}

	if strings.HasPrefix(cur, "-") {
		if cmd == "" {
			return withPrefix([]string{"--help", "--version", "--json"}, cur)
		}
		return withPrefix(append(append([]string(nil), cmdFlags[key]...), "--json"), cur)
	}
	switch {
	case len(pos) == 0:
		return withPrefix(subcommands[""], cur)
	case len(pos) == 1 && subcommands[cmd] != nil:
		return withPrefix(subcommands[cmd], cur)
	}
	return nil // serbest metin (şarkı adı, dosya…): kabuk kendi bildiğini yapsın
}

func withPrefix(list []string, cur string) []string {
	var out []string
	for _, s := range list {
		if strings.HasPrefix(s, cur) {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// completeMain: "sotto __complete c:<cur> <önceki…>".
func completeMain(args []string, stdout io.Writer) int {
	cur := ""
	if len(args) > 0 {
		cur = strings.TrimPrefix(args[0], "c:")
		args = args[1:]
	}
	for _, c := range Complete(args, cur) {
		fmt.Fprintln(stdout, c)
	}
	return 0
}

// Completion, "sotto completion <kabuk>": betiği yazar.
func Completion(args []string, stdout, stderr io.Writer) int {
	shell := ""
	if len(args) > 0 {
		shell = args[0]
	}
	script, ok := completionScripts[shell]
	if !ok {
		fmt.Fprintln(stderr, "sotto: completion needs a shell: powershell, bash, zsh or fish")
		return 2
	}
	fmt.Fprint(stdout, script)
	return 0
}

var completionScripts = map[string]string{
	"powershell": `# sotto tab completion for PowerShell. To load it every time, add this line
# to your profile (notepad $PROFILE):
#   sotto completion powershell | Out-String | Invoke-Expression
Register-ArgumentCompleter -Native -CommandName 'sotto', 'sotto.exe' -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)
    $before = @($commandAst.CommandElements | Select-Object -Skip 1 |
        Where-Object { $_.Extent.EndOffset -lt $cursorPosition } |
        ForEach-Object { $_.Extent.Text })
    $exe = $commandAst.CommandElements[0].Extent.Text
    & $exe __complete "c:$wordToComplete" @before 2>$null | ForEach-Object {
        [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
    }
}
`,
	"bash": `# sotto tab completion for bash. Add to ~/.bashrc:
#   source <(sotto completion bash)
_sotto() {
    local IFS=$'\n'
    COMPREPLY=($(sotto __complete "c:${COMP_WORDS[COMP_CWORD]}" "${COMP_WORDS[@]:1:COMP_CWORD-1}" 2>/dev/null))
}
complete -o default -F _sotto sotto
`,
	"zsh": `#compdef sotto
# sotto tab completion for zsh. Add to ~/.zshrc (after compinit):
#   source <(sotto completion zsh)
_sotto() {
    local -a c
    c=(${(f)"$(sotto __complete "c:${words[CURRENT]}" ${words[2,CURRENT-1]} 2>/dev/null)"})
    (( ${#c} )) && compadd -a c || _files
}
compdef _sotto sotto
`,
	"fish": `# sotto tab completion for fish. Save it once:
#   sotto completion fish > ~/.config/fish/completions/sotto.fish
complete -c sotto -f -a '(sotto __complete "c:"(commandline -ct) (commandline -opc | tail -n +2) 2>/dev/null)'
`,
}
