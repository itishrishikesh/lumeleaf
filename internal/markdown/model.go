package markdown

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	goldtext "github.com/yuin/goldmark/text"
)

type Kind uint8

const (
	Paragraph Kind = iota
	Heading
	CodeBlock
	ListItem
	Quote
	ThematicBreak
	Table
)

type Span struct{ Start, End int }
type Block struct {
	Kind     Kind
	Level    int
	Text     string
	Language string
	Checked  *bool
	Source   Span
}
type Document struct {
	Blocks    []Block
	Headings  []int
	SourceLen int
}

var engine = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
)

func Parse(source []byte) (Document, error) {
	root := engine.Parser().Parse(goldtext.NewReader(source))
	doc := Document{SourceLen: len(source)}
	err := ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		var block Block
		switch v := n.(type) {
		case *ast.Heading:
			block = Block{Kind: Heading, Level: v.Level, Text: inlineText(v, source), Source: nodeSpan(v, source)}
			doc.Headings = append(doc.Headings, len(doc.Blocks))
		case *ast.Paragraph:
			if _, insideList := v.Parent().(*ast.ListItem); insideList {
				return ast.WalkContinue, nil
			}
			block = Block{Kind: Paragraph, Text: inlineText(v, source), Source: nodeSpan(v, source)}
		case *ast.FencedCodeBlock:
			block = Block{Kind: CodeBlock, Language: string(v.Language(source)), Text: linesText(v.Lines(), source), Source: nodeSpan(v, source)}
		case *ast.CodeBlock:
			block = Block{Kind: CodeBlock, Text: linesText(v.Lines(), source), Source: nodeSpan(v, source)}
		case *ast.ListItem:
			text := inlineText(v, source)
			block = Block{Kind: ListItem, Text: text, Source: nodeSpan(v, source)}
			trim := strings.TrimSpace(text)
			if strings.HasPrefix(trim, "[ ]") {
				x := false
				block.Checked = &x
				block.Text = strings.TrimSpace(trim[3:])
			} else if strings.HasPrefix(strings.ToLower(trim), "[x]") {
				x := true
				block.Checked = &x
				block.Text = strings.TrimSpace(trim[3:])
			}
		case *ast.Blockquote:
			block = Block{Kind: Quote, Text: inlineText(v, source), Source: nodeSpan(v, source)}
		case *ast.ThematicBreak:
			block = Block{Kind: ThematicBreak, Source: nodeSpan(v, source)}
		default:
			return ast.WalkContinue, nil
		}
		doc.Blocks = append(doc.Blocks, block)
		return ast.WalkContinue, nil
	})
	if err != nil {
		return Document{}, err
	}
	return doc, nil
}

func nodeSpan(n ast.Node, _ []byte) Span {
	if n.Lines() != nil && n.Lines().Len() > 0 {
		return Span{Start: n.Lines().At(0).Start, End: n.Lines().At(n.Lines().Len() - 1).Stop}
	}
	return Span{}
}
func linesText(lines *goldtext.Segments, source []byte) string {
	var b strings.Builder
	for i := 0; i < lines.Len(); i++ {
		segment := lines.At(i)
		b.Write(segment.Value(source))
	}
	return b.String()
}

func inlineText(n ast.Node, source []byte) string {
	var b strings.Builder
	_ = ast.Walk(n, func(child ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch v := child.(type) {
		case *ast.Text:
			b.Write(v.Segment.Value(source))
			if v.SoftLineBreak() || v.HardLineBreak() {
				b.WriteByte('\n')
			}
		case *ast.CodeSpan:
			b.Write(v.Text(source))
		case *ast.AutoLink:
			b.Write(v.URL(source))
		case *ast.String:
			b.Write(v.Value)
		}
		return ast.WalkContinue, nil
	})
	return strings.TrimSpace(b.String())
}

// ResourceAllowed applies Lumeleaf's safe-by-default Markdown policy.
func ResourceAllowed(raw, documentPath, trustedRoot string, remoteImages bool) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if !remoteImages {
			return "", fmt.Errorf("remote resources are disabled")
		}
		return u.String(), nil
	case "javascript", "data", "file":
		return "", fmt.Errorf("scheme %q is blocked", u.Scheme)
	case "":
		if strings.HasPrefix(raw, "//") {
			return "", fmt.Errorf("network-path resources are blocked")
		}
		root, err := filepath.Abs(trustedRoot)
		if err != nil {
			return "", err
		}
		p := filepath.Join(filepath.Dir(documentPath), filepath.FromSlash(u.Path))
		p, err = filepath.Abs(p)
		if err != nil {
			return "", err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("resource leaves trusted workspace")
		}
		return p, nil
	default:
		return "", fmt.Errorf("scheme %q is blocked", u.Scheme)
	}
}

func NearestBlock(doc Document, sourceOffset int) int {
	best := 0
	for i, b := range doc.Blocks {
		if b.Source.Start <= sourceOffset {
			best = i
		} else {
			break
		}
	}
	return best
}
