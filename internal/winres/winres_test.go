package winres

import (
	"bytes"
	"encoding/binary"
	"testing"
	"unicode/utf16"
)

func utf16le(s string) []byte {
	var b bytes.Buffer
	for _, c := range utf16.Encode([]rune(s)) {
		binary.Write(&b, binary.LittleEndian, c)
	}
	return b.Bytes()
}

func TestSyso(t *testing.T) {
	info := Info{Icons: [][]byte{[]byte("png16"), []byte("png256")}, IconSizes: []int{16, 256}, Name: "sotto", Version: "0.1.0", Filename: "sotto.exe"}
	for arch, machine := range map[Arch]uint16{AMD64: 0x8664, ARM64: 0xAA64} {
		b, err := Syso(arch, info)
		if err != nil {
			t.Fatal(err)
		}
		le := binary.LittleEndian
		if le.Uint16(b) != machine || le.Uint16(b[2:]) != 1 || string(b[20:25]) != ".rsrc" {
			t.Fatalf("%s: bad header", arch)
		}
		if n := le.Uint16(b[20+32:]); n != 4 { // 2 simge + grup + sürüm
			t.Fatalf("%s: %d relocations", arch, n)
		}
		for _, want := range [][]byte{[]byte("png256"), utf16le("FileDescription"), utf16le("sotto.exe"), utf16le("VS_VERSION_INFO")} {
			if !bytes.Contains(b, want) {
				t.Fatalf("%s: lacks %q", arch, want)
			}
		}
	}
	if _, err := Syso(AMD64, Info{Icons: [][]byte{nil}, IconSizes: nil}); err == nil {
		t.Fatal("mismatched sizes accepted")
	}
	if _, err := Syso(AMD64, Info{Version: "1.x"}); err == nil {
		t.Fatal("bad version accepted")
	}
}
