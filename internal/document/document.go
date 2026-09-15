package document

import (
	"crypto/sha256"
	"errors"
	"sort"
	"sync"
	"unicode/utf8"

	textencoding "github.com/itishrishikesh/lumeleaf/internal/encoding"
)

var (
	ErrInvalidEdit = errors.New("invalid edit range or UTF-8 boundary")
	ErrReadOnly    = errors.New("document is read-only")
)

// LineEnding is the preferred ending used for inserted newlines. Existing
// endings remain byte-for-byte intact, so mixed-ending source is not rewritten.
type LineEnding string

const (
	LF   LineEnding = "LF"
	CRLF LineEnding = "CRLF"
)

type Metadata struct {
	Encoding         textencoding.Provenance
	LineEnding       LineEnding
	MixedEndings     bool
	FinalNewline     bool
	ContainsNUL      bool
	PathologicalLine bool
}

// Position is a zero-based LSP-compatible line/UTF-16-column coordinate.
type Position struct{ Line, UTF16Column int }

// ByteEdit replaces [Start, End) with Text. Ranges refer to the same revision
// and are applied atomically, making it suitable for multi-cursor edits.
type ByteEdit struct {
	Start, End int
	Text       string
}

// Change is a compact record of a transaction for incremental parsers.
type Change struct{ Start, OldEnd, NewEnd int }

type transaction struct {
	before, after []byte
	changes       []Change
}

type Document struct {
	mu         sync.RWMutex
	rope       *Rope
	meta       Metadata
	revision   uint64
	undo, redo []transaction
	log        []revisionChange
	readOnly   bool
	disk       Fingerprint
}
type revisionChange struct {
	revision uint64
	changes  []Change
}

func New(text []byte) (*Document, error) {
	r, err := NewRope(text)
	if err != nil {
		return nil, err
	}
	return &Document{rope: r, meta: inspect(text, textencoding.Provenance{Kind: textencoding.UTF8})}, nil
}

func NewDecoded(text []byte, provenance textencoding.Provenance) (*Document, error) {
	r, err := NewRope(text)
	if err != nil {
		return nil, err
	}
	return &Document{rope: r, meta: inspect(text, provenance)}, nil
}

func (d *Document) Revision() uint64   { d.mu.RLock(); defer d.mu.RUnlock(); return d.revision }
func (d *Document) Metadata() Metadata { d.mu.RLock(); defer d.mu.RUnlock(); return d.meta }
func (d *Document) Text() []byte       { d.mu.RLock(); defer d.mu.RUnlock(); return d.rope.Bytes() }
func (d *Document) ByteLen() int       { d.mu.RLock(); defer d.mu.RUnlock(); return d.rope.ByteLen() }
func (d *Document) RuneLen() int       { d.mu.RLock(); defer d.mu.RUnlock(); return d.rope.RuneLen() }
func (d *Document) UTF16Len() int      { d.mu.RLock(); defer d.mu.RUnlock(); return d.rope.UTF16Len() }
func (d *Document) ReadOnly() bool     { d.mu.RLock(); defer d.mu.RUnlock(); return d.readOnly }

func (d *Document) SetReadOnly(value bool) { d.mu.Lock(); defer d.mu.Unlock(); d.readOnly = value }

// Apply atomically validates every edit against the pre-edit revision before
// changing text. Overlapping selections are rejected rather than guessed at.
func (d *Document) Apply(edits []ByteEdit) ([]Change, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.readOnly {
		return nil, ErrReadOnly
	}
	if len(edits) == 0 {
		return nil, nil
	}
	before := d.rope.Bytes()
	sorted := append([]ByteEdit(nil), edits...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Start < sorted[j].Start })
	for i, e := range sorted {
		if e.Start < 0 || e.End < e.Start || e.End > len(before) || !utf8.ValidString(e.Text) || !isBoundary(before, e.Start) || !isBoundary(before, e.End) || (i > 0 && sorted[i-1].End > e.Start) {
			return nil, ErrInvalidEdit
		}
	}
	changes := make([]Change, len(sorted))
	for i := len(sorted) - 1; i >= 0; i-- {
		e := sorted[i]
		insert := []byte(e.Text)
		if d.meta.LineEnding == CRLF {
			insert = expandNewlines(insert)
		}
		changes[i] = Change{Start: e.Start, OldEnd: e.End, NewEnd: e.Start + len(insert)}
		if err := d.rope.replace(e.Start, e.End, insert); err != nil {
			return nil, err
		}
	}
	after := d.rope.Bytes()
	d.undo = append(d.undo, transaction{before: before, after: after, changes: changes})
	d.redo = nil
	d.revision++
	d.log = append(d.log, revisionChange{d.revision, changes})
	d.meta = inspect(after, d.meta.Encoding)
	return append([]Change(nil), changes...), nil
}

func (d *Document) Undo() ([]Change, error) { return d.history(false) }
func (d *Document) Redo() ([]Change, error) { return d.history(true) }
func (d *Document) history(redo bool) ([]Change, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.readOnly {
		return nil, ErrReadOnly
	}
	var from, to *[]transaction
	if redo {
		from, to = &d.redo, &d.undo
	} else {
		from, to = &d.undo, &d.redo
	}
	if len(*from) == 0 {
		return nil, nil
	}
	t := (*from)[len(*from)-1]
	*from = (*from)[:len(*from)-1]
	*to = append(*to, t)
	text := t.before
	if redo {
		text = t.after
	}
	d.rope.set(text)
	d.revision++
	d.meta = inspect(text, d.meta.Encoding)
	changes := append([]Change(nil), t.changes...)
	d.log = append(d.log, revisionChange{d.revision, changes})
	return changes, nil
}

// ChangesSince returns a copy of edits made after revision. The boolean is
// false if the caller is already current; log compaction has not occurred.
func (d *Document) ChangesSince(revision uint64) ([]Change, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if revision >= d.revision {
		return nil, false
	}
	var out []Change
	for _, e := range d.log {
		if e.revision > revision {
			out = append(out, e.changes...)
		}
	}
	return out, true
}

func (d *Document) LineCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return lineCount(d.rope.Bytes())
}

// ByteAtRune maps a zero-based rune index to a byte offset.
func (d *Document) ByteAtRune(index int) (int, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if index < 0 {
		return 0, ErrInvalidEdit
	}
	text := d.rope.Bytes()
	for byteOffset, runeIndex := 0, 0; ; {
		if runeIndex == index {
			return byteOffset, nil
		}
		if byteOffset == len(text) {
			return 0, ErrInvalidEdit
		}
		_, size := utf8.DecodeRune(text[byteOffset:])
		byteOffset += size
		runeIndex++
	}
}

// RuneAtByte maps a byte offset at a UTF-8 boundary to a zero-based rune
// index. It is intentionally strict about offsets inside multi-byte runes.
func (d *Document) RuneAtByte(offset int) (int, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	text := d.rope.Bytes()
	if offset < 0 || offset > len(text) || !isBoundary(text, offset) {
		return 0, ErrInvalidEdit
	}
	return utf8.RuneCount(text[:offset]), nil
}

// ByteAtUTF16 maps a zero-based UTF-16 code-unit offset to a byte offset.
// The middle of a non-BMP character has no representable byte coordinate.
func (d *Document) ByteAtUTF16(index int) (int, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if index < 0 {
		return 0, ErrInvalidEdit
	}
	text := d.rope.Bytes()
	units := 0
	for offset := 0; ; {
		if units == index {
			return offset, nil
		}
		if offset == len(text) {
			return 0, ErrInvalidEdit
		}
		r, size := utf8.DecodeRune(text[offset:])
		width := 1
		if r > 0xffff {
			width = 2
		}
		if units+width > index {
			return 0, ErrInvalidEdit
		}
		units += width
		offset += size
	}
}

// LineStartByte maps a physical line number to its byte start.
func (d *Document) LineStartByte(line int) (int, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if line < 0 {
		return 0, ErrInvalidEdit
	}
	if line == 0 {
		return 0, nil
	}
	text := d.rope.Bytes()
	seen := 0
	for i, b := range text {
		if b == '\n' {
			seen++
			if seen == line {
				return i + 1, nil
			}
		}
	}
	return 0, ErrInvalidEdit
}

// PositionAtByte converts a valid byte offset to an LSP position.
func (d *Document) PositionAtByte(offset int) (Position, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	text := d.rope.Bytes()
	if offset < 0 || offset > len(text) || !isBoundary(text, offset) {
		return Position{}, ErrInvalidEdit
	}
	line, col, i := 0, 0, 0
	for i < offset {
		if text[i] == '\n' {
			line++
			col = 0
			i++
			continue
		}
		r, n := utf8.DecodeRune(text[i:])
		if r > 0xffff {
			col += 2
		} else {
			col++
		}
		i += n
	}
	return Position{line, col}, nil
}

// ByteAtPosition converts an LSP position. A column that splits a surrogate
// pair or extends beyond its line is invalid.
func (d *Document) ByteAtPosition(pos Position) (int, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	text := d.rope.Bytes()
	if pos.Line < 0 || pos.UTF16Column < 0 {
		return 0, ErrInvalidEdit
	}
	line, col, i := 0, 0, 0
	for i < len(text) {
		if line == pos.Line && col == pos.UTF16Column {
			return i, nil
		}
		if text[i] == '\n' {
			if line == pos.Line {
				break
			}
			line++
			col = 0
			i++
			continue
		}
		r, n := utf8.DecodeRune(text[i:])
		width := 1
		if r > 0xffff {
			width = 2
		}
		if line == pos.Line && col+width > pos.UTF16Column {
			break
		}
		col += width
		i += n
	}
	if line == pos.Line && col == pos.UTF16Column {
		return i, nil
	}
	return 0, ErrInvalidEdit
}

func lineCount(text []byte) int {
	n := 1
	for _, b := range text {
		if b == '\n' {
			n++
		}
	}
	return n
}
func expandNewlines(in []byte) []byte {
	out := make([]byte, 0, len(in))
	for i, b := range in {
		if b == '\n' && (i == 0 || in[i-1] != '\r') {
			out = append(out, '\r')
		}
		out = append(out, b)
	}
	return out
}

func inspect(text []byte, provenance textencoding.Provenance) Metadata {
	m := Metadata{Encoding: provenance, LineEnding: LF, FinalNewline: len(text) > 0 && text[len(text)-1] == '\n'}
	lf, crlf, start, longest := 0, 0, 0, 0
	for i, b := range text {
		if b == 0 {
			m.ContainsNUL = true
		}
		if b == '\n' {
			if i > 0 && text[i-1] == '\r' {
				crlf++
			} else {
				lf++
			}
			if i-start > longest {
				longest = i - start
			}
			start = i + 1
		}
	}
	if len(text)-start > longest {
		longest = len(text) - start
	}
	m.PathologicalLine = longest > 1024*1024
	if crlf > lf {
		m.LineEnding = CRLF
	}
	m.MixedEndings = crlf > 0 && lf > 0
	return m
}

// Fingerprint is a content identity captured when a file is opened/saved.
type Fingerprint struct {
	Size    int64
	ModTime int64
	Hash    [sha256.Size]byte
	Valid   bool
}
