package ui

import "testing"

func TestVirtualMillionLineDocument(t *testing.T) {
	v := Viewport{FirstLine: 500000, VisibleLines: 200, Overscan: 20, TotalLines: 1000000}
	s, e := v.Range()
	if s != 499980 || e != 500220 {
		t.Fatalf("range=%d:%d", s, e)
	}
	if e-s > 240 {
		t.Fatal("layout grew with document")
	}
}
func TestSemanticTree(t *testing.T) {
	tree := SemanticTree("Reader.java", []string{"class Reader {}"}, 20, true)
	if tree.Role != "document" || len(tree.Children) != 1 || tree.Children[0].Line != 20 || !tree.Children[0].ReadOnly {
		t.Fatalf("tree=%+v", tree)
	}
}
func BenchmarkVisibleLines_1M(b *testing.B) {
	v := Viewport{FirstLine: 500000, VisibleLines: 200, Overscan: 20, TotalLines: 1000000}
	b.ReportAllocs()
	for b.Loop() {
		s, e := v.Range()
		if e-s != 240 {
			b.Fatal(e - s)
		}
	}
}
