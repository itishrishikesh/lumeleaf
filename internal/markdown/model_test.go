package markdown

import (
	"path/filepath"
	"testing"
)

func TestParseNativeModel(t *testing.T) {
	src := []byte("# Reader\n\nA *fast* viewer.\n\n- [x] Java\n\n```java\nrecord A(int n) {}\n```\n")
	doc, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Blocks) < 4 || doc.Blocks[0].Kind != Heading || doc.Blocks[0].Text != "Reader" {
		t.Fatalf("unexpected model: %#v", doc)
	}
	foundCode := false
	for _, b := range doc.Blocks {
		if b.Kind == CodeBlock {
			foundCode = true
			if b.Language != "java" {
				t.Fatalf("language=%q", b.Language)
			}
		}
	}
	if !foundCode {
		t.Fatal("code block missing")
	}
}

func TestResourcePolicy(t *testing.T) {
	root := t.TempDir()
	doc := filepath.Join(root, "notes", "readme.md")
	if _, err := ResourceAllowed("https://example.com/x.png", doc, root, false); err == nil {
		t.Fatal("remote resource allowed")
	}
	if _, err := ResourceAllowed("../../secret", doc, root, false); err == nil {
		t.Fatal("workspace escape allowed")
	}
	if got, err := ResourceAllowed("image.png", doc, root, false); err != nil || got != filepath.Join(root, "notes", "image.png") {
		t.Fatalf("got %q err=%v", got, err)
	}
	if _, err := ResourceAllowed("javascript:alert(1)", doc, root, true); err == nil {
		t.Fatal("javascript allowed")
	}
}

func BenchmarkMarkdownParse_10KBlocks(b *testing.B) {
	block := []byte("## Heading\n\nA paragraph with `code` and a [link](https://example.com).\n\n")
	src := make([]byte, 0, len(block)*10000)
	for range 10000 {
		src = append(src, block...)
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Parse(src); err != nil {
			b.Fatal(err)
		}
	}
}
