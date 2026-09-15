package document

import (
	"bytes"
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"
)

func TestCoordinatesUnicodeAndLineEndings(t *testing.T) {
	d, err := New([]byte("a\r\né😀\nשלום"))
	if err != nil {
		t.Fatal(err)
	}
	if got := d.LineCount(); got != 3 {
		t.Fatalf("LineCount=%d", got)
	}
	for _, tc := range []struct {
		byte int
		pos  Position
	}{
		{0, Position{0, 0}}, {3, Position{1, 0}}, {len("a\r\né"), Position{1, 2}}, {len("a\r\né😀"), Position{1, 4}}, {len("a\r\né😀\n"), Position{2, 0}},
	} {
		got, err := d.PositionAtByte(tc.byte)
		if err != nil || got != tc.pos {
			t.Fatalf("PositionAtByte(%d)=%+v,%v want %+v", tc.byte, got, err, tc.pos)
		}
		back, err := d.ByteAtPosition(tc.pos)
		if err != nil || back != tc.byte {
			t.Fatalf("ByteAtPosition(%+v)=%d,%v want %d", tc.pos, back, err, tc.byte)
		}
	}
	if _, err := d.ByteAtPosition(Position{1, 3}); !errors.Is(err, ErrInvalidEdit) {
		t.Fatalf("surrogate interior: %v", err)
	}
	if m := d.Metadata(); !m.MixedEndings {
		t.Fatalf("metadata=%+v", m)
	}
	if got, err := d.ByteAtRune(5); err != nil || got != len("a\r\né") {
		t.Fatalf("ByteAtRune = %d, %v", got, err)
	}
	if got, err := d.RuneAtByte(len("a\r\né")); err != nil || got != 5 {
		t.Fatalf("RuneAtByte = %d, %v", got, err)
	}
	if got, err := d.ByteAtUTF16(5); err != nil || got != len("a\r\né") {
		t.Fatalf("ByteAtUTF16 = %d, %v", got, err)
	}
	if got, err := d.LineStartByte(2); err != nil || got != len("a\r\né😀\n") {
		t.Fatalf("LineStartByte = %d, %v", got, err)
	}
}

func TestApplyIsAtomicUndoRedo(t *testing.T) {
	d, _ := New([]byte("one two three"))
	if _, err := d.Apply([]ByteEdit{{0, 3, "ONE"}, {8, 13, "THREE"}}); err != nil {
		t.Fatal(err)
	}
	if got := string(d.Text()); got != "ONE two THREE" {
		t.Fatal(got)
	}
	if _, err := d.Apply([]ByteEdit{{0, 2, "x"}, {1, 3, "y"}}); !errors.Is(err, ErrInvalidEdit) {
		t.Fatalf("overlap=%v", err)
	}
	if got := string(d.Text()); got != "ONE two THREE" {
		t.Fatalf("atomicity lost: %q", got)
	}
	if _, err := d.Undo(); err != nil || string(d.Text()) != "one two three" {
		t.Fatalf("undo=%q %v", d.Text(), err)
	}
	if _, err := d.Redo(); err != nil || string(d.Text()) != "ONE two THREE" {
		t.Fatalf("redo=%q %v", d.Text(), err)
	}
}

func TestRopeEditsAgreeWithByteOracle(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	oracle := []byte("start")
	d, _ := New(oracle)
	for i := 0; i < 500; i++ {
		start := rng.Intn(len(oracle) + 1)
		end := start + rng.Intn(len(oracle)-start+1)
		insert := []byte([]string{"x", "é", "😀", "\n", ""}[rng.Intn(5)])
		// choose rune boundaries for the byte oracle.
		for start < len(oracle) && !isBoundary(oracle, start) {
			start++
		}
		for end < len(oracle) && !isBoundary(oracle, end) {
			end++
		}
		want := append(append(append([]byte(nil), oracle[:start]...), insert...), oracle[end:]...)
		if _, err := d.Apply([]ByteEdit{{start, end, string(insert)}}); err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		if got := d.Text(); !bytes.Equal(got, want) {
			t.Fatalf("iteration %d: got %q want %q", i, got, want)
		}
		oracle = want
	}
}

func TestSaveConflictAndSymlink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.txt")
	if err := os.WriteFile(path, []byte("one"), 0o640); err != nil {
		t.Fatal(err)
	}
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = d.Apply([]ByteEdit{{0, 3, "two"}}); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("outside"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err = d.Save(path, false); !errors.Is(err, ErrDiskConflict) {
		t.Fatalf("Save conflict=%v", err)
	}
	if err = d.Save(path, true); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "two" {
		t.Fatalf("saved %q", got)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o640 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
	link := filepath.Join(dir, "link.txt")
	if err = os.Symlink(path, link); err == nil {
		if err = d.Save(link, true); !errors.Is(err, ErrSymlinkSave) {
			t.Fatalf("symlink save=%v", err)
		}
	}
}

func FuzzAtomicEdits(f *testing.F) {
	f.Add("hello", uint8(0), uint8(3), "é")
	f.Fuzz(func(t *testing.T, initial string, a, b uint8, replacement string) {
		if !utf8.ValidString(initial) || !utf8.ValidString(replacement) {
			return
		}
		d, err := New([]byte(initial))
		if err != nil {
			t.Fatal(err)
		}
		start := int(a) % (len(initial) + 1)
		end := start + int(b)%(len(initial)-start+1)
		if !isBoundary([]byte(initial), start) || !isBoundary([]byte(initial), end) {
			before := d.Text()
			if _, err := d.Apply([]ByteEdit{{Start: start, End: end, Text: replacement}}); err == nil || !bytes.Equal(before, d.Text()) {
				t.Fatal("invalid byte edit was not rejected atomically")
			}
			return
		}
		if _, err := d.Apply([]ByteEdit{{Start: start, End: end, Text: replacement}}); err != nil {
			t.Fatal(err)
		}
	})
}

func BenchmarkRopeInsertMiddle(b *testing.B) {
	text := bytes.Repeat([]byte("abcdefghij\n"), 10000)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		d, _ := New(text)
		_, _ = d.Apply([]ByteEdit{{len(text) / 2, len(text) / 2, "insert"}})
	}
}
