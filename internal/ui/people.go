package ui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Ehtisam/sotto/internal/spclient"
	"github.com/Ehtisam/sotto/internal/spotify"
	"github.com/Ehtisam/sotto/internal/ui/theme"
)

// Kişiler (ek sayfa, g f) ✱: Takipçiler · Takip edilenler · Arkadaş
// etkinliği. Bir kişiye enter onun profilini (extraUser) açar: sayılar ve
// profilinde gösterdiği listeler; h takip eder ya da bırakır. Takip edilen
// sanatçıya enter sanatçı sayfasını açar. Hepsi iç yoldan; resmî Web API
// takipçi listesi vermiyor.

type peopleSection int

const (
	pplFollowers peopleSection = iota
	pplFollowing
	pplFriends
	pplCount
)

var peopleKeys = [pplCount]string{"people.followers", "people.following", "people.friends"}

type peopleState struct {
	section peopleSection
	user    string // kimin listeleri (kendi profilin ya da başkasınınki)
	name    string

	loading [pplCount]bool
	asked   [pplCount]bool
	err     [pplCount]error
	login   bool

	followers []spclient.UserCard
	following []spclient.UserCard
	friends   []spclient.Friend
	fetched   time.Time
}

type peopleMsg struct {
	user    string
	section peopleSection
	cards   []spclient.UserCard
	friends []spclient.Friend
	login   bool
	err     error
}

type userState struct {
	id, uri, name string
	loading       bool
	err           error
	login         bool
	view          *spclient.Profile
}

type userMsg struct {
	id    string
	view  *spclient.Profile
	login bool
	err   error
}

// openUserMsg, bir kullanıcının profilini açtırır.
type openUserMsg struct{ uri, name string }

func openUserCmd(uri, name string) tea.Cmd {
	return func() tea.Msg { return openUserMsg{uri, name} }
}

// openPeople, kişiler sayfasını açar (user boşsa kendi profilin).
func (m Model) openPeople(user, name string, section peopleSection) (tea.Model, tea.Cmd) {
	if m.spState != spReady {
		return m.say(T("flash.noaccount")), nil
	}
	if !m.cfg.Extras.Enabled {
		return m.say(T("people.extras")), nil
	}
	if user == "" {
		user, name = m.me.ID, ""
	}
	if user == "" {
		return m.note(T("lib.loading")), nil
	}
	var prev *extraState
	if m.extraOn() {
		p := m.extra
		prev = &p
	}
	if user != m.me.ID && section == pplFriends {
		section = pplFollowers
	}
	ps := &peopleState{section: section, user: user, name: name}
	m.extra = extraState{page: extraPeople, host: m.page, rng: m.extra.rng, prev: prev, people: ps}
	m.focus = focusContent
	m.rebuildExtra()
	return m, m.loadPeople(section)
}

func (m Model) loadPeople(sec peopleSection) tea.Cmd {
	ps := m.extra.people
	if ps == nil {
		return nil
	}
	ps.loading[sec], ps.asked[sec] = true, true
	user, br := ps.user, m.rt.bridge
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		msg := peopleMsg{user: user, section: sec}
		login, err := withBridge(ctx, br, func(tok string) error {
			var err error
			switch sec {
			case pplFollowers:
				msg.cards, err = spclient.Followers(ctx, tok, user)
			case pplFollowing:
				msg.cards, err = spclient.Following(ctx, tok, user)
			case pplFriends:
				msg.friends, err = spclient.Friends(ctx, tok)
			}
			return err
		})
		msg.login, msg.err = login != "", err
		return msg
	}
}

func (m Model) updatePeople(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case peopleMsg:
		for x := &m.extra; x != nil; x = x.prev {
			ps := x.people
			if ps == nil || ps.user != msg.user {
				continue
			}
			ps.loading[msg.section], ps.err[msg.section], ps.login = false, msg.err, msg.login
			switch msg.section {
			case pplFollowers:
				ps.followers = msg.cards
			case pplFollowing:
				ps.following = msg.cards
			case pplFriends:
				ps.friends = msg.friends
			}
		}
		if msg.login {
			m = m.note(T("extras.login"))
		}
	case openUserMsg:
		mm, cmd := m.openUser(msg.uri, msg.name)
		return mm, cmd
	case userMsg:
		for x := &m.extra; x != nil; x = x.prev {
			if u := x.user; u != nil && u.id == msg.id {
				u.loading, u.err, u.login, u.view = false, msg.err, msg.login, msg.view
				if msg.view != nil && msg.view.Name != "" {
					u.name = msg.view.Name
				}
			}
		}
		if msg.login {
			m = m.note(T("extras.login"))
		}
	}
	m.rebuildExtra()
	return m, nil
}

func (m Model) switchPeopleSection(d int) (Model, tea.Cmd) {
	ps := m.extra.people
	n := pplCount
	if ps.user != m.me.ID {
		n = pplFriends // başkasının arkadaş etkinliği yok
	}
	ps.section = peopleSection((int(ps.section) + d + int(n)) % int(n))
	m.extra.cursor, m.extra.scroll = 0, 0
	m.rebuildExtra()
	if !ps.asked[ps.section] {
		return m, m.loadPeople(ps.section)
	}
	return m, nil
}

func (m Model) peopleItems() []searchItem {
	ps := m.extra.people
	if ps == nil {
		return nil
	}
	var out []searchItem
	cards := func(cs []spclient.UserCard) {
		var users, artists []searchItem
		for _, c := range cs {
			sub := F("people.nfollowers", groupDigits(c.Followers))
			if strings.HasPrefix(c.URI, "spotify:artist:") {
				artists = append(artists, searchItem{kind: "artist", name: c.Name, sub: joinDot(T("kind.artist"), sub), uri: c.URI})
				continue
			}
			if c.Following {
				sub = joinDot(sub, T("people.youfollow"))
			}
			users = append(users, searchItem{kind: "user", name: c.Name, sub: sub, uri: c.URI})
		}
		if len(users) > 0 && len(artists) > 0 {
			out = append(out, searchItem{kind: "header", section: "people.users"})
		}
		out = append(out, users...)
		if len(artists) > 0 {
			if len(users) > 0 {
				out = append(out, searchItem{kind: "space"}, searchItem{kind: "header", section: "people.artists"})
			}
			out = append(out, artists...)
		}
	}
	switch ps.section {
	case pplFollowers:
		cards(ps.followers)
	case pplFollowing:
		cards(ps.following)
	case pplFriends:
		for _, f := range ps.friends {
			if len(out) > 0 {
				out = append(out, searchItem{kind: "space"})
			}
			when := ago(m.now, time.UnixMilli(f.Timestamp))
			if m.now.Sub(time.UnixMilli(f.Timestamp)) < 5*time.Minute {
				when = T("people.now")
			}
			out = append(out, searchItem{kind: "user", name: f.User.Name, sub: when, uri: f.User.URI})
			t := f.Track
			tr := &spotify.Track{ID: spotify.IDFromURI(t.URI), Name: t.Name, URI: t.URI, Type: "track",
				Artists: []spotify.Artist{{Name: t.Artist.Name, URI: t.Artist.URI}},
				Album:   spotify.Album{ID: spotify.IDFromURI(t.Album.URI), URI: t.Album.URI, Name: t.Album.Name}}
			sub := joinDot(t.Artist.Name, t.Ctx.Name)
			out = append(out, searchItem{kind: "track", name: "  " + theme.Icon.Note + " " + t.Name, sub: sub, uri: t.URI, track: tr, ctx: t.Ctx.URI, history: true})
		}
	}
	return out
}

func (m Model) peopleHeader(w int) []string {
	ps := m.extra.people
	title := T("people.title")
	if ps.name != "" && ps.user != m.me.ID {
		title = ps.name + " · " + title
	}
	sub := T("people.sub")
	var chips []string
	n := pplCount
	if ps.user != m.me.ID {
		n = pplFriends
	}
	for s := peopleSection(0); s < n; s++ {
		label := T(peopleKeys[s])
		count := -1
		switch s {
		case pplFollowers:
			if ps.asked[s] && !ps.loading[s] {
				count = len(ps.followers)
			}
		case pplFollowing:
			if ps.asked[s] && !ps.loading[s] {
				count = len(ps.following)
			}
		}
		if count >= 0 {
			label += " " + groupDigits(count)
		}
		if s == ps.section {
			chips = append(chips, theme.Hi.Underline(true).Render(label))
		} else {
			chips = append(chips, theme.Dim.Render(label))
		}
	}
	return []string{theme.Title.Render(truncate(title, w)), theme.Dim.Render(truncate(sub, w)), joinChips(chips), ""}
}

func (m Model) peopleEmpty() (string, bool) {
	ps := m.extra.people
	s := ps.section
	switch {
	case ps.loading[s] || !ps.asked[s]:
		return T("lib.loading"), false
	case ps.login:
		return T("extras.login"), true
	case ps.err[s] != nil:
		return ps.err[s].Error(), true
	case s == pplFriends:
		return T("people.nofriends"), false
	}
	return T("people.none"), false
}

// ---- başka bir kullanıcının profili ----

func (m Model) openUser(uri, name string) (Model, tea.Cmd) {
	if m.spState != spReady {
		return m.say(T("flash.noaccount")), nil
	}
	if !m.cfg.Extras.Enabled {
		return m.say(T("people.extras")), nil
	}
	id := strings.TrimPrefix(uri, "spotify:user:")
	if id == "" || id == uri {
		return m, nil
	}
	if id == m.me.ID { // kendi profilin
		m.extra = extraState{rng: m.extra.rng}
		m.page, m.focus = pageProfile, focusContent
		return m, m.enterPage()
	}
	var prev *extraState
	if m.extraOn() {
		p := m.extra
		prev = &p
	}
	u := &userState{id: id, uri: uri, name: name, loading: true}
	m.extra = extraState{page: extraUser, host: m.page, rng: m.extra.rng, prev: prev, user: u}
	m.focus = focusContent
	m.rebuildExtra()
	return m, m.loadUser(id)
}

func (m Model) loadUser(id string) tea.Cmd {
	br := m.rt.bridge
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		msg := userMsg{id: id}
		login, err := withBridge(ctx, br, func(tok string) error {
			p, err := spclient.ProfileView(ctx, tok, id)
			if err == nil {
				msg.view = &p
			}
			return err
		})
		msg.login, msg.err = login != "", err
		return msg
	}
}

func (m Model) userItems() []searchItem {
	u := m.extra.user
	if u == nil || u.view == nil {
		return nil
	}
	out := []searchItem{
		{kind: "people", name: T("people.followers"), sub: groupDigits(u.view.Followers), uri: u.uri, section: "followers"},
		{kind: "people", name: T("people.following"), sub: groupDigits(u.view.FollowingN), uri: u.uri, section: "following"},
	}
	if len(u.view.Playlists) > 0 {
		out = append(out, searchItem{kind: "space"}, searchItem{kind: "header", section: "people.playlists"})
		for _, p := range u.view.Playlists {
			id := spotify.IDFromURI(p.URI)
			e := libEntry{kind: kindPlaylist, key: id, uri: p.URI, name: p.Name, owner: p.OwnerName, image: p.ImageURL, public: true}
			out = append(out, searchItem{kind: "playlist", name: p.Name, sub: p.OwnerName, uri: p.URI, entry: e})
		}
	}
	return out
}

func (m Model) userHeader(w int) []string {
	u := m.extra.user
	name := u.name
	if name == "" {
		name = u.id
	}
	var sub []string
	sub = append(sub, T("kind.user"))
	if v := u.view; v != nil {
		sub = append(sub, F("people.nfollowers", groupDigits(v.Followers)))
		if v.Following {
			sub = append(sub, T("people.youfollow"))
		}
	}
	return []string{theme.Title.Render(truncate(name, w)), theme.Dim.Render(truncate(strings.Join(sub, " · "), w)), theme.Faint.Render(truncate(T("people.user.hint"), w)), ""}
}

// openUserInBrowser, e: kullanıcının Spotify profilini tarayıcıda açar.
func (m Model) openUserInBrowser() Model {
	u := m.extra.user
	if err := openBrowser(spotify.ProfileLink(u.id)); err != nil {
		return m.say(err.Error())
	}
	return m.note(T("prof.opened"))
}

func (m Model) userEmpty() (string, bool) {
	u := m.extra.user
	switch {
	case u.loading:
		return T("lib.loading"), false
	case u.login:
		return T("extras.login"), true
	case u.err != nil:
		return u.err.Error(), true
	}
	return T("people.noplaylists"), false
}

// openPeopleMsg, bir kullanıcının kişiler sayfasını açtırır.
type openPeopleMsg struct {
	user, name string
	section    peopleSection
}
