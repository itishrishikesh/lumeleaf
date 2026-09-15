package watcher

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanConvergesToDisk(t *testing.T) {
	root := t.TempDir()
	p := &Poller{Root: root}
	events, err := p.scan()
	if err != nil || len(events) != 0 {
		t.Fatalf("events=%v err=%v", events, err)
	}
	path := filepath.Join(root, "a.md")
	if err := os.WriteFile(path, []byte("one"), 0600); err != nil {
		t.Fatal(err)
	}
	events, _ = p.scan()
	if len(events) != 1 || events[0].Kind != Create {
		t.Fatalf("create=%v", events)
	}
	if err := os.WriteFile(path, []byte("two-longer"), 0600); err != nil {
		t.Fatal(err)
	}
	events, _ = p.scan()
	if len(events) != 1 || events[0].Kind != Modify {
		t.Fatalf("modify=%v", events)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	events, _ = p.scan()
	if len(events) != 1 || events[0].Kind != Remove {
		t.Fatalf("remove=%v", events)
	}
}
