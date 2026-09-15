//go:build desktop

package ui

import (
	"image/color"
	"os"

	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	giotext "gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/itishrishikesh/lumeleaf/internal/buildinfo"
	"github.com/itishrishikesh/lumeleaf/internal/theme"
)

func RunDesktop(path, themeName string) error {
	done := make(chan error, 1)
	go func() { done <- window(path, themeName) }()
	app.Main()
	return <-done
}
func window(path, themeName string) error {
	p, err := theme.Load(themeName)
	if err != nil {
		return err
	}
	w := new(app.Window)
	w.Option(app.Title("Lumeleaf "+buildinfo.Version), app.Size(unit.Dp(1100), unit.Dp(760)))
	th := material.NewTheme()
	th.Shaper = giotext.NewShaper(giotext.WithCollection(gofont.Collection()))
	var ops op.Ops
	body := "Open a Java or Markdown file to begin."
	if path != "" {
		data, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		body = string(data)
	}
	bg, _ := theme.Parse(p.Background)
	fg, _ := theme.Parse(p.Text)
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			paintBackground(gtx, bg)
			label := material.Body1(th, body)
			label.Color = fg
			label.Font.Typeface = "Go Mono"
			layout.UniformInset(unit.Dp(24)).Layout(gtx, label.Layout)
			e.Frame(gtx.Ops)
		}
	}
}
func paintBackground(gtx layout.Context, c color.NRGBA) { paint.Fill(gtx.Ops, c) }
