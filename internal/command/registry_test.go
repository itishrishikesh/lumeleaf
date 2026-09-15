package command

import "testing"

func TestPaletteRanking(t *testing.T) {
	r := Default().Search("theme")
	if len(r) == 0 || r[0].ID != "view.theme" {
		t.Fatalf("top=%+v", r)
	}
}
