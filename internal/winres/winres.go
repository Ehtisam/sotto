// Package winres, Windows programına gömülen kaynakları (simge, sürüm
// bilgisi) Go bağlayıcısının okuduğu .syso nesne dosyası (COFF, .rsrc bölümü)
// olarak yazar. Dışarıdan araç gerekmez: go run ./tools/mkicon.
//
// Biçim: PE/COFF belgesindeki "The .rsrc Section". Kaynak ağacı üç düzeydir:
// tür → kimlik → dil; yapraklar veri girişleridir. Veri girişlerindeki
// adresler (RVA) bağlayıcının dolduracağı yeniden konumlandırmalarla bırakılır.
package winres

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

// Kaynak türleri.
const (
	rtIcon      = 3
	rtGroupIcon = 14
	rtVersion   = 16
)

const langEnUS = 0x0409

// Arch, hedef işlemci.
type Arch string

const (
	AMD64 Arch = "amd64"
	ARM64 Arch = "arm64"
)

// Info, gömülecek kaynaklar.
type Info struct {
	Icons       [][]byte // PNG'ler, küçükten büyüğe
	IconSizes   []int    // her PNG'nin kenarı (piksel)
	Name        string   // ürün adı ve açıklama (Görev Yöneticisi bunu gösterir)
	Version     string   // "0.1.0"
	Filename    string   // "sotto.exe"
	Description string
}

type resource struct {
	typ, id uint16
	data    []byte
}

// Syso, verilen işlemci için .syso dosyasının içeriğini kurar.
func Syso(arch Arch, info Info) ([]byte, error) {
	if len(info.Icons) != len(info.IconSizes) {
		return nil, fmt.Errorf("winres: %d icons, %d sizes", len(info.Icons), len(info.IconSizes))
	}
	var res []resource
	for i, p := range info.Icons {
		res = append(res, resource{rtIcon, uint16(i + 1), p})
	}
	if len(info.Icons) > 0 {
		res = append(res, resource{rtGroupIcon, 1, groupIcon(info)})
	}
	if info.Version != "" {
		v, err := versionInfo(info)
		if err != nil {
			return nil, err
		}
		res = append(res, resource{rtVersion, 1, v})
	}
	section, relocs := rsrcSection(res)

	var machine uint16
	var relType uint16
	switch arch {
	case AMD64:
		machine, relType = 0x8664, 0x0003 // IMAGE_REL_AMD64_ADDR32NB
	case ARM64:
		machine, relType = 0xAA64, 0x0002 // IMAGE_REL_ARM64_ADDR32NB
	default:
		return nil, fmt.Errorf("winres: unsupported arch %s", arch)
	}

	const fileHeader, sectionHeader = 20, 40
	rawAt := fileHeader + sectionHeader
	relocAt := rawAt + len(section)
	symAt := relocAt + 10*len(relocs)

	var b bytes.Buffer
	le := binary.LittleEndian
	w := func(v any) { _ = binary.Write(&b, le, v) }
	// IMAGE_FILE_HEADER
	w(machine)
	w(uint16(1))     // bölüm sayısı
	w(uint32(0))     // zaman damgası
	w(uint32(symAt)) // sembol tablosu
	w(uint32(1))     // sembol sayısı
	w(uint16(0))     // isteğe bağlı başlık yok
	w(uint16(0))     // özellikler
	// IMAGE_SECTION_HEADER
	b.WriteString(".rsrc\x00\x00\x00")
	w(uint32(0)) // VirtualSize
	w(uint32(0)) // VirtualAddress
	w(uint32(len(section)))
	w(uint32(rawAt))
	w(uint32(relocAt))
	w(uint32(0)) // satır numaraları
	w(uint16(len(relocs)))
	w(uint16(0))
	w(uint32(0x40000040)) // IMAGE_SCN_CNT_INITIALIZED_DATA | IMAGE_SCN_MEM_READ
	b.Write(section)
	// Yeniden konumlandırmalar: her veri girişinin RVA alanı .rsrc'nin başına göre.
	for _, off := range relocs {
		w(uint32(off))
		w(uint32(0)) // sembol 0: .rsrc
		w(relType)
	}
	// Sembol tablosu: tek sembol, bölümün başı.
	b.WriteString(".rsrc\x00\x00\x00")
	w(uint32(0))   // değer
	w(uint16(1))   // bölüm numarası
	w(uint16(0))   // tür
	b.WriteByte(3) // IMAGE_SYM_CLASS_STATIC
	b.WriteByte(0) // ek sembol yok
	// Dizgi tablosu (boş).
	w(uint32(4))
	return b.Bytes(), nil
}

// rsrcSection, kaynak ağacını ve verileri yazar; veri girişlerindeki RVA
// alanlarının bölüm içindeki yerlerini döner.
func rsrcSection(res []resource) ([]byte, []int) {
	sort.Slice(res, func(i, j int) bool {
		if res[i].typ != res[j].typ {
			return res[i].typ < res[j].typ
		}
		return res[i].id < res[j].id
	})
	var types []uint16
	byType := map[uint16][]resource{}
	for _, r := range res {
		if len(byType[r.typ]) == 0 {
			types = append(types, r.typ)
		}
		byType[r.typ] = append(byType[r.typ], r)
	}

	// Yerleşim: kök dizin, tür dizinleri, kimlik dizinleri (her biri tek dil
	// girişli), veri girişleri, veriler.
	dirSize := func(n int) int { return 16 + 8*n }
	off := dirSize(len(types))
	typeDirAt := map[uint16]int{}
	for _, t := range types {
		typeDirAt[t] = off
		off += dirSize(len(byType[t]))
	}
	langDirAt := make([]int, len(res))
	for i := range res {
		langDirAt[i] = off
		off += dirSize(1)
	}
	entryAt := make([]int, len(res))
	for i := range res {
		entryAt[i] = off
		off += 16
	}
	dataAt := make([]int, len(res))
	for i, r := range res {
		off = (off + 7) &^ 7
		dataAt[i] = off
		off += len(r.data)
	}

	buf := make([]byte, (off+7)&^7)
	le := binary.LittleEndian
	dir := func(at, n int) {
		le.PutUint16(buf[at+14:], uint16(n)) // kimlikli giriş sayısı
	}
	entry := func(at int, id uint32, target uint32) {
		le.PutUint32(buf[at:], id)
		le.PutUint32(buf[at+4:], target)
	}
	dir(0, len(types))
	idx := 0
	for i, t := range types {
		entry(16+8*i, uint32(t), 0x80000000|uint32(typeDirAt[t]))
		rs := byType[t]
		dir(typeDirAt[t], len(rs))
		for j, r := range rs {
			entry(typeDirAt[t]+16+8*j, uint32(r.id), 0x80000000|uint32(langDirAt[idx]))
			dir(langDirAt[idx], 1)
			entry(langDirAt[idx]+16, langEnUS, uint32(entryAt[idx]))
			idx++
		}
	}
	var relocs []int
	for i, r := range res {
		le.PutUint32(buf[entryAt[i]:], uint32(dataAt[i])) // RVA: bağlayıcı bölümün adresini ekler
		le.PutUint32(buf[entryAt[i]+4:], uint32(len(r.data)))
		relocs = append(relocs, entryAt[i])
		copy(buf[dataAt[i]:], r.data)
	}
	return buf, relocs
}

// groupIcon, RT_GROUP_ICON verisi (GRPICONDIR): simgenin boyutlarını ve
// RT_ICON kimliklerini listeler.
func groupIcon(info Info) []byte {
	var b bytes.Buffer
	le := binary.LittleEndian
	_ = binary.Write(&b, le, [3]uint16{0, 1, uint16(len(info.Icons))})
	for i, p := range info.Icons {
		n := info.IconSizes[i]
		dim := byte(n)
		if n >= 256 {
			dim = 0
		}
		b.Write([]byte{dim, dim, 0, 0})
		_ = binary.Write(&b, le, uint16(1))  // düzlem
		_ = binary.Write(&b, le, uint16(32)) // bit
		_ = binary.Write(&b, le, uint32(len(p)))
		_ = binary.Write(&b, le, uint16(i+1))
	}
	return b.Bytes()
}

// ---- sürüm bilgisi (VS_VERSIONINFO) ----

func wstr(s string) []byte {
	u := utf16.Encode([]rune(s + "\x00"))
	b := make([]byte, 2*len(u))
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[2*i:], c)
	}
	return b
}

func pad4(b *bytes.Buffer) {
	for b.Len()%4 != 0 {
		b.WriteByte(0)
	}
}

// block, sürüm bilgisinin bir düğümü: başlık, anahtar, değer, alt düğümler.
// text: değer metin (wValueLength karakter sayısıdır).
func block(key string, value []byte, text bool, children ...[]byte) []byte {
	var b bytes.Buffer
	b.Write([]byte{0, 0, 0, 0, 0, 0}) // wLength, wValueLength, wType: sonra
	b.Write(wstr(key))
	pad4(&b)
	b.Write(value)
	for _, c := range children {
		pad4(&b)
		b.Write(c)
	}
	out := b.Bytes()
	le := binary.LittleEndian
	le.PutUint16(out[0:], uint16(len(out)))
	vlen := len(value)
	if text {
		vlen /= 2
		le.PutUint16(out[4:], 1)
	}
	le.PutUint16(out[2:], uint16(vlen))
	return out
}

func parseVersion(v string) ([4]uint16, error) {
	var out [4]uint16
	parts := strings.Split(v, ".")
	if len(parts) > 4 {
		return out, fmt.Errorf("winres: bad version %q", v)
	}
	for i, p := range parts {
		n, err := strconv.ParseUint(p, 10, 16)
		if err != nil {
			return out, fmt.Errorf("winres: bad version %q", v)
		}
		out[i] = uint16(n)
	}
	return out, nil
}

func versionInfo(info Info) ([]byte, error) {
	v, err := parseVersion(info.Version)
	if err != nil {
		return nil, err
	}
	var fixed bytes.Buffer
	le := binary.LittleEndian
	ms, ls := uint32(v[0])<<16|uint32(v[1]), uint32(v[2])<<16|uint32(v[3])
	for _, x := range []uint32{
		0xFEEF04BD, 0x00010000, // imza, yapı sürümü
		ms, ls, ms, ls, // dosya ve ürün sürümü
		0x3F, 0, // bayrak maskesi, bayraklar
		0x00040004, // VOS_NT_WINDOWS32
		1, 0,       // VFT_APP
		0, 0, // tarih
	} {
		_ = binary.Write(&fixed, le, x)
	}
	desc := info.Description
	if desc == "" {
		desc = info.Name
	}
	var strs [][]byte
	for _, kv := range [][2]string{
		{"FileDescription", desc},
		{"FileVersion", info.Version},
		{"InternalName", info.Name},
		{"OriginalFilename", info.Filename},
		{"ProductName", info.Name},
		{"ProductVersion", info.Version},
	} {
		if kv[1] != "" {
			strs = append(strs, block(kv[0], wstr(kv[1]), true))
		}
	}
	table := block("040904B0", nil, true, strs...)
	sfi := block("StringFileInfo", nil, true, table)
	var tr bytes.Buffer
	_ = binary.Write(&tr, le, [2]uint16{langEnUS, 0x04B0}) // dil, Unicode
	vfi := block("VarFileInfo", nil, true, block("Translation", tr.Bytes(), false))
	return block("VS_VERSION_INFO", fixed.Bytes(), false, sfi, vfi), nil
}
