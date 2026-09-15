package search

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestStreamLiteralAndRegex(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Reader.java"), []byte("one\nLumeleaf fast\nthree\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, req := range []Request{{Root: root, Pattern: "lumeleaf", IgnoreCase: true, Workers: 2}, {Root: root, Pattern: "Lume.*fast", Regex: true, Workers: 2}} {
		out := make(chan Result, 4)
		if err := Stream(context.Background(), req, out); err != nil {
			t.Fatal(err)
		}
		got := <-out
		if got.LineNumber != 2 || got.Column != 1 {
			t.Fatalf("got=%+v", got)
		}
	}
}
