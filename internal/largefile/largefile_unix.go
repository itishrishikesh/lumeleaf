//go:build linux || darwin

// Package largefile implements the bounded-memory read-only document backend.
package largefile

import (
	"context"
	"errors"
	"os"
	"sync"
	"syscall"
	"unicode/utf8"
)

const (
	LargeFileThreshold = 250 * 1024 * 1024
	SparseEveryLines   = 4096
	MaxSegmentBytes    = 1 * 1024 * 1024
)

var (
	ErrClosed      = errors.New("large file is closed")
	ErrBadOffset   = errors.New("invalid byte range")
	ErrInvalidUTF8 = errors.New("invalid UTF-8 in large file")
)

// File maps a read-only file. Indexing is deliberately separate from Open, so
// the first viewport is available before a potentially long newline scan.
type File struct {
	mu           sync.RWMutex
	indexMu      sync.Mutex
	f            *os.File
	data         []byte
	closed       bool
	anchors      []int64 // byte offsets for lines 0, 4096, 8192, ...
	indexedLines int64
	indexedBytes int64
	indexedDone  bool
}

func Open(path string) (*File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	lf := &File{f: f, anchors: []int64{0}}
	if info.Size() == 0 {
		lf.indexedDone = true
		return lf, nil
	}
	data, err := syscall.Mmap(int(f.Fd()), 0, int(info.Size()), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		f.Close()
		return nil, err
	}
	lf.data = data
	return lf, nil
}

func (f *File) Size() int64  { f.mu.RLock(); defer f.mu.RUnlock(); return int64(len(f.data)) }
func (f *File) Closed() bool { f.mu.RLock(); defer f.mu.RUnlock(); return f.closed }

// Segment returns a bounded copy suitable for a viewport cache. The mapping is
// never exposed directly, preventing use-after-Unmap after Close.
func (f *File) Segment(offset int64, length int) ([]byte, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.closed {
		return nil, ErrClosed
	}
	if offset < 0 || offset > int64(len(f.data)) || length < 0 {
		return nil, ErrBadOffset
	}
	if length > MaxSegmentBytes {
		length = MaxSegmentBytes
	}
	end := offset + int64(length)
	if end > int64(len(f.data)) {
		end = int64(len(f.data))
	}
	return append([]byte(nil), f.data[offset:end]...), nil
}

// BuildIndex validates UTF-8 and builds sparse line anchors. It can be called
// once in a background worker; cancellation leaves the known prefix usable.
func (f *File) BuildIndex(ctx context.Context) error {
	// Hold a shared mapping lock while scanning. This lets viewport Segment
	// reads proceed, while making Close wait until no code can touch the map.
	f.indexMu.Lock()
	defer f.indexMu.Unlock()
	f.mu.RLock()
	if f.closed {
		f.mu.RUnlock()
		return ErrClosed
	}
	if f.indexedDone {
		f.mu.RUnlock()
		return nil
	}
	data := f.data
	anchors := []int64{0}
	line := int64(0)
	for i := 0; i < len(data); {
		if i&0xffff == 0 {
			select {
			case <-ctx.Done():
				f.mu.RUnlock()
				f.recordIndex(anchors, int64(i), line, false)
				return ctx.Err()
			default:
			}
		}
		r, n := utf8.DecodeRune(data[i:])
		if r == utf8.RuneError && n == 1 {
			f.mu.RUnlock()
			return ErrInvalidUTF8
		}
		if data[i] == '\n' {
			line++
			if line%SparseEveryLines == 0 {
				anchors = append(anchors, int64(i+1))
			}
		}
		i += n
	}
	f.mu.RUnlock()
	f.recordIndex(anchors, int64(len(data)), line+1, true)
	return nil
}

func (f *File) recordIndex(anchors []int64, bytes, lines int64, done bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return
	}
	f.anchors = anchors
	f.indexedBytes = bytes
	f.indexedLines = lines
	f.indexedDone = done
}

// IndexProgress is the count of fully known lines, known byte prefix, and
// whether the entire mapping has been indexed.
func (f *File) IndexProgress() (lines, bytes int64, complete bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.indexedLines, f.indexedBytes, f.indexedDone
}

// Line reads a physical line once the relevant sparse anchor is known. It
// performs a bounded local scan rather than allocating all lines in memory.
func (f *File) Line(number int64) ([]byte, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.closed {
		return nil, ErrClosed
	}
	if number < 0 {
		return nil, ErrBadOffset
	}
	anchor := number / SparseEveryLines
	if anchor >= int64(len(f.anchors)) {
		return nil, ErrBadOffset
	}
	off := f.anchors[anchor]
	line := anchor * SparseEveryLines
	for off < int64(len(f.data)) && line < number {
		if f.data[off] == '\n' {
			line++
		}
		off++
	}
	if line != number {
		return nil, ErrBadOffset
	}
	end := off
	for end < int64(len(f.data)) && f.data[end] != '\n' {
		end++
	}
	if end > off && f.data[end-1] == '\r' {
		end--
	}
	return append([]byte(nil), f.data[off:end]...), nil
}

func (f *File) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil
	}
	f.closed = true
	var err error
	if len(f.data) > 0 {
		err = syscall.Munmap(f.data)
		f.data = nil
	}
	if closeErr := f.f.Close(); err == nil {
		err = closeErr
	}
	return err
}
