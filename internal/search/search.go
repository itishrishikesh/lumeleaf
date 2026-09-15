// Package search provides a streaming search engine with no runtime dependencies.
package search

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

type Request struct {
	Root, Pattern            string
	Paths                    []string
	Regex, IgnoreCase, UseRG bool
	Workers                  int
}
type Result struct {
	Path, Line         string
	LineNumber, Column int
}

// Stream sends matches as soon as lines are read and always closes out.
func Stream(ctx context.Context, req Request, out chan<- Result) error {
	defer close(out)
	if req.UseRG {
		if p, e := exec.LookPath("rg"); e == nil {
			return streamRG(ctx, req, out, p)
		}
	}
	return streamGo(ctx, req, out)
}

func streamRG(ctx context.Context, r Request, out chan<- Result, bin string) error {
	args := []string{"--json", "--line-number", "--column", "--no-heading"}
	if r.IgnoreCase {
		args = append(args, "-i")
	}
	if r.Regex {
		args = append(args, "-e", r.Pattern)
	} else {
		args = append(args, "-F", "-e", r.Pattern)
	}
	args = append(args, ".")
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = r.Root
	stdout, e := cmd.StdoutPipe()
	if e != nil {
		return e
	}
	if e = cmd.Start(); e != nil {
		return e
	}
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		var v struct {
			Type string `json:"type"`
			Data struct {
				Path struct {
					Text string `json:"text"`
				} `json:"path"`
				Lines struct {
					Text string `json:"text"`
				} `json:"lines"`
				LineNumber, AbsoluteOffset, Submatch []struct{} `json:"-"`
				LineNumber2                          int        `json:"line_number"`
				Submatches                           []struct {
					Start int `json:"start"`
				} `json:"submatches"`
			} `json:"data"`
		}
		if json.Unmarshal(sc.Bytes(), &v) != nil || v.Type != "match" {
			continue
		}
		line := strings.TrimSuffix(v.Data.Lines.Text, "\n")
		col := 1
		if len(v.Data.Submatches) > 0 {
			col = v.Data.Submatches[0].Start + 1
		}
		path := strings.TrimPrefix(v.Data.Path.Text, "./")
		send(ctx, out, Result{Path: path, Line: line, LineNumber: v.Data.LineNumber2, Column: col})
	}
	if e = sc.Err(); e != nil {
		return e
	}
	return cmd.Wait()
}

func streamGo(ctx context.Context, r Request, out chan<- Result) error {
	if r.Workers < 1 {
		r.Workers = 4
	}
	var files []string
	if len(r.Paths) > 0 {
		for _, p := range r.Paths {
			files = append(files, filepath.Join(r.Root, p))
		}
	} else {
		e := filepath.WalkDir(r.Root, func(p string, d os.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if d.IsDir() {
				if p != r.Root && strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if d.Type().IsRegular() {
				files = append(files, p)
			}
			return nil
		})
		if e != nil {
			return e
		}
	}
	var re *regexp.Regexp
	var e error
	pattern := r.Pattern
	if r.IgnoreCase && !r.Regex {
		pattern = strings.ToLower(pattern)
	}
	if r.Regex {
		flags := ""
		if r.IgnoreCase {
			flags = "(?i)"
		}
		re, e = regexp.Compile(flags + r.Pattern)
		if e != nil {
			return e
		}
	}
	jobs := make(chan string)
	var wg sync.WaitGroup
	wg.Add(r.Workers)
	for i := 0; i < r.Workers; i++ {
		go func() {
			defer wg.Done()
			for p := range jobs {
				scanFile(ctx, p, pattern, re, r, out)
			}
		}()
	}
	for _, p := range files {
		select {
		case jobs <- p:
		case <-ctx.Done():
			break
		}
	}
	close(jobs)
	wg.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}
func scanFile(ctx context.Context, path, pattern string, re *regexp.Regexp, r Request, out chan<- Result) {
	f, e := os.Open(path)
	if e != nil {
		return
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 64*1024), 16*1024*1024)
	n := 0
	for s.Scan() {
		n++
		line := s.Text()
		hay := line
		needle := pattern
		if !r.Regex && r.IgnoreCase {
			hay = strings.ToLower(hay)
		}
		col := -1
		if re != nil {
			loc := re.FindStringIndex(line)
			if loc != nil {
				col = loc[0]
			}
		} else {
			col = bytes.Index([]byte(hay), []byte(needle))
		}
		if col >= 0 {
			send(ctx, out, Result{Path: path, Line: line, LineNumber: n, Column: col + 1})
		}
	}
}
func send(ctx context.Context, out chan<- Result, v Result) bool {
	select {
	case out <- v:
		return true
	case <-ctx.Done():
		return false
	}
}
