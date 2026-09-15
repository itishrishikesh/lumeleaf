package encoding

import (
	"bytes"
	"testing"
)

func TestDecodeEncodeRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
		kind Kind
	}{
		{"utf8", []byte("héllo"), "héllo", UTF8},
		{"utf8 bom", []byte{0xef, 0xbb, 0xbf, 'x'}, "x", UTF8BOM},
		{"utf16 le", []byte{0xff, 0xfe, 'A', 0, 0x3d, 0xd8, 0, 0xde}, "A😀", UTF16LE},
		{"utf16 be", []byte{0xfe, 0xff, 0, 'A', 0xd8, 0x3d, 0xde, 0}, "A😀", UTF16BE},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, p, err := Decode(tc.data)
			if err != nil || string(got) != tc.want || p.Kind != tc.kind {
				t.Fatalf("Decode() = %q, %#v, %v", got, p, err)
			}
			back, err := Encode(got, p)
			if err != nil || !bytes.Equal(back, tc.data) {
				t.Fatalf("Encode() = %x, %v", back, err)
			}
		})
	}
}

func TestDecodeRejectsMalformed(t *testing.T) {
	for _, in := range [][]byte{{0xff}, {0xff, 0xfe, 0x00}, {0xff, 0xfe, 0x00, 0xdc}} {
		if _, _, err := Decode(in); err == nil {
			t.Fatalf("Decode(%x) unexpectedly succeeded", in)
		}
	}
}

func FuzzDecode(f *testing.F) {
	f.Add([]byte("plain text"))
	f.Add([]byte{0xff, 0xfe, 'x', 0})
	f.Add([]byte{0xff})
	f.Fuzz(func(t *testing.T, data []byte) {
		decoded, provenance, err := Decode(data)
		if err != nil {
			return
		}
		encoded, err := Encode(decoded, provenance)
		if err != nil || !bytes.Equal(encoded, data) {
			t.Fatalf("round trip failed: %x, %v", encoded, err)
		}
	})
}
