//go:build linux || darwin

package largefile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMappedFileIndexAndClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.txt")
	if err := os.WriteFile(path, []byte("zero\nuno\r\ndos\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err = f.BuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	lines, bytes, done := f.IndexProgress()
	if lines != 4 || bytes != 14 || !done {
		t.Fatalf("progress %d %d %v", lines, bytes, done)
	}
	for _, tc := range []struct {
		n    int64
		want string
	}{{0, "zero"}, {1, "uno"}, {2, "dos"}, {3, ""}} {
		got, err := f.Line(tc.n)
		if err != nil || string(got) != tc.want {
			t.Fatalf("Line(%d)=%q,%v", tc.n, got, err)
		}
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = f.Segment(0, 1); !errors.Is(err, ErrClosed) {
		t.Fatalf("after close=%v", err)
	}
}

func TestMappedFileRejectsInvalidUTF8(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.txt")
	if err := os.WriteFile(path, []byte{0xff}, 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err = f.BuildIndex(context.Background()); !errors.Is(err, ErrInvalidUTF8) {
		t.Fatalf("BuildIndex=%v", err)
	}
}

func FuzzSparseIndex(f *testing.F) {
	f.Add([]byte("a\nb\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		path := filepath.Join(t.TempDir(), "input")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		mapped, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer mapped.Close()
		err = mapped.BuildIndex(context.Background())
		if err == nil {
			_, _, complete := mapped.IndexProgress()
			if !complete {
				t.Fatal("successful index is incomplete")
			}
		}
	})
}

func BenchmarkLargeFileViewportSegment(b *testing.B) {
	path := filepath.Join(b.TempDir(), "large.txt")
	data := make([]byte, 8*1024*1024)
	for i := range data {
		data[i] = 'x'
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		b.Fatal(err)
	}
	f, err := Open(path)
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := f.Segment(int64((i*4096)%len(data)), 4096)
		if err != nil {
			b.Fatal(err)
		}
	}
}
