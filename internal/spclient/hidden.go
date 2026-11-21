package spclient

import (
	"context"
	"time"
)

// Gizlenen şarkılar (✱): Spotify uygulamalarındaki "Şarkıyı gizle". Spotify
// bunları collection v2'de "ban" kümesinde tutar; gizli şarkı listelerde soluk
// görünür ve çalarken atlanır. 21.11.2026'da gerçek hesapta denendi: buraya
// yazılan şarkı telefondaki Spotify'da gizli göründü, removed ile geri alındı.
//
//	WriteRequest { 1: username, 2: set, 3: repeated Item { 1: uri, 2: added_at (sn), 3: is_removed } }

const collectionType = "application/vnd.collection-v2.spotify.proto"

// Hidden, kullanıcının gizlediği şarkılar.
func Hidden(ctx context.Context, token, user string) ([]string, error) {
	items, err := collectionSet(ctx, token, user, "ban")
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.URI)
	}
	return out, nil
}

// SetHidden, şarkıyı gizler (hide) ya da gizlemeyi kaldırır.
func SetHidden(ctx context.Context, token, user, uri string, hide bool) error {
	_, err := postProto(ctx, token, "/collection/v2/write", collectionType, hiddenWriteBody(user, uri, hide, time.Now()))
	return err
}

func hiddenWriteBody(user, uri string, hide bool, now time.Time) []byte {
	var item []byte
	item = pbString(item, 1, uri)
	item = pbVarint(item, 2, uint64(now.Unix()))
	if !hide {
		item = pbVarint(item, 3, 1)
	}
	var body []byte
	body = pbString(body, 1, user)
	body = pbString(body, 2, "ban")
	return pbBytes(body, 3, item)
}
