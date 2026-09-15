package ui

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestGoldenRenderDeterministic(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "Reader.java")
	if err := os.WriteFile(input, []byte("package demo;\n\npublic record Reader(String name) {\n    // Human-scale review\n    public String title() { return \"Hello \" + name; }\n}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	a := filepath.Join(dir, "a.png")
	b := filepath.Join(dir, "b.png")
	o := RenderOptions{Input: input, Output: a, Theme: "sepia", Width: 800, Height: 500}
	if err := Render(o); err != nil {
		t.Fatal(err)
	}
	o.Output = b
	if err := Render(o); err != nil {
		t.Fatal(err)
	}
	x, _ := os.ReadFile(a)
	y, _ := os.ReadFile(b)
	if string(x) != string(y) {
		t.Fatal("render is not deterministic")
	}
	f, err := os.Open(a)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 800 || img.Bounds().Dy() != 500 {
		t.Fatalf("bounds=%v", img.Bounds())
	}
}
func BenchmarkPaintViewport_200Lines(b *testing.B) {
	dir := b.TempDir()
	input := filepath.Join(dir, "x.java")
	var body []byte
	for range 200 {
		body = append(body, []byte("public String value() { return \"fast\"; }\n")...)
	}
	if err := os.WriteFile(input, body, 0600); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if err := Render(RenderOptions{Input: input, Output: filepath.Join(dir, "x.png"), Theme: "dark", Width: 1000, Height: 700}); err != nil {
			b.Fatal(err)
		}
	}
}
