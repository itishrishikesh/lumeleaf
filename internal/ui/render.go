package ui

import (
	"bufio"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/itishrishikesh/lumeleaf/internal/markdown"
	"github.com/itishrishikesh/lumeleaf/internal/theme"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

type RenderOptions struct {
	Input, Output, Theme, View string
	Scale                      float64
	Width, Height              int
	FirstLine                  int
	Focused, ReducedMotion     bool
}

func Render(options RenderOptions) error {
	if options.Width == 0 {
		options.Width = 1440
	}
	if options.Height == 0 {
		options.Height = 900
	}
	if options.Scale == 0 {
		options.Scale = 1
	}
	if options.Theme == "" {
		options.Theme = "sepia"
	}
	if options.View == "" {
		options.View = "code"
	}
	p, err := theme.Load(options.Theme)
	if err != nil {
		return err
	}
	body, err := os.ReadFile(options.Input)
	if err != nil {
		return err
	}
	return renderBytes(body, filepath.Base(options.Input), p, options)
}
func renderBytes(body []byte, name string, p theme.Palette, o RenderOptions) error {
	bg, _ := theme.Parse(p.Background)
	surface, _ := theme.Parse(p.Surface)
	raised, _ := theme.Parse(p.SurfaceRaised)
	fg, _ := theme.Parse(p.Text)
	muted, _ := theme.Parse(p.Muted)
	accent, _ := theme.Parse(p.Accent)
	selection, _ := theme.Parse(p.Selection)
	img := image.NewNRGBA(image.Rect(0, 0, o.Width, o.Height))
	draw.Draw(img, img.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)
	fill(img, image.Rect(0, 0, o.Width, 58), raised)
	fill(img, image.Rect(0, 58, 250, o.Height), surface)
	fill(img, image.Rect(250, 58, o.Width, 96), selection)
	regular := face(goregular.TTF, 14*o.Scale)
	small := face(goregular.TTF, 11*o.Scale)
	mono := face(gomono.TTF, 13*o.Scale)
	text(img, regular, fg, 24, 37, "Lumeleaf")
	text(img, small, muted, 132, 36, "VIEW-FIRST WORKSPACE")
	text(img, regular, fg, 274, 84, name)
	text(img, small, muted, 24, 91, "CHANGES")
	text(img, small, accent, 24, 122, "●  Reader.java")
	text(img, small, muted, 24, 148, "   README.md")
	text(img, small, muted, 24, o.Height-38, "LOCAL  •  TRUSTED")
	content := image.Rect(250, 96, o.Width, o.Height)
	fill(img, content, bg)
	switch o.View {
	case "markdown":
		renderMarkdown(img, body, p, regular, mono, o)
	case "diff":
		renderDiff(img, body, p, mono, o)
	default:
		renderCode(img, body, p, mono, o)
	}
	if err := os.MkdirAll(filepath.Dir(o.Output), 0755); err != nil {
		return err
	}
	f, err := os.Create(o.Output)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
func renderCode(img *image.NRGBA, body []byte, p theme.Palette, mono font.Face, o RenderOptions) {
	fg, _ := theme.Parse(p.Text)
	muted, _ := theme.Parse(p.Muted)
	comment, _ := theme.Parse(p.Comment)
	keyword, _ := theme.Parse(p.Keyword)
	strc, _ := theme.Parse(p.String)
	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	lineNo := 0
	y := 126
	lineH := int(22 * o.Scale)
	for scanner.Scan() {
		lineNo++
		if lineNo <= o.FirstLine {
			continue
		}
		if y > img.Bounds().Dy()-25 {
			break
		}
		line := scanner.Text()
		text(img, mono, muted, 272, y, fmt.Sprintf("%4d", lineNo))
		x := 330
		segments := lexLine(line)
		for _, s := range segments {
			c := fg
			if s.kind == "comment" {
				c = comment
			} else if s.kind == "keyword" {
				c = keyword
			} else if s.kind == "string" {
				c = strc
			}
			text(img, mono, c, x, y, s.text)
			x += font.MeasureString(mono, s.text).Ceil()
		}
		y += lineH
	}
}

type lexical struct{ text, kind string }

func lexLine(line string) []lexical {
	trim := strings.TrimSpace(line)
	if strings.HasPrefix(trim, "//") {
		return []lexical{{line, "comment"}}
	}
	var out []lexical
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			out = append(out, lexical{b.String(), "plain"})
			b.Reset()
		}
	}
	words := map[string]bool{"public": true, "private": true, "protected": true, "class": true, "interface": true, "record": true, "enum": true, "static": true, "final": true, "void": true, "return": true, "new": true, "package": true, "import": true, "extends": true, "implements": true, "if": true, "else": true, "for": true, "while": true, "try": true, "catch": true, "throw": true}
	for i := 0; i < len(line); {
		if line[i] == '"' {
			flush()
			j := i + 1
			for j < len(line) {
				if line[j] == '"' && line[j-1] != '\\' {
					j++
					break
				}
				j++
			}
			out = append(out, lexical{line[i:j], "string"})
			i = j
			continue
		}
		if isWord(line[i]) {
			j := i + 1
			for j < len(line) && isWord(line[j]) {
				j++
			}
			word := line[i:j]
			flush()
			kind := "plain"
			if words[word] {
				kind = "keyword"
			}
			out = append(out, lexical{word, kind})
			i = j
			continue
		}
		b.WriteByte(line[i])
		i++
	}
	flush()
	return out
}

func isWord(b byte) bool {
	return b == '_' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}
func renderMarkdown(img *image.NRGBA, body []byte, p theme.Palette, regular, mono font.Face, o RenderOptions) {
	doc, _ := markdown.Parse(body)
	fg, _ := theme.Parse(p.Text)
	muted, _ := theme.Parse(p.Muted)
	accent, _ := theme.Parse(p.Accent)
	surface, _ := theme.Parse(p.Surface)
	y := 138
	for _, b := range doc.Blocks {
		if y > img.Bounds().Dy()-40 {
			break
		}
		switch b.Kind {
		case markdown.Heading:
			text(img, regular, accent, 300, y, b.Text)
			y += 42
		case markdown.CodeBlock:
			codeLines := strings.Split(strings.TrimSuffix(b.Text, "\n"), "\n")
			height := 34 + len(codeLines)*24
			fill(img, image.Rect(290, y-22, img.Bounds().Dx()-40, y-22+height), surface)
			for _, line := range codeLines {
				text(img, mono, muted, 310, y, line)
				y += 24
			}
			y += 32
		case markdown.ListItem:
			text(img, regular, fg, 314, y, "• "+b.Text)
			y += 28
		default:
			if b.Text != "" {
				text(img, regular, fg, 300, y, b.Text)
				y += 32
			}
		}
	}
}
func renderDiff(img *image.NRGBA, body []byte, p theme.Palette, mono font.Face, o RenderOptions) {
	fg, _ := theme.Parse(p.Text)
	bg, _ := theme.Parse(p.Background)
	add, _ := theme.Parse(p.Added)
	remove, _ := theme.Parse(p.Removed)
	surface, _ := theme.Parse(p.Surface)
	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	y := 126
	for scanner.Scan() {
		line := scanner.Text()
		c := fg
		if strings.HasPrefix(line, "+") {
			c = add
			fill(img, image.Rect(260, y-17, img.Bounds().Dx(), y+5), mix(bg, add, 0.16))
		} else if strings.HasPrefix(line, "-") {
			c = remove
			fill(img, image.Rect(260, y-17, img.Bounds().Dx(), y+5), mix(bg, remove, 0.16))
		} else if strings.HasPrefix(line, "@@") {
			c = remove
			fill(img, image.Rect(260, y-17, img.Bounds().Dx(), y+5), surface)
		}
		text(img, mono, c, 280, y, line)
		y += 22
		if y > img.Bounds().Dy()-20 {
			break
		}
	}
}
func face(data []byte, size float64) font.Face {
	f, err := opentype.Parse(data)
	if err != nil {
		panic(err)
	}
	v, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 96, Hinting: font.HintingFull})
	if err != nil {
		panic(err)
	}
	return v
}
func text(dst draw.Image, face font.Face, c color.NRGBA, x, y int, s string) {
	d := font.Drawer{Dst: dst, Src: &image.Uniform{C: c}, Face: face, Dot: fixed.P(x, y)}
	d.DrawString(strings.ReplaceAll(s, "\t", "    "))
}
func fill(dst draw.Image, r image.Rectangle, c color.NRGBA) {
	draw.Draw(dst, r, &image.Uniform{C: c}, image.Point{}, draw.Src)
}
func mix(a, b color.NRGBA, amount float64) color.NRGBA {
	blend := func(x, y uint8) uint8 { return uint8(float64(x)*(1-amount) + float64(y)*amount) }
	return color.NRGBA{R: blend(a.R, b.R), G: blend(a.G, b.G), B: blend(a.B, b.B), A: 255}
}
