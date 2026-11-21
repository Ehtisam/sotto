package connect

import (
	"errors"
	"strings"
)

// ErrNotFound: sotto'nun gösterdiği satır kümede bulunamadı (sıra bu arada değişti).
var ErrNotFound = errors.New("connect: the queue changed")

// Locate, sotto'nun gösterdiği sıradaki pos'uncu satırı (0'dan, işaret
// satırları sayılmadan) kümedeki satırlar arasında bulur. Önce uid'ye, sonra
// yerine (adres tutuyorsa), en son adrese bakar. Bulamazsa -1.
func Locate(next []Track, pos int, uri, uid string) int {
	if uid != "" && !strings.HasPrefix(uid, "spotify:") {
		for i, t := range next {
			if t.UID == uid {
				return i
			}
		}
	}
	n := 0
	for i, t := range next {
		if t.Delimiter() {
			continue
		}
		if n == pos {
			if t.URI == uri {
				return i
			}
			break
		}
		n++
	}
	for i, t := range next {
		if t.URI == uri {
			return i
		}
	}
	return -1
}

// Remove, i'nci satırı çıkarılmış yeni bir liste döner.
func Remove(next []Track, i int) []Track {
	out := make([]Track, 0, len(next))
	out = append(out, next[:i]...)
	return append(out, next[i+1:]...)
}

// Move, i'nci satırı d yönünde (−1 yukarı, +1 aşağı) bir sonraki şarkıyla
// yer değiştirir. Şarkı kendi bölümünden (sıraya eklenenler / listeden
// gelenler) çıkmaz; çıkacaksa ok=false.
func Move(next []Track, i, d int) (out []Track, ok bool) {
	j := i + d
	for j >= 0 && j < len(next) && next[j].Delimiter() {
		j += d
	}
	if j < 0 || j >= len(next) || next[j].Queued() != next[i].Queued() {
		return next, false
	}
	out = append([]Track(nil), next...)
	out[i], out[j] = out[j], out[i]
	return out, true
}

// ClearQueued, kullanıcının sıraya eklediklerini çıkarır; listeden/albümden
// gelenler ve işaret satırları kalır. Kaç şarkı çıktığını da döner.
func ClearQueued(next []Track) ([]Track, int) {
	out := make([]Track, 0, len(next))
	for _, t := range next {
		if !t.Delimiter() && t.Queued() {
			continue
		}
		out = append(out, t)
	}
	return out, len(next) - len(out)
}
