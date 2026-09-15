package theme

import "testing"

func TestThemesAreCompleteAndReadable(t *testing.T) {
	for _, name := range Names() {
		p, err := Load(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		ratio, err := Contrast(p.Text, p.Background)
		if err != nil || ratio < 7 {
			t.Fatalf("%s contrast=%v err=%v", name, ratio, err)
		}
	}
}

func TestParseRejectsInvalid(t *testing.T) {
	if _, err := Parse("red"); err == nil {
		t.Fatal("accepted invalid color")
	}
}
