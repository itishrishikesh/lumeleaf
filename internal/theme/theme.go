package theme

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"sort"

	themeassets "github.com/itishrishikesh/lumeleaf/assets/themes"
)

type Palette struct {
	Name          string `json:"name"`
	Background    string `json:"background"`
	Surface       string `json:"surface"`
	SurfaceRaised string `json:"surfaceRaised"`
	Text          string `json:"text"`
	Muted         string `json:"muted"`
	Accent        string `json:"accent"`
	Keyword       string `json:"keyword"`
	Type          string `json:"type"`
	String        string `json:"string"`
	Comment       string `json:"comment"`
	Number        string `json:"number"`
	Selection     string `json:"selection"`
	Added         string `json:"added"`
	Removed       string `json:"removed"`
	Warning       string `json:"warning"`
	Error         string `json:"error"`
}

var required = []string{"Background", "Surface", "SurfaceRaised", "Text", "Muted", "Accent", "Keyword", "Type", "String", "Comment", "Number", "Selection", "Added", "Removed", "Warning", "Error"}

func Names() []string { return []string{"dark", "light", "sepia"} }

func Load(name string) (Palette, error) {
	data, err := themeassets.Files.ReadFile(name + ".json")
	if err != nil {
		return Palette{}, fmt.Errorf("theme %q: %w", name, err)
	}
	var p Palette
	if err := json.Unmarshal(data, &p); err != nil {
		return Palette{}, fmt.Errorf("theme %q: %w", name, err)
	}
	if err := p.Validate(); err != nil {
		return Palette{}, err
	}
	return p, nil
}

func (p Palette) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("theme name is empty")
	}
	values := map[string]string{"Background": p.Background, "Surface": p.Surface, "SurfaceRaised": p.SurfaceRaised, "Text": p.Text, "Muted": p.Muted, "Accent": p.Accent, "Keyword": p.Keyword, "Type": p.Type, "String": p.String, "Comment": p.Comment, "Number": p.Number, "Selection": p.Selection, "Added": p.Added, "Removed": p.Removed, "Warning": p.Warning, "Error": p.Error}
	for _, key := range required {
		if _, err := Parse(values[key]); err != nil {
			return fmt.Errorf("theme %s token %s: %w", p.Name, key, err)
		}
	}
	if ratio, _ := Contrast(p.Text, p.Background); ratio < 7 {
		return fmt.Errorf("theme %s text contrast %.2f below 7", p.Name, ratio)
	}
	return nil
}

func Parse(s string) (color.NRGBA, error) {
	if len(s) != 7 || s[0] != '#' {
		return color.NRGBA{}, fmt.Errorf("expected #RRGGBB")
	}
	b, err := hex.DecodeString(s[1:])
	if err != nil {
		return color.NRGBA{}, err
	}
	return color.NRGBA{R: b[0], G: b[1], B: b[2], A: 255}, nil
}

func Contrast(a, b string) (float64, error) {
	ca, err := Parse(a)
	if err != nil {
		return 0, err
	}
	cb, err := Parse(b)
	if err != nil {
		return 0, err
	}
	la, lb := luminance(ca), luminance(cb)
	if la < lb {
		la, lb = lb, la
	}
	return (la + .05) / (lb + .05), nil
}
func luminance(c color.NRGBA) float64 {
	f := func(v uint8) float64 {
		x := float64(v) / 255
		if x <= .04045 {
			return x / 12.92
		}
		return math.Pow((x+.055)/1.055, 2.4)
	}
	return .2126*f(c.R) + .7152*f(c.G) + .0722*f(c.B)
}

func TokenNames() []string { out := append([]string(nil), required...); sort.Strings(out); return out }
