package spclient

import (
	"context"
	"net/http"
)

// Özel oturum (✱): Spotify uygulamalarının yaptığı gibi hesabın
// "private-session-duration" istemci ayarı yazılır: "21600" (6 saat) açar,
// "0" kapatır. Hesap çapında bir ayar; Soloist'te çalınanları da kapsar
// (arkadaş etkinliği, öneriler, Wrapped görmez). Spotify bu ayarı yalnız
// kendi uygulamalarına geri okutuyor; sotto yazabilir ama okuyamaz, bu
// yüzden ne zaman bittiğini kendisi tutar.
//
//	PutRequest { 1: key, 2: Value { 4: string } }

// PrivateSessionLength: Spotify özel oturumu bu kadar sonra kendisi bitirir.
const PrivateSessionLength = 6 * 60 * 60

// SetPrivateSession, özel oturumu açar ya da kapatır.
func SetPrivateSession(ctx context.Context, token string, on bool) error {
	v := "0"
	if on {
		v = "21600"
	}
	var value, body []byte
	value = pbString(value, 4, v)
	body = pbString(body, 1, "private-session-duration")
	body = pbBytes(body, 2, value)
	_, err := sendProto(ctx, token, http.MethodPut, "/clientsettings/api/v1/", "application/x-protobuf", body)
	return err
}

// Recommendations, seed'e (liste, albüm, sanatçı ya da şarkı) benzeyen
// şarkılar: Spotify'ın onun için ürettiği radyonun ilk sayfası.
func Recommendations(ctx context.Context, token, seed string) ([]string, error) {
	radio, err := Radio(ctx, token, seed)
	if err != nil {
		return nil, err
	}
	info, err := Context(ctx, token, radio)
	if err != nil {
		return nil, err
	}
	return info.Tracks, nil
}
