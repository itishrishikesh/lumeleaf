// Package watcher exposes filesystem changes as reconciled, coalesced hints.
// Polling is intentional: it works on every supported OS and treats events as
// hints, so callers can always re-read the file before applying a change.
package watcher

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type EventKind uint8

const (
	Create EventKind = iota
	Modify
	Remove
)

type Event struct {
	Path    string
	Kind    EventKind
	Size    int64
	ModTime time.Time
	Hash    [32]byte
}
type Poller struct {
	Root               string
	Interval, Debounce time.Duration
	mu                 sync.Mutex
	previous           map[string]fingerprint
}
type fingerprint struct {
	size int64
	mod  time.Time
	hash [32]byte
}

func (p *Poller) Run(ctx context.Context, out chan<- []Event) error {
	defer close(out)
	if p.Interval <= 0 {
		p.Interval = 250 * time.Millisecond
	}
	if p.Debounce <= 0 {
		p.Debounce = 50 * time.Millisecond
	}
	if p.previous == nil {
		p.previous = map[string]fingerprint{}
	}
	ticker := time.NewTicker(p.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			ev, e := p.scan()
			if e != nil {
				return e
			}
			if len(ev) > 0 {
				select {
				case out <- ev:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
	}
}
func (p *Poller) scan() ([]Event, error) {
	current := map[string]fingerprint{}
	e := filepath.WalkDir(p.Root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		inf, e := d.Info()
		if e != nil {
			return nil
		}
		h := sha256.New()
		f, e := os.Open(path)
		if e == nil {
			buf := make([]byte, 64*1024)
			for {
				n, x := f.Read(buf)
				if n > 0 {
					h.Write(buf[:n])
				}
				if x != nil {
					break
				}
			}
			f.Close()
		}
		var sum [32]byte
		copy(sum[:], h.Sum(nil))
		rel, _ := filepath.Rel(p.Root, path)
		current[filepath.ToSlash(rel)] = fingerprint{inf.Size(), inf.ModTime(), sum}
		return nil
	})
	if e != nil {
		return nil, e
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	var ev []Event
	for path, now := range current {
		old, ok := p.previous[path]
		if !ok {
			ev = append(ev, Event{path, Create, now.size, now.mod, now.hash})
		} else if old != now {
			ev = append(ev, Event{path, Modify, now.size, now.mod, now.hash})
		}
	}
	for path, old := range p.previous {
		if _, ok := current[path]; !ok {
			ev = append(ev, Event{path, Remove, old.size, old.mod, old.hash})
		}
	}
	p.previous = current
	sort.Slice(ev, func(i, j int) bool { return ev[i].Path < ev[j].Path })
	return ev, nil
}
