package syntax

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_java "github.com/tree-sitter/tree-sitter-java/bindings/go"
)

type Role uint8

const (
	RolePlain Role = iota
	RoleKeyword
	RoleType
	RoleString
	RoleComment
	RoleNumber
	RoleFunction
	RoleAnnotation
)

type Span struct {
	Start, End int
	Role       Role
}
type Symbol struct {
	Kind, Name string
	Start, End int
	Depth      int
}
type Fold struct{ StartLine, EndLine int }
type Result struct {
	Revision  uint64
	Spans     []Span
	Symbols   []Symbol
	Folds     []Fold
	HasErrors bool
}

type Java struct {
	mu       sync.Mutex
	parser   *sitter.Parser
	tree     *sitter.Tree
	source   []byte
	revision uint64
}

func NewJava() (*Java, error) {
	p := sitter.NewParser()
	if err := p.SetLanguage(sitter.NewLanguage(tree_sitter_java.Language())); err != nil {
		p.Close()
		return nil, err
	}
	return &Java{parser: p}, nil
}
func (j *Java) Close() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.tree != nil {
		j.tree.Close()
		j.tree = nil
	}
	if j.parser != nil {
		j.parser.Close()
		j.parser = nil
	}
}

func (j *Java) Parse(ctx context.Context, source []byte, revision uint64, visibleStart, visibleEnd int) (Result, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.parser == nil {
		return Result{}, fmt.Errorf("parser is closed")
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	var tree *sitter.Tree
	if ctx.Done() == nil {
		tree = j.parser.Parse(source, nil)
	} else {
		tree = j.parser.ParseWithOptions(func(offset int, _ sitter.Point) []byte {
			if offset >= len(source) {
				return nil
			}
			return source[offset:]
		}, nil, &sitter.ParseOptions{ProgressCallback: func(s sitter.ParseState) bool { return ctx.Err() != nil }})
	}
	if tree == nil {
		return Result{}, ctx.Err()
	}
	if j.tree != nil {
		j.tree.Close()
	}
	j.tree = tree
	j.source = append(j.source[:0], source...)
	j.revision = revision
	if visibleEnd <= 0 || visibleEnd > len(source) {
		visibleEnd = len(source)
	}
	if visibleStart < 0 {
		visibleStart = 0
	}
	result := Result{Revision: revision, HasErrors: tree.RootNode().HasError()}
	walk(tree.RootNode(), source, 0, visibleStart, visibleEnd, &result)
	sort.Slice(result.Spans, func(a, b int) bool { return result.Spans[a].Start < result.Spans[b].Start })
	return result, nil
}

func walk(n *sitter.Node, src []byte, depth, start, end int, out *Result) {
	kind := n.Kind()
	s, e := int(n.StartByte()), int(n.EndByte())
	if e <= start || s >= end {
		return
	}
	if role, ok := roleFor(kind); ok && e > start && s < end {
		out.Spans = append(out.Spans, Span{max(s, start), min(e, end), role})
	}
	if sk, ok := symbolKinds[kind]; ok {
		if name := n.ChildByFieldName("name"); name != nil {
			out.Symbols = append(out.Symbols, Symbol{Kind: sk, Name: name.Utf8Text(src), Start: s, End: e, Depth: depth})
		}
	}
	sp, ep := n.StartPosition(), n.EndPosition()
	if ep.Row > sp.Row && isFold(kind) {
		out.Folds = append(out.Folds, Fold{int(sp.Row), int(ep.Row)})
	}
	cursor := n.Walk()
	defer cursor.Close()
	if !cursor.GotoFirstChild() {
		return
	}
	for {
		child := cursor.Node()
		if child.IsNamed() {
			if int(child.StartByte()) >= end {
				return
			}
			walk(child, src, depth+1, start, end, out)
		}
		if !cursor.GotoNextSibling() {
			return
		}
	}
}

var symbolKinds = map[string]string{"class_declaration": "class", "interface_declaration": "interface", "enum_declaration": "enum", "record_declaration": "record", "method_declaration": "method", "constructor_declaration": "constructor", "field_declaration": "field", "annotation_type_declaration": "annotation"}

func isFold(k string) bool {
	return strings.HasSuffix(k, "_declaration") || k == "block" || k == "class_body" || k == "interface_body" || k == "enum_body" || k == "switch_block" || k == "block_comment"
}
func roleFor(k string) (Role, bool) {
	switch {
	case k == "line_comment" || k == "block_comment":
		return RoleComment, true
	case strings.Contains(k, "string") || k == "character_literal":
		return RoleString, true
	case strings.Contains(k, "integer_literal") || strings.Contains(k, "floating_point_literal"):
		return RoleNumber, true
	case k == "type_identifier" || k == "integral_type" || k == "floating_point_type" || k == "void_type":
		return RoleType, true
	case k == "annotation":
		return RoleAnnotation, true
	case k == "identifier":
		return RolePlain, false
	case keyword[k]:
		return RoleKeyword, true
	}
	return RolePlain, false
}

var keyword = map[string]bool{"abstract": true, "assert": true, "boolean": true, "break": true, "byte": true, "case": true, "catch": true, "char": true, "class": true, "const": true, "continue": true, "default": true, "do": true, "double": true, "else": true, "enum": true, "exports": true, "extends": true, "final": true, "finally": true, "float": true, "for": true, "if": true, "implements": true, "import": true, "instanceof": true, "int": true, "interface": true, "long": true, "module": true, "native": true, "new": true, "non-sealed": true, "open": true, "opens": true, "package": true, "permits": true, "private": true, "protected": true, "provides": true, "public": true, "record": true, "requires": true, "return": true, "sealed": true, "short": true, "static": true, "strictfp": true, "super": true, "switch": true, "synchronized": true, "this": true, "throw": true, "throws": true, "to": true, "transient": true, "transitive": true, "try": true, "uses": true, "var": true, "void": true, "volatile": true, "while": true, "with": true, "yield": true}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
