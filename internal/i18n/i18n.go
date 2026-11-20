// Package i18n, sotto'nun arayüz metinleri. Varsayılan dil İngilizce.
package i18n

import (
	"fmt"
	"strings"
)

var lang = "en"

// Set, arayüz dilini ayarlar ("en" ya da "tr"). Bilinmeyen değerler İngilizceye düşer.
func Set(l string) {
	l = strings.ToLower(strings.TrimSpace(l))
	if l == "tr" {
		lang = "tr"
		return
	}
	lang = "en"
}

// Lang, geçerli dil.
func Lang() string { return lang }

// T, anahtarın geçerli dildeki karşılığı.
func T(key string) string {
	s, ok := texts[key]
	if !ok {
		return key
	}
	if lang == "tr" {
		return s.tr
	}
	return s.en
}

// F, biçimlendirilmiş metin.
func F(key string, args ...any) string { return fmt.Sprintf(T(key), args...) }

var texts = map[string]struct{ en, tr string }{
	// sayfalar
	"page.now":      {"Now playing", "Şimdi çalıyor"},
	"page.home":     {"Home", "Ana sayfa"},
	"page.search":   {"Search", "Ara"},
	"page.library":  {"Library", "Kitaplık"},
	"page.queue":    {"Queue", "Sıra"},
	"page.devices":  {"Devices", "Cihazlar"},
	"page.profile":  {"Profile", "Profil"},
	"page.settings": {"Settings", "Ayarlar"},

	"soon.home":      {"jump back in, recently played, top tracks", "kaldığın yerden devam, son çalınanlar, en çok dinlenenler"},
	"soon.search":    {"songs, albums, artists, playlists, podcasts", "şarkı, albüm, sanatçı, çalma listesi, podcast"},
	"soon.queue":     {"up next and recently played", "sıradakiler ve son çalınanlar"},
	"soon.devices":   {"Soloist, the Spotify app, your phone", "Soloist, Spotify uygulaması, telefon"},
	"soon.profile":   {"photo, followers, top tracks", "fotoğraf, takipçiler, en çok dinlenenler"},
	"soon.settings":  {"colours, themes, keys, speaker", "renkler, temalar, kısayollar, hoparlör"},
	"page.unwritten": {"This page isn't written yet.", "Bu sayfa henüz yazılmadı."},
	"page.coming":    {"coming: ", "gelecek: "},

	// kabuk
	"tagline":  {"softly", "alçak sesle"},
	"contents": {"contents", "içindekiler"},

	// cihaz durumu (sağ üst)
	"dev.off":      {"speaker off", "hoparlör kapalı"},
	"dev.starting": {"starting Soloist…", "Soloist başlatılıyor…"},
	"dev.failed":   {"Soloist is off", "Soloist kapalı"},
	"dev.connect":  {"connecting…", "bağlanıyor…"},
	"dev.pairing":  {"waiting for pairing", "eşleşme bekleniyor"},
	"dev.light":    {" · light mode", " · hafif mod"},
	"dev.inactive": {" · not active", " · etkin değil"},
	"dev.remote":   {" · remote", " · kumanda"},
	"dev.none":     {"no active device", "etkin cihaz yok"},

	// şimdi çalıyor
	"now.nothing":     {"Nothing is playing.", "Çalan bir şey yok."},
	"now.nothing.how": {"Pick a playlist in Library, or press space to pick up where you left off.", "Kitaplık'tan bir liste seç ya da boşlukla kaldığın yerden devam et."},
	"now.inactive":    {"sotto isn't the active device. Press a to play here.", "sotto şu an etkin cihaz değil. Buraya almak için a'ya bas."},
	"now.remote":      {"Playing on %s. sotto is the remote.", "%s üzerinde çalıyor. sotto kumanda olarak çalışıyor."},
	"now.upnext":      {"up next", "sıradakiler"},
	"now.more":        {"    and %d more", "    ve %d şarkı daha"},

	// Soloist durumları
	"eng.off":          {"Light mode is off.", "Hafif mod kapalı."},
	"eng.starting":     {"Starting Soloist…", "Soloist başlatılıyor…"},
	"eng.notfound":     {"Soloist wasn't found.", "Soloist bulunamadı."},
	"eng.expected":     {"Expected at: ", "Beklenen yer: "},
	"eng.wizard":       {"The setup wizard will download it for you later.", "İlk kurulum sihirbazı ileride bunu kendisi indirecek."},
	"eng.nokey":        {"There's no Soloist key.", "Soloist anahtarı yok."},
	"eng.savekey":      {"Put it in the api_key line of: ", "Anahtarı şu dosyadaki api_key satırına yaz: "},
	"eng.expired":      {"This Soloist build has expired.", "Soloist'in süresi dolmuş."},
	"eng.expired.how":  {"Each build runs for 90 days. A newer one is needed.", "Her sürüm 90 gün çalışıyor; yenisini indirmek gerekiyor."},
	"eng.busy":         {"Soloist is already running somewhere else.", "Soloist başka bir yerde zaten çalışıyor."},
	"eng.busy.how":     {"Close the other sotto window, then press o.", "Diğer sotto penceresini kapatıp o'ya bas."},
	"eng.broken":       {"Soloist isn't running.", "Soloist çalışmıyor."},
	"eng.log":          {"Log: ", "Kayıt: "},
	"eng.retry":        {"Press o to start Soloist again.", "Soloist'i yeniden başlatmak için o'ya bas."},
	"eng.disconnected": {"Lost the connection to Soloist.", "Soloist ile bağlantı koptu."},
	"eng.connecting":   {"Connecting to Soloist…", "Soloist'e bağlanılıyor…"},
	"eng.pair":         {"Waiting for pairing.", "Eşleşme bekleniyor."},
	"eng.pair.how":     {"In the Spotify app or on your phone, pick sotto from the device list.", "Spotify uygulamasında ya da telefonda cihaz listesinden sotto'yu seç."},
	"eng.pair.once":    {"You only do this once. sotto remembers it.", "Bu bir kerelik; sotto girişi hatırlar."},
	"eng.unknown":      {"unknown error", "bilinmeyen hata"},

	// alt şerit
	"strip.nothing": {"nothing playing", "çalan bir şey yok"},
	"strip.vol":     {"   vol ", "   ses "},

	// kısayollar
	"key.space":     {"space", "boşluk"},
	"hint.page":     {"page", "sayfa"},
	"hint.enter":    {"open", "gir"},
	"hint.select":   {"select", "seç"},
	"hint.play":     {"open/play", "aç/çal"},
	"hint.back":     {"back", "geri"},
	"hint.menu":     {"menu", "menü"},
	"hint.pause":    {"play/pause", "çal/durdur"},
	"hint.skip":     {"next/prev", "sonraki/önceki"},
	"hint.seek":     {"seek", "sar"},
	"hint.vol":      {"volume", "ses"},
	"hint.shuf":     {"shuffle", "karıştır"},
	"hint.rep":      {"repeat", "tekrar"},
	"hint.solo":     {"Soloist", "Soloist"},
	"hint.switch":   {"speaker", "hoparlör"},
	"hint.keys":     {"keys", "kısayollar"},
	"hint.close":    {"close", "kapat"},
	"hint.type":     {"type", "yaz"},
	"hint.tosearch": {"to search", "ara"},
	"hint.filter":   {"filter", "süzgeç"},
	"hint.results":  {"results", "sonuçlar"},

	// ana sayfa
	"home.jump":       {"Jump back in", "Kaldığın yerden devam"},
	"home.recent":     {"Recently played", "Son çalınanlar"},
	"home.toptracks":  {"Your top songs this month", "Bu ay en çok dinlediklerin"},
	"home.topartists": {"Your top artists this month", "Bu ayın sanatçıları"},
	"home.empty":      {"Nothing here yet. Play something and it shows up here.", "Burada henüz bir şey yok. Bir şey çal, burada görünür."},
	"home.greet":      {"%s, %s.", "%s, %s."},
	"greet.morning":   {"Good morning", "Günaydın"},
	"greet.afternoon": {"Good afternoon", "İyi günler"},
	"greet.evening":   {"Good evening", "İyi akşamlar"},
	"greet.night":     {"Good night", "İyi geceler"},
	"ago.now":         {"just now", "az önce"},
	"ago.min":         {"%d min ago", "%d dk önce"},
	"ago.hour":        {"%d h ago", "%d sa önce"},
	"ago.yesterday":   {"yesterday", "dün"},
	"ago.days":        {"%d days ago", "%d gün önce"},

	// ara
	"search.all":          {"All", "Hepsi"},
	"search.songs":        {"Songs", "Şarkılar"},
	"search.artists":      {"Artists", "Sanatçılar"},
	"search.albums":       {"Albums", "Albümler"},
	"search.playlists":    {"Playlists", "Listeler"},
	"search.shows":        {"Podcasts", "Podcast'ler"},
	"search.episodes":     {"Episodes", "Bölümler"},
	"search.episodecount": {"%d episodes", "%d bölüm"},
	"search.hint":         {"What do you want to listen to?", "Ne dinlemek istersin?"},
	"search.empty":        {"Type to search. Tab changes the filter.", "Aramak için yaz. Tab süzgeci değiştirir."},
	"search.none":         {"Nothing found.", "Bir şey bulunamadı."},

	// ? ile açılan liste
	"help.title":   {"Keys", "Kısayollar"},
	"help.move":    {"move", "gezin"},
	"help.open":    {"open, play", "aç, çal"},
	"help.back":    {"back", "geri"},
	"help.tab":     {"switch between menu and page", "menü ile sayfa arasında geç"},
	"help.pages":   {"go to a page", "sayfaya git"},
	"help.pause":   {"play, pause", "çal, durdur"},
	"help.skip":    {"next, previous", "sonraki, önceki"},
	"help.seek":    {"back, forward 10 seconds", "10 saniye geri, ileri"},
	"help.vol":     {"volume", "ses"},
	"help.shuf":    {"shuffle", "karıştır"},
	"help.rep":     {"repeat: off, playlist, song", "tekrar: kapalı, liste, şarkı"},
	"help.switch":  {"move the music between Soloist and your other device", "müziği Soloist ile diğer cihazın arasında taşı"},
	"help.solo":    {"start or stop Soloist", "Soloist'i aç ya da kapat"},
	"help.keys":    {"this list", "bu liste"},
	"help.search":  {"search", "ara"},
	"help.actions": {"actions: queue, like, add to playlist…", "eylemler: sıraya ekle, beğen, listeye ekle…"},
	"hint.do":      {"do", "yap"},

	// eylem menüsü
	"act.nothing":       {"Nothing selected and nothing playing.", "Seçili ya da çalan bir şey yok."},
	"act.queue":         {"Add to queue", "Sıraya ekle"},
	"act.like":          {"Add to Liked Songs", "Beğenilenlere ekle"},
	"act.unlike":        {"Remove from Liked Songs", "Beğenilenlerden çıkar"},
	"act.addlist":       {"Add to playlist…", "Listeye ekle…"},
	"act.removelist":    {"Remove from this playlist", "Bu listeden çıkar"},
	"act.album":         {"Go to album", "Albüme git"},
	"act.artist":        {"Play the artist", "Sanatçıyı çal"},
	"act.open":          {"Open", "Aç"},
	"act.play":          {"Play", "Çal"},
	"act.save":          {"Save to your library", "Kitaplığına kaydet"},
	"act.unsave":        {"Remove from your library", "Kitaplığından çıkar"},
	"act.follow":        {"Follow", "Takip et"},
	"act.unfollow":      {"Unfollow", "Takibi bırak"},
	"act.link":          {"Copy link", "Bağlantıyı kopyala"},
	"act.addlist.title": {"Add to playlist", "Listeye ekle"},
	"act.nolists":       {"You have no playlists you can edit.", "Düzenleyebileceğin bir listen yok."},
	"act.queued":        {"Added to queue", "Sıraya eklendi"},
	"act.liked":         {"Added to Liked Songs", "Beğenilenlere eklendi"},
	"act.unliked":       {"Removed from Liked Songs", "Beğenilenlerden çıkarıldı"},
	"act.saved":         {"Saved to your library", "Kitaplığına kaydedildi"},
	"act.unsaved":       {"Removed from your library", "Kitaplığından çıkarıldı"},
	"act.added":         {"Added to %s", "%s listesine eklendi"},
	"act.removed":       {"Removed from %s", "%s listesinden çıkarıldı"},
	"act.copied":        {"Link copied", "Bağlantı kopyalandı"},
	"kind.track":        {"song", "şarkı"},
	"kind.episode":      {"episode", "bölüm"},
	"kind.liked":        {"liked songs", "beğenilenler"},
	"help.lyrics":       {"lyrics; press again for up next", "sözler; tekrar basınca sıradakiler"},
	"help.lyrlogin":     {"log in for Spotify's lyrics (when asked)", "Spotify sözleri için giriş (istenirse)"},

	// sözler
	"lyr.loading":   {"Looking for lyrics…", "Sözler aranıyor…"},
	"lyr.none":      {"No lyrics for this song.", "Bu şarkının sözleri yok."},
	"lyr.login":     {"Spotify's lyrics need a one-time login.", "Spotify'ın sözleri için bir kerelik giriş gerekiyor."},
	"lyr.login.how": {"Press L to open it in your browser.", "Tarayıcıda açmak için L'ye bas."},
	"lyr.source":    {"lyrics · ", "sözler · "},
	"lyr.opened":    {"Spotify's login page opened in your browser.", "Spotify'ın giriş sayfası tarayıcında açıldı."},
	"help.quit":     {"quit", "çık"},

	// cihazlar
	"dev.sub":           {"Where the music plays. Press enter to move it.", "Müziğin çaldığı yer. Taşımak için enter'a bas."},
	"dev.kind.light":    {"light mode", "hafif mod"},
	"dev.kind.off":      {"light mode · off, enter starts it", "hafif mod · kapalı, enter açar"},
	"dev.kind.starting": {"light mode · starting…", "hafif mod · açılıyor…"},
	"dev.help":          {"t switches between Soloist and your last device. o starts or stops Soloist.", "t Soloist ile son cihaz arasında geçer. o Soloist'i açar ya da kapatır."},
	"hint.quit":         {"quit", "çık"},

	// kitaplık
	"lib.liked":        {"Liked Songs", "Beğenilenler"},
	"lib.playlists":    {"%d playlists", "%d çalma listesi"},
	"lib.plus.liked":   {" · liked songs", " · beğenilenler"},
	"lib.songs":        {"%d songs", "%d şarkı"},
	"lib.songs.short":  {" · %d songs", " · %d şarkı"},
	"lib.liked.sub":    {"songs you liked", "beğendiğin şarkılar"},
	"lib.loading":      {"loading…", "yükleniyor…"},
	"lib.noclient":     {"No Spotify app is set up.", "Spotify uygulaması ayarlanmamış."},
	"lib.noclient.how": {"Put your Client ID in: ", "Client ID'yi şu dosyaya yaz: "},
	"lib.loggedout":    {"Your Spotify account isn't connected.", "Spotify hesabın bağlı değil."},
	"lib.login.how":    {"Press enter to connect. Spotify's permission page opens in your browser.", "Bağlamak için enter'a bas. Tarayıcıda Spotify'ın izin sayfası açılır."},
	"lib.waiting":      {"Waiting for you in the browser…", "Tarayıcıda onay bekleniyor…"},
	"lib.waiting.how":  {"Once you allow it on Spotify's page, sotto picks it up by itself.", "Spotify'ın sayfasında izin verince sotto kendiliğinden devam eder."},

	// kısa notlar (alt satır)
	"flash.nosoloist":  {"Soloist isn't connected", "Soloist bağlı değil"},
	"flash.nodevice":   {"No device to play on. Start Soloist (o) or open Spotify somewhere.", "Çalacak cihaz yok. Soloist'i aç (o) ya da Spotify'ı bir yerde aç."},
	"flash.elsewhere":  {"Soloist is off, so it's playing on %s.", "Soloist kapalı, bu yüzden %s üzerinde çalıyor."},
	"flash.attached":   {"This Soloist belongs to another sotto window. Close it there.", "Bu Soloist başka bir sotto penceresinin. Onu oradan kapat."},
	"flash.pickdevice": {"Pick where the music should go.", "Müziğin nereye gideceğini seç."},
	"flash.moved":      {"Now playing on %s", "Artık %s üzerinde çalıyor"},
	"flash.noaccount":  {"Connect your Spotify account in Library first.", "Önce Kitaplık'tan Spotify hesabını bağla."},

	// tarayıcıdaki giriş sayfası
	"web.done":   {"All set. You can close this tab and go back to sotto.", "Giriş tamam. Bu sekmeyi kapatıp sotto'ya dönebilirsin."},
	"web.denied": {"Permission wasn't given. Go back to sotto to try again.", "İzin verilmedi. sotto'ya dönüp tekrar deneyebilirsin."},
	"web.stale":  {"This link belongs to an older login. Go back to sotto and try again.", "Bu bağlantı eski bir girişe ait. sotto'ya dönüp tekrar dene."},

	// türler
	"kind.playlist":   {"playlist", "çalma listesi"},
	"kind.album":      {"album", "albüm"},
	"kind.artist":     {"artist", "sanatçı"},
	"kind.show":       {"podcast", "podcast"},
	"kind.collection": {"liked songs", "beğenilenler"},
}
