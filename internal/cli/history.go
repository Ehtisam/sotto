package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Ehtisam/sotto/internal/config"
	"github.com/Ehtisam/sotto/internal/history"
)

// History: "sotto history [week|month|year|all] [--json]" yerel dinleme
// geçmişinin özeti (sotto açıkken en az 30 sn dinlenen şarkılar).
func History(cfg config.Config, args []string, stdout, stderr io.Writer, asJSON bool) int {
	r := history.Week
	if len(args) > 0 {
		r = history.Range(strings.ToLower(args[0]))
		ok := false
		for _, x := range history.Ranges {
			ok = ok || x == r
		}
		if !ok {
			fmt.Fprintf(stderr, "sotto: unknown range %q (week, month, year or all)\n", args[0])
			return 2
		}
	}
	plays, err := history.Load(cfg.LocalDir)
	if err != nil {
		fmt.Fprintln(stderr, "sotto:", err)
		return 1
	}
	s := history.Compute(plays, r, time.Now(), 10)
	if asJSON {
		top := func(cs []history.Count) []map[string]any {
			out := []map[string]any{}
			for _, c := range cs {
				out = append(out, map[string]any{"name": c.Name, "by": c.Sub, "uri": c.Key, "plays": c.Plays, "ms": c.MS})
			}
			return out
		}
		b, _ := json.MarshalIndent(map[string]any{
			"range": r, "plays": s.Plays, "ms": s.MS, "songs": s.Songs, "artists": s.Artists, "days": s.Days,
			"by_hour": s.ByHour, "by_weekday": s.ByDay, "top_songs": top(s.Top), "top_artists": top(s.TopArt),
		}, "", "  ")
		fmt.Fprintln(stdout, string(b))
		return 0
	}
	if s.Plays == 0 {
		fmt.Fprintf(stdout, "Nothing recorded for %s yet (sotto keeps a note while it is open).\n", rangeName(r))
		return 0
	}
	d := time.Duration(s.MS) * time.Millisecond
	fmt.Fprintf(stdout, "%s: %d plays, %dh %dm, %d songs, %d artists, %d days\n",
		rangeName(r), s.Plays, int(d.Hours()), int(d.Minutes())%60, s.Songs, s.Artists, s.Days)
	fmt.Fprintln(stdout, "\nMost played songs")
	for i, c := range s.Top {
		fmt.Fprintf(stdout, "%3d. %s — %s  (%d)\n", i+1, c.Name, c.Sub, c.Plays)
	}
	fmt.Fprintln(stdout, "\nMost played artists")
	for i, c := range s.TopArt {
		fmt.Fprintf(stdout, "%3d. %s  (%d)\n", i+1, c.Name, c.Plays)
	}
	best, bestN := 0, -1
	for h, n := range s.ByHour {
		if n > bestN {
			best, bestN = h, n
		}
	}
	fmt.Fprintf(stdout, "\nYou listen most around %02d:00.\n", best)
	return 0
}

func rangeName(r history.Range) string {
	switch r {
	case history.Week:
		return "Last 7 days"
	case history.Month:
		return "Last month"
	case history.Year:
		return "Last year"
	}
	return "All time"
}
