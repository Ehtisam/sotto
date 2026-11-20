package spclient

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Kitaplığın sırası ve klasörleri (✱): Spotify bunları "rootlist" adlı iç
// listede tutar. Klasör, düz listede bir çift işarettir:
//
//	spotify:start-group:{id}:{ad} … içindekiler … spotify:end-group:{id}
//
// Resmî Web API klasörleri hiç göstermez.

// ErrConflict: kitaplık bu arada başka yerde değişti; yeniden okunmalı.
var ErrConflict = errors.New("spclient: library changed meanwhile")

// RootItem, rootlist'teki bir satır.
type RootItem struct {
	URI       string
	Timestamp string
}

// Rootlist, kitaplığın düz hali ve sürümü (değişiklik gönderirken gerekir).
type Rootlist struct {
	Revision string
	Items    []RootItem
}

// GetRootlist, kullanıcının kitaplık sırasını okur.
func GetRootlist(ctx context.Context, token, user string) (Rootlist, error) {
	var out Rootlist
	for from := 0; ; {
		var r struct {
			Revision string `json:"revision"`
			Contents struct {
				Truncated bool `json:"truncated"`
				Items     []struct {
					URI        string `json:"uri"`
					Attributes struct {
						Timestamp string `json:"timestamp"`
					} `json:"attributes"`
				} `json:"items"`
			} `json:"contents"`
		}
		path := fmt.Sprintf("/playlist/v2/user/%s/rootlist?decorate=revision,attributes,length,owner&from=%d&length=500",
			url.PathEscape(user), from)
		if err := getJSON(ctx, token, path, &r); err != nil {
			return Rootlist{}, err
		}
		if from == 0 {
			out.Revision = r.Revision
		}
		for _, it := range r.Contents.Items {
			out.Items = append(out.Items, RootItem{URI: it.URI, Timestamp: it.Attributes.Timestamp})
		}
		if !r.Contents.Truncated || len(r.Contents.Items) == 0 {
			return out, nil
		}
		from += len(r.Contents.Items)
	}
}

// ---- klasörler ----

// Folder, bir klasör: işaretlerinin düz listedeki yerleri.
type Folder struct {
	ID, Name   string
	Start, End int // start-group ve end-group satırları
	Depth      int
}

// IsStart/IsEnd: satır klasör işareti mi.
func IsStart(uri string) bool { return strings.HasPrefix(uri, "spotify:start-group:") }
func IsEnd(uri string) bool   { return strings.HasPrefix(uri, "spotify:end-group:") }

// ParseStart, start-group satırından kimliği ve adı çıkarır.
func ParseStart(uri string) (id, name string) {
	rest := strings.TrimPrefix(uri, "spotify:start-group:")
	id, enc, _ := strings.Cut(rest, ":")
	name, err := url.QueryUnescape(enc)
	if err != nil {
		name = enc
	}
	return id, name
}

// Folders, düz listedeki klasörleri bulur (iç içe olabilir). Dengesiz
// işaretler yok sayılır.
func (r Rootlist) Folders() []Folder {
	var out []Folder
	var stack []int // out içindeki açık klasörler
	for i, it := range r.Items {
		switch {
		case IsStart(it.URI):
			id, name := ParseStart(it.URI)
			out = append(out, Folder{ID: id, Name: name, Start: i, End: -1, Depth: len(stack)})
			stack = append(stack, len(out)-1)
		case IsEnd(it.URI):
			id := strings.TrimPrefix(it.URI, "spotify:end-group:")
			for j := len(stack) - 1; j >= 0; j-- {
				if out[stack[j]].ID == id {
					out[stack[j]].End = i
					stack = stack[:j]
					break
				}
			}
		}
	}
	var ok []Folder
	for _, f := range out {
		if f.End > f.Start {
			ok = append(ok, f)
		}
	}
	return ok
}

// Parent, i'nci satırın içinde bulunduğu en yakın klasörün kimliği ("" kök).
func (r Rootlist) Parent(i int) string {
	best, depth := "", -1
	for _, f := range r.Folders() {
		if f.Start < i && i < f.End && f.Depth > depth {
			best, depth = f.ID, f.Depth
		}
	}
	return best
}

// Index, adresin düz listedeki yeri (-1 yoksa).
func (r Rootlist) Index(uri string) int {
	for i, it := range r.Items {
		if it.URI == uri {
			return i
		}
	}
	return -1
}

// FolderByID, kimliği verilen klasör.
func (r Rootlist) FolderByID(id string) (Folder, bool) {
	for _, f := range r.Folders() {
		if f.ID == id {
			return f, true
		}
	}
	return Folder{}, false
}

// ---- değişiklikler ----

// Op, rootlist'e bir değişiklik (Spotify web oynatıcısının JSON biçimi).
type Op map[string]any

func items(uris ...string) []map[string]any {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	out := make([]map[string]any, len(uris))
	for i, u := range uris {
		out[i] = map[string]any{"uri": u, "attributes": map[string]string{"timestamp": ts}}
	}
	return out
}

// AddAt, satırları index'e ekler.
func AddAt(index int, uris ...string) Op {
	return Op{"kind": "ADD", "add": map[string]any{"fromIndex": index, "items": items(uris...)}}
}

// Remove, from'dan başlayan n satırı çıkarır.
func Remove(from, n int) Op {
	return Op{"kind": "REM", "rem": map[string]any{"fromIndex": from, "length": n}}
}

// Move, from'dan başlayan n satırı to'nun önüne taşır (to, çıkarmadan önceki sıraya göre).
func Move(from, n, to int) Op {
	return Op{"kind": "MOV", "mov": map[string]any{"fromIndex": from, "length": n, "toIndex": to}}
}

// NewFolderID, yeni bir klasör kimliği (16 onaltılık hane).
func NewFolderID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// StartURI / EndURI, klasör işaretleri.
func StartURI(id, name string) string {
	return "spotify:start-group:" + id + ":" + url.QueryEscape(name)
}
func EndURI(id string) string { return "spotify:end-group:" + id }

// RenameOps, klasörün başlangıç işaretini yeni adla değiştirir (bitiş ve
// içindekiler yerinde kalır).
func RenameOps(f Folder, name string) []Op {
	return []Op{Remove(f.Start, 1), AddAt(f.Start, StartURI(f.ID, name))}
}

// DeleteFolderOps, klasörün işaretlerini siler; içindekiler bir üst düzeye çıkar.
func DeleteFolderOps(f Folder) []Op {
	return []Op{Remove(f.End, 1), Remove(f.Start, 1)}
}

// ChangeRootlist, değişiklikleri gönderir. Kitaplık bu arada değiştiyse
// ErrConflict döner (yeniden okuyup yeniden kurmak gerekir).
func ChangeRootlist(ctx context.Context, token, user, baseRevision string, ops []Op) error {
	body := map[string]any{
		"baseRevision": baseRevision,
		"deltas": []any{map[string]any{
			"ops":  ops,
			"info": map[string]any{"source": map[string]string{"client": "WEBPLAYER"}},
		}},
		"wantResultingRevisions": false,
		"wantSyncResult":         false,
		"nonces":                 []any{},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		base+"/playlist/v2/user/"+url.PathEscape(user)+"/rootlist/changes", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusConflict, 509:
		return ErrConflict
	}
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
	return fmt.Errorf("spotify library: %s %s", resp.Status, strings.TrimSpace(string(msg)))
}
