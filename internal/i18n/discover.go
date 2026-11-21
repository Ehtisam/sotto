package i18n

// 3. grup "Keşif" metinleri: sanatçı sayfası, ayrıntılar ve künye,
// Yenilikler, kişiler (takipçiler, arkadaşlar), kitaplıktaki albüm/sanatçı/
// podcast'ler, aramada en iyi sonuç.

func init() {
	for k, v := range discoverTexts {
		texts[k] = v
	}
}

var discoverTexts = map[string]struct{ en, tr string }{
	"yes": {"yes", "evet"},
	// cihaz geçişi: tuşa basıldığı an görünür
	"flash.switching":       {"Switching to %s…", "%s cihazına geçiliyor…"},
	"strip.switching":       {"switching to %s…", "%s cihazına geçiliyor…"},
	"dev.other":             {"the other device", "diğer cihaz"},
	"flash.soloist.stopped": {"Soloist stopped", "Soloist kapatıldı"},
	"hint.section":          {"section", "bölüm"},
	"hint.people":           {"followers", "takipçiler"},
	"hint.info":             {"details", "ayrıntılar"},
	"kind.user":             {"user", "kullanıcı"},
	"extras.login":          {"The extras need a one-time sign-in: run sotto setup", "Ekstralara bir kez giriş gerekiyor: sotto setup"},

	// sanatçı sayfası
	"artist.popular":       {"Popular", "Popüler"},
	"artist.disco":         {"Discography", "Diskografi"},
	"artist.related":       {"Fans also like", "Hayranlar bunları da seviyor"},
	"artist.about":         {"About", "Hakkında"},
	"artist.play":          {"Play %s", "%s çal"},
	"artist.play.sub":      {"h follows · S shuffles", "h takip eder · S karıştırır"},
	"artist.releases":      {"Popular releases", "Popüler çıkanlar"},
	"artist.featuring":     {"Featuring %s", "%s içeren listeler"},
	"artist.g.album":       {"Albums", "Albümler"},
	"artist.g.single":      {"Singles and EPs", "Single ve EP'ler"},
	"artist.g.compilation": {"Compilations", "Derlemeler"},
	"artist.g.appears":     {"Appears on", "Yer aldığı albümler"},
	"artist.listeners":     {"Monthly listeners", "Aylık dinleyici"},
	"artist.verified":      {"Verified artist", "Onaylı sanatçı"},
	"artist.followers":     {"Followers", "Takipçi"},
	"artist.followers.n":   {"%s followers", "%s takipçi"},
	"artist.genres":        {"Genres", "Türler"},
	"artist.bio":           {"Biography", "Biyografi"},
	"artist.nobio.extras":  {"Popular songs, monthly listeners, similar artists and the biography come through the unofficial extras (✱). Turn them on in Settings.", "Popüler şarkılar, aylık dinleyici, benzer sanatçılar ve biyografi resmî olmayan ekstralardan (✱) gelir. Ayarlar'dan açılabilir."},
	"artist.extras":        {"more with the extras (✱)", "ekstralarla (✱) daha fazlası"},

	// ayrıntılar (i)
	"info.title":            {"Details", "Ayrıntılar"},
	"info.none":             {"No details for this.", "Bunun ayrıntısı yok."},
	"info.duration":         {"Length", "Süre"},
	"info.released":         {"Released", "Çıkış"},
	"info.label":            {"Label", "Plak şirketi"},
	"info.explicit":         {"Explicit", "Müstehcen"},
	"info.credits":          {"Credits", "Künye"},
	"info.credits.extras":   {"Songwriters and producers come through the unofficial extras (✱). Turn them on in Settings.", "Söz yazarı, besteci ve yapımcılar resmî olmayan ekstralardan (✱) gelir. Ayarlar'dan açılabilir."},
	"info.source":           {"Source", "Kaynak"},
	"info.copyright":        {"Copyright", "Telif"},
	"info.type":             {"Type", "Tür"},
	"info.type.album":       {"album", "albüm"},
	"info.type.single":      {"single", "single"},
	"info.type.compilation": {"compilation", "derleme"},
	"info.tracks":           {"Songs", "Şarkılar"},
	"info.episodes.open":    {"Episodes", "Bölümler"},
	"info.languages":        {"Language", "Dil"},
	"info.about":            {"About", "Hakkında"},
	"info.resume":           {"You stopped at", "Kaldığın yer"},
	"info.played":           {"played", "dinlendi"},
	"info.list.open":        {"Open the list", "Listeyi aç"},
	"info.owner":            {"Made by", "Oluşturan"},
	"info.visibility":       {"Who can see it", "Kimler görebilir"},
	"act.info":              {"Details and credits", "Ayrıntılar ve künye"},
	"act.artistpage":        {"Open the artist's page", "Sanatçının sayfasını aç"},
	"act.userpage":          {"Open the profile", "Profili aç"},
	"help.info":             {"details: credits, album, podcast info", "ayrıntılar: künye, albüm, podcast bilgisi"},

	"dur.hm": {"%d h %d min", "%d sa %d dk"},
	"dur.m":  {"%d min", "%d dk"},

	"month.1": {"January", "Ocak"}, "month.2": {"February", "Şubat"}, "month.3": {"March", "Mart"},
	"month.4": {"April", "Nisan"}, "month.5": {"May", "Mayıs"}, "month.6": {"June", "Haziran"},
	"month.7": {"July", "Temmuz"}, "month.8": {"August", "Ağustos"}, "month.9": {"September", "Eylül"},
	"month.10": {"October", "Ekim"}, "month.11": {"November", "Kasım"}, "month.12": {"December", "Aralık"},
	"mon.1": {"Jan", "Oca"}, "mon.2": {"Feb", "Şub"}, "mon.3": {"Mar", "Mar"}, "mon.4": {"Apr", "Nis"},
	"mon.5": {"May", "May"}, "mon.6": {"Jun", "Haz"}, "mon.7": {"Jul", "Tem"}, "mon.8": {"Aug", "Ağu"},
	"mon.9": {"Sep", "Eyl"}, "mon.10": {"Oct", "Eki"}, "mon.11": {"Nov", "Kas"}, "mon.12": {"Dec", "Ara"},

	// Yenilikler (g w)
	"news.title":     {"What's new", "Yenilikler"},
	"news.sub":       {"from the %d artists you follow and your podcasts · last %d weeks", "takip ettiğin %d sanatçı ve podcast'lerinden · son %d hafta"},
	"news.loading":   {"Looking at the artists you follow…", "Takip ettiğin sanatçılara bakılıyor…"},
	"news.releases":  {"New releases", "Yeni çıkanlar"},
	"news.episodes":  {"New episodes", "Yeni bölümler"},
	"news.thisweek":  {"This week", "Bu hafta"},
	"news.lastweek":  {"Last week", "Geçen hafta"},
	"news.weeksago":  {"%d weeks ago", "%d hafta önce"},
	"news.none":      {"Nothing new in the last weeks.", "Son haftalarda yeni bir şey yok."},
	"news.noartists": {"You don't follow any artists yet. h on an artist follows them.", "Henüz takip ettiğin sanatçı yok. Bir sanatçıda h takip eder."},
	"help.g.news":    {"what's new: releases from artists you follow, new episodes", "yenilikler: takip ettiğin sanatçıların çıkanları, yeni bölümler"},
	"home.discover":  {"Discover", "Keşfet"},
	"home.more.news": {"What's new from artists you follow (g w)", "Takip ettiğin sanatçılardan yenilikler (g w)"},

	// kişiler (g f)
	"people.title":       {"People", "Kişiler"},
	"people.sub":         {"tab switches · enter opens a profile or artist", "tab geçer · enter profili ya da sanatçıyı açar"},
	"people.followers":   {"Followers", "Takipçiler"},
	"people.following":   {"Following", "Takip edilenler"},
	"people.friends":     {"Friend activity", "Arkadaş etkinliği"},
	"people.nfollowers":  {"%s followers", "%s takipçi"},
	"people.nfollowing":  {"%s following", "%s takip edilen"},
	"people.youfollow":   {"you follow", "takip ediyorsun"},
	"people.users":       {"People", "Kişiler"},
	"people.artists":     {"Artists", "Sanatçılar"},
	"people.now":         {"now", "şimdi"},
	"people.none":        {"Nobody here.", "Burada kimse yok."},
	"people.nofriends":   {"None of the people you follow is sharing what they listen to right now.", "Takip ettiğin kimse şu an dinlediğini paylaşmıyor."},
	"people.extras":      {"Followers and friend activity need the unofficial extras (✱): Settings", "Takipçiler ve arkadaş etkinliği için resmî olmayan ekstralar (✱) gerekiyor: Ayarlar"},
	"people.playlists":   {"Playlists on the profile", "Profilindeki listeler"},
	"people.noplaylists": {"No playlists on this profile.", "Bu profilde liste yok."},
	"people.user.hint":   {"h follows or unfollows · e opens in the browser", "h takip eder ya da bırakır · e tarayıcıda açar"},
	"help.g.people":      {"people: followers, following, friend activity", "kişiler: takipçiler, takip edilenler, arkadaş etkinliği"},

	// kitaplık
	"lib.f.albums":   {"Albums", "Albümler"},
	"lib.f.artists":  {"Artists", "Sanatçılar"},
	"lib.f.podcasts": {"Podcasts", "Podcast'ler"},
	"lib.n.album":    {"1 album", "1 albüm"},
	"lib.n.albums":   {"%d albums", "%d albüm"},
	"lib.n.artist":   {"1 artist", "1 sanatçı"},
	"lib.n.artists":  {"%d artists", "%d sanatçı"},
	"lib.n.podcast":  {"1 podcast", "1 podcast"},
	"lib.n.podcasts": {"%d podcasts", "%d podcast"},

	// arama
	"search.top": {"Top result", "En iyi sonuç"},
}
