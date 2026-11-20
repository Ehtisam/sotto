package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/Ehtisam/sotto/internal/ipc"
	"github.com/Ehtisam/sotto/internal/spotify"
)

var errNoDevice = errors.New("nothing is playing on any device; open sotto or start Spotify somewhere")

// Direct, komutu doğrudan Web API ile yürütür (açık sotto yokken ya da arama
// ve liste işleri için).
func Direct(ctx context.Context, sp *spotify.Client, req ipc.Request, stdin io.Reader, stderr io.Writer) ipc.Response {
	f := parse(req.Args)
	wrap := func(err error) ipc.Response {
		if spotify.IsStatus(err, 404) {
			err = errNoDevice
		}
		return ipc.Fail(Friendly(err, ""))
	}
	ok := func(text string) ipc.Response { return ipc.Done(text, nil) }

	switch req.Cmd {
	case "play":
		if f.arg(0) == "" {
			if err := sp.Resume(ctx); err != nil {
				return wrap(err)
			}
			return ok("")
		}
		uri, label, err := Target(ctx, sp, req.Args)
		if err != nil {
			return ipc.Fail(err)
		}
		if err := PlayURI(ctx, sp, "", uri); err != nil {
			return wrap(err)
		}
		return ok(Playing(label))
	case "pause":
		return errOr(wrap, sp.Pause(ctx), "")
	case "toggle":
		st, err := sp.Player(ctx)
		if err != nil {
			return wrap(err)
		}
		if st == nil {
			return ipc.Fail(errNoDevice)
		}
		if st.IsPlaying {
			return errOr(wrap, sp.Pause(ctx), "")
		}
		return errOr(wrap, sp.Resume(ctx), "")
	case "next":
		return errOr(wrap, sp.Next(ctx), "")
	case "prev":
		return errOr(wrap, sp.Previous(ctx), "")
	case "seek", "vol", "mute", "shuffle", "repeat", "status", "like", "unlike":
		st, err := sp.Player(ctx)
		if err != nil {
			return wrap(err)
		}
		cur := StatusFromAPI(st)
		devWrap := func(err error) ipc.Response { return wrap(Friendly(err, cur.Device)) }
		return withStatus(ctx, sp, req.Cmd, f, cur, st, devWrap)
	case "connect", "devices":
		if f.arg(0) == "" || req.Cmd == "devices" {
			return devices(ctx, sp)
		}
		name, err := Transfer(ctx, sp, f.rest(0))
		if err != nil {
			return ipc.Fail(err)
		}
		return ok("Playing on " + name + ".")
	case "search":
		return search(ctx, sp, f)
	case "queue":
		return queue(ctx, sp, f, wrap)
	case "playlist":
		return playlist(ctx, sp, f, stdin, stderr)
	}
	return ipc.Fail(fmt.Errorf("unknown command %q", req.Cmd))
}

func errOr(wrap func(error) ipc.Response, err error, text string) ipc.Response {
	if err != nil {
		return wrap(err)
	}
	return ipc.Done(text, nil)
}

// withStatus, şimdiki duruma göre çalışan komutlar.
func withStatus(ctx context.Context, sp *spotify.Client, cmd string, f flags, cur Status, st *spotify.PlayerState, wrap func(error) ipc.Response) ipc.Response {
	if cmd == "status" {
		return ipc.Done(cur.Text(), cur)
	}
	if st == nil {
		return ipc.Fail(errNoDevice)
	}
	refuse := func(action string) bool { return Disallowed(st, action, cur.Repeat) }
	switch cmd {
	case "seek":
		ms, err := ParseSeek(f.arg(0), cur.PositionMS)
		if err != nil {
			return ipc.Fail(err)
		}
		if refuse("seek") {
			return ipc.Fail(errors.New(RefusalText("seek", cur.Device)))
		}
		return errOr(wrap, sp.Seek(ctx, ms), "")
	case "vol":
		if f.arg(0) == "" {
			return ipc.Done(fmt.Sprintf("%d", cur.Volume), map[string]int{"volume": cur.Volume})
		}
		v, err := ParseVolume(f.arg(0), cur.Volume)
		if err != nil {
			return ipc.Fail(err)
		}
		if refuse("volume") {
			return ipc.Fail(errors.New(RefusalText("volume", cur.Device)))
		}
		return errOr(wrap, sp.SetVolume(ctx, v), "")
	case "mute":
		want := f.arg(0)
		if want != "" && want != "on" && want != "off" {
			return ipc.Fail(fmt.Errorf("expected on or off, got %q", want))
		}
		if refuse("volume") {
			return ipc.Fail(errors.New(RefusalText("volume", cur.Device)))
		}
		v, save, muted := MuteTarget(cur.Volume, MutedVolume(StateDir), want)
		if v != cur.Volume {
			if err := sp.SetVolume(ctx, v); err != nil {
				return wrap(err)
			}
		}
		SaveMuted(StateDir, save)
		return ipc.Done(MuteText(muted, v), map[string]any{"muted": muted, "volume": v})
	case "shuffle":
		on, err := ParseOnOff(f.arg(0), cur.Shuffle)
		if err != nil {
			return ipc.Fail(err)
		}
		if refuse("shuffle") {
			return ipc.Fail(errors.New(RefusalText("shuffle", cur.Device)))
		}
		return errOr(wrap, sp.SetShuffle(ctx, on), onOffText("Shuffle", on))
	case "repeat":
		mode, err := NextRepeat(f.arg(0), cur.Repeat)
		if err != nil {
			return ipc.Fail(err)
		}
		if refuse("repeat:" + mode) {
			return ipc.Fail(errors.New(RefusalText("repeat", cur.Device)))
		}
		return errOr(wrap, sp.SetRepeat(ctx, mode), "Repeat "+mode+".")
	case "like", "unlike":
		return Like(ctx, sp, cur, cmd == "like")
	}
	return ipc.Fail(fmt.Errorf("unknown command %q", cmd))
}

func onOffText(what string, on bool) string {
	if on {
		return what + " on."
	}
	return what + " off."
}

// Like, şarkıyı Beğenilenler'e ekler ya da çıkarır.
func Like(ctx context.Context, sp *spotify.Client, cur Status, like bool) ipc.Response {
	if cur.URI == "" {
		return ipc.Fail(errors.New("nothing is playing"))
	}
	if like {
		if err := sp.Save(ctx, []string{cur.URI}); err != nil {
			return ipc.Fail(err)
		}
		return ipc.Done("Added "+cur.Title+" to Liked Songs.", nil)
	}
	if err := sp.Unsave(ctx, []string{cur.URI}); err != nil {
		return ipc.Fail(err)
	}
	return ipc.Done("Removed "+cur.Title+" from Liked Songs.", nil)
}

// StatusFromAPI, Web API'nin çalma durumunu Status'a çevirir.
func StatusFromAPI(st *spotify.PlayerState) Status {
	if st == nil {
		return Status{}
	}
	s := Status{Playing: st.IsPlaying, Device: st.Device.Name, PositionMS: st.ProgressMS,
		Shuffle: st.ShuffleState, Repeat: st.RepeatState}
	if st.Device.Volume != nil {
		s.Volume = *st.Device.Volume
	}
	if t := st.Item; t != nil {
		s.Title, s.Artist, s.Album, s.URI = t.Name, t.ArtistNames(), t.Album.Name, t.URI
		s.DurationMS, s.Link = int64(t.DurationMS), spotify.WebLink(t.URI)
	}
	if st.Context != nil {
		s.ContextURI = st.Context.URI
	}
	return s
}

// PlayURI, bir adresi çalar: şarkı ya da bölümse tek başına, değilse bağlam olarak.
func PlayURI(ctx context.Context, sp *spotify.Client, deviceID, uri string) error {
	switch Kind(uri) {
	case "track", "episode":
		return sp.PlayTracks(ctx, deviceID, []string{uri}, "")
	}
	return sp.PlayContext(ctx, deviceID, uri, "")
}

// FindDevice, adı verilen cihazı bulur (büyük/küçük harf duyarsız, baştan eşleşme).
func FindDevice(ds []spotify.Device, name string) (spotify.Device, error) {
	want := strings.ToLower(strings.TrimSpace(name))
	var prefix []spotify.Device
	for _, d := range ds {
		n := strings.ToLower(d.Name)
		if n == want {
			return d, nil
		}
		if strings.HasPrefix(n, want) {
			prefix = append(prefix, d)
		}
	}
	if len(prefix) == 1 {
		return prefix[0], nil
	}
	if len(prefix) > 1 {
		return spotify.Device{}, fmt.Errorf("%q matches more than one device", name)
	}
	return spotify.Device{}, fmt.Errorf("no device called %q (sotto devices lists them)", name)
}

// Transfer, müziği adı verilen cihaza aktarır.
func Transfer(ctx context.Context, sp *spotify.Client, name string) (string, error) {
	ds, err := sp.Devices(ctx)
	if err != nil {
		return "", err
	}
	d, err := FindDevice(ds, name)
	if err != nil {
		return "", err
	}
	return d.Name, sp.Transfer(ctx, d.ID, true)
}

func devices(ctx context.Context, sp *spotify.Client) ipc.Response {
	ds, err := sp.Devices(ctx)
	if err != nil {
		return ipc.Fail(err)
	}
	if len(ds) == 0 {
		return ipc.Done("No devices. Start Spotify somewhere or open sotto.", ds)
	}
	var b strings.Builder
	for _, d := range ds {
		mark := "  "
		if d.IsActive {
			mark = "▶ "
		}
		fmt.Fprintf(&b, "%s%s  (%s)\n", mark, d.Name, strings.ToLower(d.Type))
	}
	return ipc.Done(strings.TrimRight(b.String(), "\n"), ds)
}

// ---- arama ----

type found struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Artists string `json:"artists,omitempty"`
	URI     string `json:"uri"`
	Link    string `json:"link"`
}

func search(ctx context.Context, sp *spotify.Client, f flags) ipc.Response {
	q := f.rest(0)
	if q == "" {
		return ipc.Fail(errors.New("search needs words, like: sotto search daft punk --type album"))
	}
	types := strings.Split(f.vals["type"], ",")
	if f.vals["type"] == "" {
		types = []string{"track"}
	}
	limit := max(1, min(f.int("limit", spotify.SearchLimit), spotify.SearchLimit))
	r, err := sp.Search(ctx, q, types, 0)
	if err != nil {
		return ipc.Fail(err)
	}
	var out []found
	per := map[string]int{}
	add := func(kind, name, artists, uri string) {
		if uri != "" && per[kind] < limit {
			per[kind]++
			out = append(out, found{kind, name, artists, uri, spotify.WebLink(uri)})
		}
	}
	for _, t := range r.Tracks.Items {
		if t != nil {
			add("track", t.Name, t.ArtistNames(), t.URI)
		}
	}
	for _, a := range r.Albums.Items {
		if a != nil {
			add("album", a.Name, artistNames(a.Artists), a.URI)
		}
	}
	for _, a := range r.Artists.Items {
		if a != nil {
			add("artist", a.Name, "", a.URI)
		}
	}
	for _, p := range r.Playlists.Items {
		if p != nil {
			add("playlist", p.Name, p.Owner.DisplayName, p.URI)
		}
	}
	for _, s := range r.Shows.Items {
		if s != nil {
			add("show", s.Name, "", s.URI)
		}
	}
	for _, e := range r.Episodes.Items {
		if e != nil {
			add("episode", e.Name, "", e.URI)
		}
	}
	if len(out) == 0 {
		return ipc.Done("Nothing found.", out)
	}
	var b strings.Builder
	for _, x := range out {
		line := x.Name
		if x.Artists != "" {
			line += " — " + x.Artists
		}
		fmt.Fprintf(&b, "%-8s %s\n         %s\n", x.Type, line, x.URI)
	}
	return ipc.Done(strings.TrimRight(b.String(), "\n"), out)
}

func artistNames(as []spotify.Artist) string {
	names := make([]string, 0, len(as))
	for _, a := range as {
		names = append(names, a.Name)
	}
	return strings.Join(names, ", ")
}

// ---- sıra ----

func queue(ctx context.Context, sp *spotify.Client, f flags, wrap func(error) ipc.Response) ipc.Response {
	if f.arg(0) != "" {
		var args []string
		for _, p := range f.pos {
			args = append(args, p)
		}
		if t := f.vals["type"]; t != "" {
			args = append(args, "--type", t)
		}
		uri, label, err := Target(ctx, sp, args)
		if err != nil {
			return ipc.Fail(err)
		}
		if k := Kind(uri); k != "track" && k != "episode" {
			return ipc.Fail(errors.New("only songs and episodes can go in the queue"))
		}
		text := "Added to the queue."
		if label != "" {
			text = "Added " + label + " to the queue."
		}
		return errOr(wrap, sp.AddToQueue(ctx, uri), text)
	}
	q, err := sp.Queue(ctx)
	if err != nil {
		return wrap(err)
	}
	var out []found
	var b strings.Builder
	for i, t := range q {
		out = append(out, found{t.Type, t.Name, t.ArtistNames(), t.URI, spotify.WebLink(t.URI)})
		fmt.Fprintf(&b, "%2d. %s — %s\n", i+1, t.Name, t.ArtistNames())
	}
	if len(q) == 0 {
		return ipc.Done("The queue is empty.", out)
	}
	return ipc.Done(strings.TrimRight(b.String(), "\n"), out)
}

// ---- listeler ----

type listInfo struct {
	Name   string `json:"name"`
	URI    string `json:"uri"`
	Owner  string `json:"owner"`
	Tracks int    `json:"tracks"`
	Public bool   `json:"public"`
	Link   string `json:"link"`
}

func allPlaylists(ctx context.Context, sp *spotify.Client) ([]spotify.Playlist, error) {
	var all []spotify.Playlist
	for off := 0; ; off += 50 {
		p, err := sp.MyPlaylists(ctx, off, 50)
		if err != nil {
			return nil, err
		}
		all = append(all, p.Items...)
		if p.Next == "" || len(p.Items) == 0 {
			return all, nil
		}
	}
}

// findPlaylist, adı, adresi ya da bağlantısı verilen listeyi kitaplıkta bulur.
func findPlaylist(lists []spotify.Playlist, q string) (spotify.Playlist, error) {
	q = strings.TrimSpace(q)
	if uri, err := ToURI(q); err == nil {
		q = uri
	}
	id := strings.TrimPrefix(q, "spotify:playlist:")
	var exact, prefix []spotify.Playlist
	low := strings.ToLower(q)
	for _, p := range lists {
		switch {
		case p.ID == id || p.URI == q:
			return p, nil
		case strings.ToLower(p.Name) == low:
			exact = append(exact, p)
		case strings.HasPrefix(strings.ToLower(p.Name), low):
			prefix = append(prefix, p)
		}
	}
	pick := exact
	if len(pick) == 0 {
		pick = prefix
	}
	switch len(pick) {
	case 1:
		return pick[0], nil
	case 0:
		return spotify.Playlist{}, fmt.Errorf("no playlist called %q in your library", q)
	}
	var names []string
	for _, p := range pick {
		names = append(names, p.Name+" ("+p.URI+")")
	}
	return spotify.Playlist{}, fmt.Errorf("%q matches more than one playlist: %s", q, strings.Join(names, ", "))
}

func playlist(ctx context.Context, sp *spotify.Client, f flags, stdin io.Reader, stderr io.Writer) ipc.Response {
	sub := f.arg(0)
	switch sub {
	case "", "list", "ls":
		lists, err := allPlaylists(ctx, sp)
		if err != nil {
			return ipc.Fail(err)
		}
		var out []listInfo
		var b strings.Builder
		for _, p := range lists {
			pub := p.Public != nil && *p.Public
			out = append(out, listInfo{p.Name, p.URI, p.Owner.DisplayName, p.Total(), pub, spotify.WebLink(p.URI)})
			fmt.Fprintf(&b, "%s  (%d) · %s\n", p.Name, p.Total(), p.Owner.DisplayName)
		}
		return ipc.Done(strings.TrimRight(b.String(), "\n"), out)

	case "new", "create":
		name := f.rest(1)
		if name == "" {
			return ipc.Fail(errors.New("the playlist needs a name: sotto playlist new <name>"))
		}
		p, err := create(ctx, sp, name, f.vals["desc"], f.has("public"), f.has("collab"))
		if err != nil {
			return ipc.Fail(err)
		}
		return ipc.Done("Created "+p.Name+": "+p.URI, listInfo{Name: p.Name, URI: p.URI, Link: spotify.WebLink(p.URI)})

	case "delete", "rm", "remove":
		lists, err := allPlaylists(ctx, sp)
		if err != nil {
			return ipc.Fail(err)
		}
		p, err := findPlaylist(lists, f.rest(1))
		if err != nil {
			return ipc.Fail(err)
		}
		if full, err := sp.Playlist(ctx, p.ID); err == nil {
			p = full // listedeki sayı gecikebiliyor
		}
		if !f.has("yes") {
			fmt.Fprintf(stderr, "Delete %q (%d songs)? Spotify keeps deleted playlists for 90 days. [y/N] ", p.Name, p.Total())
			line, _ := bufio.NewReader(stdin).ReadString('\n')
			if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" && a != "e" && a != "evet" {
				return ipc.Fail(errors.New("not deleted"))
			}
		}
		if err := sp.Unsave(ctx, []string{p.URI}); err != nil {
			return ipc.Fail(err)
		}
		return ipc.Done("Deleted "+p.Name+".", nil)

	case "duplicate", "copy", "dup":
		lists, err := allPlaylists(ctx, sp)
		if err != nil {
			return ipc.Fail(err)
		}
		src, err := findPlaylist(lists, f.arg(1))
		if err != nil {
			return ipc.Fail(err)
		}
		name := f.rest(2)
		if name == "" {
			name = src.Name + " (copy)"
		}
		uris, skipped, err := playlistURIs(ctx, sp, src.ID)
		if err != nil {
			return ipc.Fail(err)
		}
		p, err := create(ctx, sp, name, src.Description, false, false)
		if err != nil {
			return ipc.Fail(err)
		}
		if err := addAll(ctx, sp, p.ID, uris); err != nil {
			return ipc.Fail(fmt.Errorf("created %s but adding songs failed: %w", p.Name, err))
		}
		text := fmt.Sprintf("Created %s with %d songs: %s", p.Name, len(uris), p.URI)
		if skipped > 0 {
			text += fmt.Sprintf("\n%d local files were left out (Spotify does not allow adding them here).", skipped)
		}
		return ipc.Done(text, listInfo{Name: p.Name, URI: p.URI, Tracks: len(uris), Link: spotify.WebLink(p.URI)})

	case "import":
		return importList(ctx, sp, f, stdin, stderr)
	}
	return ipc.Fail(fmt.Errorf("unknown playlist command %q (list, new, delete, duplicate, import)", sub))
}

func create(ctx context.Context, sp *spotify.Client, name, desc string, public, collab bool) (spotify.Playlist, error) {
	me, err := sp.Me(ctx)
	if err != nil {
		return spotify.Playlist{}, err
	}
	pub := public && !collab
	d := spotify.PlaylistDetails{Name: &name, Public: &pub}
	if desc != "" {
		d.Description = &desc
	}
	if collab {
		d.Collaborative = &collab
	}
	return sp.CreatePlaylist(ctx, me.ID, d)
}

func playlistURIs(ctx context.Context, sp *spotify.Client, id string) (uris []string, skipped int, err error) {
	for off := 0; ; off += 100 {
		p, err := sp.PlaylistItems(ctx, id, off, 100)
		if err != nil {
			return nil, 0, err
		}
		for _, it := range p.Items {
			t := it.T()
			switch {
			case t == nil || t.URI == "":
			case t.IsLocal || strings.HasPrefix(t.URI, "spotify:local:"):
				skipped++
			default:
				uris = append(uris, t.URI)
			}
		}
		if p.Next == "" || len(p.Items) == 0 {
			return uris, skipped, nil
		}
	}
}

func addAll(ctx context.Context, sp *spotify.Client, id string, uris []string) error {
	for i := 0; i < len(uris); i += 100 {
		if err := sp.AddToPlaylist(ctx, id, uris[i:min(i+100, len(uris))]); err != nil {
			return err
		}
	}
	return nil
}

// importList, bir dosyadaki şarkıları listeye koyar. Her satır bir Spotify
// bağlantısı/adresi ya da "sanatçı - şarkı" (aranır, ilk sonuç alınır).
func importList(ctx context.Context, sp *spotify.Client, f flags, stdin io.Reader, stderr io.Writer) ipc.Response {
	file := f.arg(1)
	if file == "" {
		return ipc.Fail(errors.New("import needs a file (or - for standard input)"))
	}
	var r io.Reader = stdin
	if file != "-" {
		fh, err := os.Open(file)
		if err != nil {
			return ipc.Fail(err)
		}
		defer fh.Close()
		r = fh
	}
	var uris, missed []string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), string(rune(0xFEFF))))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if uri, err := ToURI(line); err == nil {
			switch Kind(uri) {
			case "track", "episode":
				uris = append(uris, uri)
			default:
				missed = append(missed, line+" (only songs and episodes)")
			}
			continue
		}
		res, err := sp.Search(ctx, strings.ReplaceAll(line, " - ", " "), []string{"track"}, 0)
		if err != nil {
			return ipc.Fail(err)
		}
		hit := ""
		for _, t := range res.Tracks.Items {
			if t != nil && Matches(line, t.Name, t.ArtistNames()) {
				hit = t.URI
				break
			}
		}
		if hit == "" {
			missed = append(missed, line)
			continue
		}
		uris = append(uris, hit)
	}
	if err := sc.Err(); err != nil {
		return ipc.Fail(err)
	}
	if len(uris) == 0 {
		return ipc.Fail(errors.New("no songs found in the file"))
	}

	var target spotify.Playlist
	if into := f.vals["into"]; into != "" {
		lists, err := allPlaylists(ctx, sp)
		if err != nil {
			return ipc.Fail(err)
		}
		if target, err = findPlaylist(lists, into); err != nil {
			return ipc.Fail(err)
		}
	} else {
		name := f.vals["name"]
		if name == "" {
			name = "Imported"
			if file != "-" {
				name = strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
			}
		}
		var err error
		if target, err = create(ctx, sp, name, "", false, false); err != nil {
			return ipc.Fail(err)
		}
	}
	if err := addAll(ctx, sp, target.ID, uris); err != nil {
		return ipc.Fail(err)
	}
	text := fmt.Sprintf("Added %d songs to %s: %s", len(uris), target.Name, target.URI)
	if len(missed) > 0 {
		text += fmt.Sprintf("\nNot found (%d):\n  %s", len(missed), strings.Join(missed, "\n  "))
	}
	return ipc.Done(text, map[string]any{"playlist": target.URI, "added": len(uris), "missed": missed})
}

// words, metni küçük harfli sözcüklere ayırır (noktalama atılır).
func words(s string) []string {
	return strings.FieldsFunc(fold.Replace(strings.ToLower(s)), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// fold: aksanlı harfleri sadeleştirir; "seksendort" "Seksendört"e uyar.
var fold = strings.NewReplacer(
	"ö", "o", "ü", "u", "ş", "s", "ç", "c", "ğ", "g", "ı", "i", "i̇", "i",
	"é", "e", "è", "e", "ê", "e", "ë", "e", "á", "a", "à", "a", "â", "a", "ä", "a", "ã", "a", "å", "a",
	"í", "i", "ì", "i", "î", "i", "ï", "i", "ó", "o", "ò", "o", "ô", "o", "õ", "o", "ø", "o",
	"ú", "u", "ù", "u", "û", "u", "ñ", "n", "ß", "ss",
)

// Matches, bir arama sonucunun içe aktarılan satıra gerçekten uyup uymadığını
// söyler: Spotify'ın araması her şeye bir sonuç bulur. Satırdaki sözcüklerin
// en az yarısı şarkının adında ya da sanatçısında geçmeli; "sanatçı - şarkı"
// biçiminde şarkı adının sözcüklerinin çoğu da.
func Matches(line, title, artists string) bool {
	have := map[string]bool{}
	for _, w := range words(title + " " + artists) {
		have[w] = true
	}
	share := func(s string) float64 {
		ws := words(s)
		if len(ws) == 0 {
			return 0
		}
		n := 0
		for _, w := range ws {
			if have[w] {
				n++
			}
		}
		return float64(n) / float64(len(ws))
	}
	if a, t, ok := strings.Cut(line, " - "); ok {
		return share(a+" "+t) >= 0.5 && share(t) >= 0.5
	}
	return share(line) >= 0.5
}

// Fold, metni karşılaştırma için sadeleştirir: küçük harf, aksansız.
func Fold(s string) string { return fold.Replace(strings.ToLower(s)) }
