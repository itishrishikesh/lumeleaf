package ui

import "fmt"

type Viewport struct{ FirstLine, VisibleLines, Overscan, TotalLines int }

func (v Viewport) Range() (start, end int) {
	start = v.FirstLine - v.Overscan
	if start < 0 {
		start = 0
	}
	end = v.FirstLine + v.VisibleLines + v.Overscan
	if end > v.TotalLines {
		end = v.TotalLines
	}
	if end < start {
		end = start
	}
	return
}
func (v Viewport) Validate() error {
	if v.FirstLine < 0 || v.VisibleLines < 0 || v.Overscan < 0 || v.TotalLines < 0 {
		return fmt.Errorf("viewport values cannot be negative")
	}
	if v.FirstLine > v.TotalLines {
		return fmt.Errorf("first line exceeds document")
	}
	return nil
}

type SemanticNode struct {
	Role, Label                 string
	Line                        int
	Focused, Selected, ReadOnly bool
	Children                    []SemanticNode
}

func SemanticTree(name string, lines []string, first int, readOnly bool) SemanticNode {
	root := SemanticNode{Role: "document", Label: name, Focused: true, ReadOnly: readOnly}
	root.Children = make([]SemanticNode, 0, len(lines))
	for i, line := range lines {
		root.Children = append(root.Children, SemanticNode{Role: "text", Label: line, Line: first + i, ReadOnly: readOnly})
	}
	return root
}
