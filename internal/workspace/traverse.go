// Package workspace contains the incremental, filesystem-only workspace index.
package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Entry is a regular file discovered by a traversal. Path is relative to Root
// and uses slash separators, making entries portable and stable in tests.
type Entry struct {
	Path string
	Size int64
	Mode os.FileMode
}

// WalkOptions controls traversal. Exclusions are path or basename patterns
// using filepath.Match; a leading slash matches from the workspace root.
type WalkOptions struct {
	Root          string
	Workers       int
	IncludeHidden bool
	IncludeBinary bool
	Exclusions    []string
	BatchSize     int
}

// Walk incrementally sends file batches. Cancellation is checked before every
// directory read and while publishing. Traversal does not follow symlinks.
func Walk(ctx context.Context, opts WalkOptions, batches chan<- []Entry) error {
	defer close(batches)
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return err
	}
	if opts.Workers < 1 {
		opts.Workers = 4
	}
	if opts.BatchSize < 1 {
		opts.BatchSize = 128
	}
	if batches == nil {
		return os.ErrInvalid
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type scanResult struct {
		directories []string
		entries     []Entry
		err         error
	}
	jobs := make(chan string)
	results := make(chan scanResult, opts.Workers)
	for range opts.Workers {
		go func() {
			for dir := range jobs {
				r := scanResult{}
				ds, e := os.ReadDir(dir)
				if e != nil {
					r.err = e
					results <- r
					continue
				}
				sort.Slice(ds, func(i, j int) bool { return ds[i].Name() < ds[j].Name() })
				for _, d := range ds {
					rel, _ := filepath.Rel(root, filepath.Join(dir, d.Name()))
					rel = filepath.ToSlash(rel)
					if excluded(rel, d.Name(), opts) {
						continue
					}
					full := filepath.Join(dir, d.Name())
					if d.IsDir() {
						r.directories = append(r.directories, full)
						continue
					}
					if !d.Type().IsRegular() {
						continue
					}
					info, e := d.Info()
					if e != nil {
						continue
					}
					if !opts.IncludeBinary && info.Size() <= 1<<20 && binaryFile(full) {
						continue
					}
					r.entries = append(r.entries, Entry{Path: rel, Size: info.Size(), Mode: info.Mode()})
				}
				select {
				case results <- r:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	queue := []string{root}
	active := 0
	batch := make([]Entry, 0, opts.BatchSize)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		out := append([]Entry(nil), batch...)
		batch = batch[:0]
		select {
		case batches <- out:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	for len(queue) > 0 || active > 0 {
		var send chan string
		var next string
		if len(queue) > 0 && active < opts.Workers {
			send = jobs
			next = queue[0]
		}
		select {
		case <-ctx.Done():
			close(jobs)
			return ctx.Err()
		case send <- next:
			queue = queue[1:]
			active++
		case r := <-results:
			active--
			if r.err != nil {
				close(jobs)
				return r.err
			}
			queue = append(queue, r.directories...)
			for _, entry := range r.entries {
				batch = append(batch, entry)
				if len(batch) >= opts.BatchSize {
					if err := flush(); err != nil {
						close(jobs)
						return err
					}
				}
			}
		}
	}
	close(jobs)
	if err := flush(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return ctx.Err()
}

func excluded(rel, name string, o WalkOptions) bool {
	if !o.IncludeHidden && strings.HasPrefix(name, ".") && name != "." {
		return true
	}
	for _, p := range o.Exclusions {
		p = strings.ReplaceAll(p, "\\", "/")
		if strings.HasPrefix(p, "/") {
			p = strings.TrimPrefix(p, "/")
			if ok, _ := filepath.Match(p, rel); ok {
				return true
			}
			if strings.HasPrefix(rel, p+"/") {
				return true
			}
		} else {
			if ok, _ := filepath.Match(p, name); ok {
				return true
			}
			if ok, _ := filepath.Match(p, rel); ok {
				return true
			}
		}
	}
	return false
}

func binaryFile(path string) bool {
	f, e := os.Open(path)
	if e != nil {
		return false
	}
	defer f.Close()
	b := make([]byte, 8192)
	n, _ := f.Read(b)
	return strings.IndexByte(string(b[:n]), 0) >= 0
}
