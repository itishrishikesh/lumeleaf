package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWalkNestedAndFiltered(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{"src/a.java", "src/deep/b.md", ".git/config", "build/out.txt"} {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("text"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "binary"), []byte{0, 1, 2}, 0600); err != nil {
		t.Fatal(err)
	}
	out := make(chan []Entry, 8)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := Walk(ctx, WalkOptions{Root: root, Workers: 2, BatchSize: 1, Exclusions: []string{"build"}}, out); err != nil {
		t.Fatal(err)
	}
	var paths []string
	for batch := range out {
		for _, e := range batch {
			paths = append(paths, e.Path)
		}
	}
	if fmt.Sprint(paths) != "[src/a.java src/deep/b.md]" && fmt.Sprint(paths) != "[src/deep/b.md src/a.java]" {
		t.Fatalf("paths=%v", paths)
	}
}
func TestQuickOpenRanking(t *testing.T) {
	now := time.Now()
	paths := []string{"src/Reader.java", "docs/reader.md", "src/Other.java"}
	meta := map[string]QuickOpenEntry{"src/Reader.java": {Changed: true, LastOpened: now}}
	got := RankQuickOpen(paths, QuickOpenQuery{Text: "read", Limit: 2, Now: now}, meta)
	if len(got) != 2 || got[0].Path != "src/Reader.java" {
		t.Fatalf("got=%v", got)
	}
}
func BenchmarkQuickOpenRank_100KPaths(b *testing.B) {
	paths := make([]string, 100000)
	for i := range paths {
		paths[i] = fmt.Sprintf("module-%03d/src/package-%03d/Reader%06d.java", i%100, i%1000, i)
	}
	q := QuickOpenQuery{Text: "Reader999", Limit: 50, Now: time.Unix(0, 0)}
	b.ReportAllocs()
	for b.Loop() {
		RankQuickOpen(paths, q, nil)
	}
}
