package command

import (
	"sort"
	"strings"
)

type Command struct {
	ID, Title, Shortcut string
	Available           bool
}
type Registry struct{ items []Command }

func New(items ...Command) Registry { return Registry{items: append([]Command(nil), items...)} }
func Default() Registry {
	return New(Command{"file.open", "Open File…", "Ctrl+O", true}, Command{"workspace.quickOpen", "Quick Open", "Ctrl+P", true}, Command{"search.repository", "Search Repository", "Ctrl+Shift+F", true}, Command{"view.theme", "Choose Theme", "Ctrl+K Ctrl+T", true}, Command{"markdown.toggle", "Toggle Markdown Reading", "Ctrl+Shift+M", true}, Command{"git.nextHunk", "Next Change", "F7", true}, Command{"git.previousHunk", "Previous Change", "Shift+F7", true}, Command{"java.definition", "Go to Definition", "F12", false})
}
func (r Registry) All() []Command { return append([]Command(nil), r.items...) }
func (r Registry) Search(q string) []Command {
	q = strings.ToLower(strings.TrimSpace(q))
	out := append([]Command(nil), r.items...)
	sort.SliceStable(out, func(i, j int) bool { return score(out[i], q) > score(out[j], q) })
	return out
}
func score(c Command, q string) int {
	if q == "" {
		return 0
	}
	s := strings.ToLower(c.Title + " " + c.ID)
	if strings.Contains(s, q) {
		return 100 - len(s)
	}
	at := 0
	for _, ch := range q {
		k := strings.IndexRune(s[at:], ch)
		if k < 0 {
			return -1
		}
		at += k + 1
	}
	return 10 - at
}
