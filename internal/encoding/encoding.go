// Package encoding decodes the text encodings supported by Lumeleaf's document
// model.  It deliberately has no lossy fallback: callers must ask a user
// before opening an unsupported or malformed file as text.
package encoding

import (
	"bytes"
	"encoding/binary"
	"errors"
	"unicode/utf16"
	"unicode/utf8"
)

var ErrUnsupported = errors.New("unsupported or malformed text encoding")

// Kind is the on-disk encoding of a document.
type Kind uint8

const (
	UTF8 Kind = iota
	UTF8BOM
	UTF16LE
	UTF16BE
)

// Provenance records the encoding selected while opening a document. It is
// used when saving so opening a UTF-16 file never silently changes its format.
type Provenance struct {
	Kind Kind
}

// Decode converts BOM-detected UTF-8 or UTF-16 data to valid UTF-8.
func Decode(src []byte) ([]byte, Provenance, error) {
	switch {
	case bytes.HasPrefix(src, []byte{0xef, 0xbb, 0xbf}):
		body := src[3:]
		if !utf8.Valid(body) {
			return nil, Provenance{}, ErrUnsupported
		}
		return append([]byte(nil), body...), Provenance{Kind: UTF8BOM}, nil
	case bytes.HasPrefix(src, []byte{0xff, 0xfe}):
		return decodeUTF16(src[2:], binary.LittleEndian, UTF16LE)
	case bytes.HasPrefix(src, []byte{0xfe, 0xff}):
		return decodeUTF16(src[2:], binary.BigEndian, UTF16BE)
	default:
		if !utf8.Valid(src) {
			return nil, Provenance{}, ErrUnsupported
		}
		return append([]byte(nil), src...), Provenance{Kind: UTF8}, nil
	}
}

func decodeUTF16(src []byte, order binary.ByteOrder, kind Kind) ([]byte, Provenance, error) {
	if len(src)%2 != 0 {
		return nil, Provenance{}, ErrUnsupported
	}
	units := make([]uint16, len(src)/2)
	for i := range units {
		units[i] = order.Uint16(src[i*2:])
	}
	// utf16.Decode replaces bad surrogate pairs, which would make a corrupt
	// source file look valid. Validate first instead.
	for i := 0; i < len(units); i++ {
		u := units[i]
		if u >= 0xd800 && u <= 0xdbff {
			if i+1 == len(units) || units[i+1] < 0xdc00 || units[i+1] > 0xdfff {
				return nil, Provenance{}, ErrUnsupported
			}
			i++
		} else if u >= 0xdc00 && u <= 0xdfff {
			return nil, Provenance{}, ErrUnsupported
		}
	}
	return []byte(string(utf16.Decode(units))), Provenance{Kind: kind}, nil
}

// Encode converts valid UTF-8 to the document's original encoding.
func Encode(text []byte, provenance Provenance) ([]byte, error) {
	if !utf8.Valid(text) {
		return nil, ErrUnsupported
	}
	switch provenance.Kind {
	case UTF8:
		return append([]byte(nil), text...), nil
	case UTF8BOM:
		return append([]byte{0xef, 0xbb, 0xbf}, text...), nil
	case UTF16LE, UTF16BE:
		units := utf16.Encode([]rune(string(text)))
		out := make([]byte, 2+len(units)*2)
		if provenance.Kind == UTF16LE {
			out[0], out[1] = 0xff, 0xfe
			for i, u := range units {
				binary.LittleEndian.PutUint16(out[2+i*2:], u)
			}
		} else {
			out[0], out[1] = 0xfe, 0xff
			for i, u := range units {
				binary.BigEndian.PutUint16(out[2+i*2:], u)
			}
		}
		return out, nil
	default:
		return nil, ErrUnsupported
	}
}
