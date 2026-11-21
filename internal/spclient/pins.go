package spclient

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
)

// Sabitlenmiş öğeler (✱): Spotify uygulamalarında Kitaplık'ın üstüne
// iğnelenen listeler, albümler, sanatçılar. Spotify bunları collection v2'de
// "ylpin" kümesinde tutar; yalnızca protobuf konuşur. Yalnız okunur: bu
// kümeye yazmayı Spotify kabul edip yok sayıyor (trill'in bulgusu).
//
//	PageRequest  { 1: username, 2: set, 3: page token, 4: limit }
//	PageResponse { 1: repeated Item { 1: uri, 2: added_at (sn), 3: is_removed }, 2: next page token }

// Pin, sabitlenmiş bir öğe.
type Pin struct {
	URI     string
	AddedAt int64 // sn; en yeni sabitlenen önce
}

// Pins, kullanıcının sabitledikleri (en son sabitlenen önce).
func Pins(ctx context.Context, token, user string) ([]Pin, error) {
	return collectionSet(ctx, token, user, "ylpin")
}

func collectionSet(ctx context.Context, token, user, set string) ([]Pin, error) {
	var out []Pin
	page := ""
	for i := 0; i < 50; i++ {
		var body []byte
		body = pbString(body, 1, user)
		body = pbString(body, 2, set)
		if page != "" {
			body = pbString(body, 3, page)
		}
		body = pbVarint(body, 4, 2000)
		resp, err := postProto(ctx, token, "/collection/v2/paging", "application/vnd.collection-v2.spotify.proto", body)
		if err != nil {
			return nil, err
		}
		items, next, err := parsePage(resp)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
		if next == "" {
			break
		}
		page = next
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].AddedAt > out[j].AddedAt })
	return out, nil
}

func postProto(ctx context.Context, token, path, ctype string, body []byte) ([]byte, error) {
	return sendProto(ctx, token, http.MethodPost, path, ctype, body)
}

func sendProto(ctx context.Context, token, method, path, ctype string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, base+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", ctype)
	req.Header.Set("Accept", ctype)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent:
	case http.StatusUnauthorized:
		return nil, ErrUnauthorized
	default:
		return nil, fmt.Errorf("spotify: %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func parsePage(b []byte) (items []Pin, next string, err error) {
	err = pbEach(b, func(field int, wire int, v uint64, data []byte) error {
		switch {
		case field == 1 && wire == 2:
			var p Pin
			removed := false
			if err := pbEach(data, func(f, w int, v uint64, d []byte) error {
				switch {
				case f == 1 && w == 2:
					p.URI = string(d)
				case f == 2 && w == 0:
					p.AddedAt = int64(v)
				case f == 3 && w == 0:
					removed = v != 0
				}
				return nil
			}); err != nil {
				return err
			}
			if !removed && p.URI != "" {
				items = append(items, p)
			}
		case field == 2 && wire == 2:
			next = string(data)
		}
		return nil
	})
	return items, next, err
}

// ---- en küçük protobuf ----

var errProto = errors.New("spclient: bad protobuf")

func pbVarintRaw(b []byte, v uint64) []byte { return binary.AppendUvarint(b, v) }

func pbString(b []byte, field int, s string) []byte {
	b = pbVarintRaw(b, uint64(field)<<3|2)
	b = pbVarintRaw(b, uint64(len(s)))
	return append(b, s...)
}

func pbBytes(b []byte, field int, d []byte) []byte { return pbString(b, field, string(d)) }

func pbVarint(b []byte, field int, v uint64) []byte {
	b = pbVarintRaw(b, uint64(field)<<3)
	return pbVarintRaw(b, v)
}

// pbEach, mesajın alanlarını sırayla verir: varint (wire 0) için v, uzunluklu
// (wire 2) için data; 32/64 bitlikler atlanır.
func pbEach(b []byte, fn func(field, wire int, v uint64, data []byte) error) error {
	for len(b) > 0 {
		tag, n := binary.Uvarint(b)
		if n <= 0 {
			return errProto
		}
		b = b[n:]
		field, wire := int(tag>>3), int(tag&7)
		switch wire {
		case 0:
			v, n := binary.Uvarint(b)
			if n <= 0 {
				return errProto
			}
			b = b[n:]
			if err := fn(field, wire, v, nil); err != nil {
				return err
			}
		case 2:
			l, n := binary.Uvarint(b)
			if n <= 0 || uint64(len(b)-n) < l {
				return errProto
			}
			data := b[n : n+int(l)]
			b = b[n+int(l):]
			if err := fn(field, wire, 0, data); err != nil {
				return err
			}
		case 1:
			if len(b) < 8 {
				return errProto
			}
			b = b[8:]
		case 5:
			if len(b) < 4 {
				return errProto
			}
			b = b[4:]
		default:
			return errProto
		}
	}
	return nil
}
