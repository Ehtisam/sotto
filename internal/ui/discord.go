package ui

import (
	"strings"
	"time"

	"github.com/Ehtisam/sotto/internal/discord"
)

// Discord'da "şu an dinliyor" ([desktop] discord, discord_app_id): çalan
// şarkı, sanatçı, kapak ve kalan süre Discord profilinde görünür; durunca
// temizlenir. Discord geliştirici sitesinde oluşturulan bir uygulamanın
// kimliği gerekir; uygulamanın adı "Listening to …" yazısında görünür.

// discordReady: ayar açık ve kimlik var.
func (m Model) discordReady() bool {
	return m.cfg.Desktop.Discord && strings.TrimSpace(m.cfg.Desktop.DiscordAppID) != ""
}

// syncDiscord, ayara göre bağlantıyı açar ya da kapatır.
func (m Model) syncDiscord() {
	switch {
	case m.discordReady() && m.rt.discord == nil:
		p := discord.Start(strings.TrimSpace(m.cfg.Desktop.DiscordAppID))
		logs := m.rt.logs
		p.Err = func(err error) { logs.add(logError, err.Error(), time.Now()) }
		m.rt.discord = p
		m.updateDiscord()
	case !m.discordReady() && m.rt.discord != nil:
		p := m.rt.discord
		m.rt.discord = nil
		go p.Close()
	}
}

// updateDiscord, çalan şarkıyı Discord'a bildirir (bekletmez).
func (m Model) updateDiscord() {
	p := m.rt.discord
	if p == nil {
		return
	}
	it := m.state.Item
	name, artists, album := m.describe(it)
	if it == nil || name == "" || !m.playing() {
		p.Set(nil)
		return
	}
	a := &discord.Activity{Title: name, Artist: artists, Album: album, Cover: m.notifyCoverURL()}
	if d := it.Duration(); d > 0 {
		a.Start = m.now.Add(-m.position())
		a.End = a.Start.Add(d)
	}
	if parts := strings.Split(it.URI, ":"); len(parts) == 3 && parts[0] == "spotify" {
		a.URL = "https://open.spotify.com/" + parts[1] + "/" + parts[2]
	}
	p.Set(a)
}
