package session

import (
	"path/filepath"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "session.json")
	want := State{Theme: "sepia", Active: Location{Path: "Reader.java", Line: 42}}
	if err := Save(p, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if got.Theme != want.Theme || got.Active != want.Active {
		t.Fatalf("got=%+v", got)
	}
}
