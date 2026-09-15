package syntax

import (
	"context"
	"strings"
	"testing"
)

func TestJavaModernStructure(t *testing.T) {
	j, err := NewJava()
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	src := []byte("package demo; public sealed interface Shape permits Circle {} record Circle(double radius) implements Shape { double area(){ return Math.PI * radius * radius; } }")
	r, err := j.Parse(context.Background(), src, 7, 0, len(src))
	if err != nil {
		t.Fatal(err)
	}
	if r.Revision != 7 || r.HasErrors {
		t.Fatalf("result=%+v", r)
	}
	var names []string
	for _, s := range r.Symbols {
		names = append(names, s.Name)
	}
	joined := strings.Join(names, ",")
	if !strings.Contains(joined, "Shape") || !strings.Contains(joined, "Circle") || !strings.Contains(joined, "area") {
		t.Fatalf("symbols=%v", names)
	}
	if len(r.Spans) == 0 {
		t.Fatal("no highlighting spans")
	}
}
func TestJavaMalformedStillReturnsTree(t *testing.T) {
	j, _ := NewJava()
	defer j.Close()
	r, err := j.Parse(context.Background(), []byte("class Broken { void x("), 1, 0, 99)
	if err != nil {
		t.Fatal(err)
	}
	if !r.HasErrors {
		t.Fatal("expected syntax error")
	}
}
func BenchmarkTreeSitterJavaInitial_10KLines(b *testing.B) {
	j, _ := NewJava()
	defer j.Close()
	src := []byte("package p; class C {\n" + strings.Repeat("int value = 42; // field\n", 10000) + "}\n")
	b.ReportAllocs()
	var rev uint64
	for b.Loop() {
		rev++
		if _, err := j.Parse(context.Background(), src, rev, 0, 4096); err != nil {
			b.Fatal(err)
		}
	}
}
