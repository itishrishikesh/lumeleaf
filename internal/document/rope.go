// Package document provides the editable, revisioned UTF-8 document model.
package document

import (
	"errors"
	"unicode/utf8"
)

const chunkSize = 32 * 1024

var ErrInvalidUTF8 = errors.New("document text is not valid UTF-8")

// Rope keeps text in moderately sized chunks. It intentionally does not expose
// chunks: callers operate on stable byte/rune/UTF-16 coordinates instead.
// Rebalancing is performed after every mutation, which keeps local edits from
// producing an unbounded number of tiny objects.
type Rope struct {
	chunks [][]byte
	bytes  int
}

func NewRope(text []byte) (*Rope, error) {
	if !utf8.Valid(text) {
		return nil, ErrInvalidUTF8
	}
	r := &Rope{}
	r.set(text)
	return r, nil
}

func (r *Rope) set(text []byte) {
	r.chunks = r.chunks[:0]
	r.bytes = len(text)
	for len(text) > 0 {
		n := chunkSize
		if n > len(text) {
			n = len(text)
		}
		// Never split the leading bytes of a UTF-8 sequence into a chunk.
		for n < len(text) && !utf8.RuneStart(text[n]) {
			n--
		}
		if n == 0 {
			n = len(text)
		}
		r.chunks = append(r.chunks, append([]byte(nil), text[:n]...))
		text = text[n:]
	}
}

func (r *Rope) Bytes() []byte {
	out := make([]byte, 0, r.bytes)
	for _, c := range r.chunks {
		out = append(out, c...)
	}
	return out
}
func (r *Rope) ByteLen() int  { return r.bytes }
func (r *Rope) RuneLen() int  { return utf8.RuneCount(r.Bytes()) }
func (r *Rope) UTF16Len() int { return countUTF16(r.Bytes()) }

func (r *Rope) replace(start, end int, insert []byte) error {
	if start < 0 || end < start || end > r.bytes || !utf8.Valid(insert) {
		return ErrInvalidEdit
	}
	b := r.Bytes()
	if !isBoundary(b, start) || !isBoundary(b, end) {
		return ErrInvalidEdit
	}
	out := make([]byte, 0, len(b)-end+start+len(insert))
	out = append(out, b[:start]...)
	out = append(out, insert...)
	out = append(out, b[end:]...)
	r.set(out)
	return nil
}

func isBoundary(text []byte, offset int) bool {
	return offset >= 0 && offset <= len(text) && (offset == len(text) || utf8.RuneStart(text[offset]))
}

func countUTF16(text []byte) int {
	n := 0
	for len(text) > 0 {
		r, size := utf8.DecodeRune(text)
		if r > 0xffff {
			n += 2
		} else {
			n++
		}
		text = text[size:]
	}
	return n
}
