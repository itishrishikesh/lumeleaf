//go:build desktop

package ui

import (
	"context"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strings"

	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	giotext "gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/itishrishikesh/lumeleaf/internal/buildinfo"
	"github.com/itishrishikesh/lumeleaf/internal/theme"
)

type projectLoad struct {
	project OpenedProject
	err     error
}

type desktopState struct {
	pathEditor            widget.Editor
	openFile, openProject widget.Clickable
	fileList, contentList widget.List
	fileClicks            []widget.Clickable
	projectResults        chan projectLoad
	root, current, status string
	files, lines          []string
	loading               bool
}

func newDesktopState() *desktopState {
	s := &desktopState{projectResults: make(chan projectLoad, 1)}
	s.pathEditor.SingleLine = true
	s.pathEditor.Submit = true
	s.fileList.Axis = layout.Vertical
	s.contentList.Axis = layout.Vertical
	s.lines = []string{"Open a Java or Markdown file, or enter a directory and choose Open Project."}
	s.status = "Ready — enter a path above"
	if cwd, err := os.Getwd(); err == nil {
		s.pathEditor.SetText(cwd)
	}
	return s
}

func RunDesktop(path, themeName string) error {
	done := make(chan error, 1)
	go func() { done <- window(path, themeName) }()
	app.Main()
	return <-done
}

func window(initialPath, themeName string) error {
	p, err := theme.Load(themeName)
	if err != nil {
		return err
	}
	w := new(app.Window)
	w.Option(app.Title("Lumeleaf "+buildinfo.Version), app.Size(unit.Dp(1180), unit.Dp(780)))
	th := material.NewTheme()
	th.Shaper = giotext.NewShaper(giotext.WithCollection(gofont.Collection()))
	state := newDesktopState()
	if initialPath != "" {
		state.pathEditor.SetText(initialPath)
		resolved, resolveErr := ResolveOpenPath(initialPath, "")
		if resolveErr != nil {
			state.status = resolveErr.Error()
		} else if info, statErr := os.Stat(resolved); statErr != nil {
			state.status = statErr.Error()
		} else if info.IsDir() {
			startProjectLoad(w, state, resolved)
		} else {
			openFile(state, resolved)
		}
	}

	bg, _ := theme.Parse(p.Background)
	surface, _ := theme.Parse(p.Surface)
	raised, _ := theme.Parse(p.SurfaceRaised)
	fg, _ := theme.Parse(p.Text)
	muted, _ := theme.Parse(p.Muted)
	accent, _ := theme.Parse(p.Accent)
	selection, _ := theme.Parse(p.Selection)
	th.Palette.Bg = bg
	th.Palette.Fg = fg
	th.Palette.ContrastBg = accent
	th.Palette.ContrastFg = bg

	var ops op.Ops
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			processDesktopEvents(w, state, gtx)
			paintBackground(gtx, bg)
			layoutDesktop(gtx, th, state, bg, surface, raised, fg, muted, accent, selection)
			e.Frame(gtx.Ops)
		}
	}
}

func processDesktopEvents(w *app.Window, state *desktopState, gtx layout.Context) {
	select {
	case result := <-state.projectResults:
		state.loading = false
		if result.err != nil {
			state.status = "Could not open project: " + result.err.Error()
			break
		}
		state.root = result.project.Root
		state.files = result.project.Files
		state.fileClicks = make([]widget.Clickable, len(state.files))
		state.pathEditor.SetText(state.root)
		state.status = fmt.Sprintf("Project opened — %d readable files", len(state.files))
		if len(state.files) > 0 {
			selected := 0
			for i, name := range state.files {
				if strings.EqualFold(name, "README.md") {
					selected = i
					break
				}
			}
			openProjectFile(state, selected)
		}
	default:
	}

	for {
		event, ok := gtx.Event(key.Filter{Name: "O", Required: key.ModShortcut})
		if !ok {
			break
		}
		if event, ok := event.(key.Event); ok && event.State == key.Press {
			gtx.Execute(key.FocusCmd{Tag: &state.pathEditor})
		}
	}
	for {
		event, ok := state.pathEditor.Update(gtx)
		if !ok {
			break
		}
		if _, ok := event.(widget.SubmitEvent); ok {
			openEnteredPath(w, state)
		}
	}
	for state.openFile.Clicked(gtx) {
		openFile(state, state.pathEditor.Text())
	}
	for state.openProject.Clicked(gtx) {
		startProjectLoad(w, state, state.pathEditor.Text())
	}
	for i := range state.fileClicks {
		for state.fileClicks[i].Clicked(gtx) {
			openProjectFile(state, i)
		}
	}
}

func openEnteredPath(w *app.Window, state *desktopState) {
	resolved, err := ResolveOpenPath(state.pathEditor.Text(), state.root)
	if err != nil {
		state.status = err.Error()
		return
	}
	info, err := os.Stat(resolved)
	if err != nil {
		state.status = err.Error()
		return
	}
	if info.IsDir() {
		startProjectLoad(w, state, resolved)
		return
	}
	openFile(state, resolved)
}

func startProjectLoad(w *app.Window, state *desktopState, input string) {
	if state.loading {
		return
	}
	state.loading = true
	state.status = "Opening project…"
	base := state.root
	go func() {
		project, err := OpenProject(context.Background(), input, base)
		state.projectResults <- projectLoad{project: project, err: err}
		w.Invalidate()
	}()
}

func openProjectFile(state *desktopState, index int) {
	if index < 0 || index >= len(state.files) {
		return
	}
	path, err := ProjectFile(state.root, state.files[index])
	if err != nil {
		state.status = err.Error()
		return
	}
	openFile(state, path)
}

func openFile(state *desktopState, input string) {
	opened, err := OpenTextFile(input, state.root)
	if err != nil {
		state.status = "Could not open file: " + err.Error()
		return
	}
	state.current = opened.Path
	state.lines = strings.Split(opened.Text, "\n")
	if len(state.lines) == 0 {
		state.lines = []string{""}
	}
	state.contentList.Position = layout.Position{}
	state.pathEditor.SetText(opened.Path)
	state.status = fmt.Sprintf("%s — %d lines", opened.Path, len(state.lines))
}

func layoutDesktop(gtx layout.Context, th *material.Theme, state *desktopState, bg, surface, raised, fg, muted, accent, selection color.NRGBA) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return colored(gtx, raised, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: 10, Bottom: 10, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							label := material.H6(th, "Lumeleaf")
							label.Color = fg
							return label.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(18)}.Layout),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return colored(gtx, surface, func(gtx layout.Context) layout.Dimensions {
								editor := material.Editor(th, &state.pathEditor, "File or project path")
								editor.Color, editor.HintColor, editor.SelectionColor = fg, muted, selection
								return layout.Inset{Top: 8, Bottom: 8, Left: 10, Right: 10}.Layout(gtx, editor.Layout)
							})
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							button := material.Button(th, &state.openFile, "Open File  ⌘/Ctrl O")
							button.Background, button.Color = accent, contrastText(accent)
							return button.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							button := material.Button(th, &state.openProject, "Open Project")
							button.Background, button.Color = surface, fg
							return button.Layout(gtx)
						}),
					)
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return colored(gtx, surface, func(gtx layout.Context) layout.Dimensions {
				label := material.Caption(th, state.status)
				label.Color = muted
				return layout.Inset{Top: 6, Bottom: 6, Left: 16, Right: 16}.Layout(gtx, label.Layout)
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Dp(unit.Dp(280))
					gtx.Constraints.Max.X = gtx.Constraints.Min.X
					return colored(gtx, surface, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: 14, Bottom: 14, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									title := "PROJECT"
									if state.root != "" {
										title = strings.ToUpper(filepath.Base(state.root))
									}
									label := material.Caption(th, title)
									label.Color = muted
									return layout.Inset{Bottom: 10}.Layout(gtx, label.Layout)
								}),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return state.fileList.List.Layout(gtx, len(state.files), func(gtx layout.Context, index int) layout.Dimensions {
										return state.fileClicks[index].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											label := material.Body2(th, state.files[index])
											label.Color = fg
											return layout.Inset{Top: 6, Bottom: 6, Left: 6, Right: 4}.Layout(gtx, label.Layout)
										})
									})
								}),
							)
						})
					})
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return colored(gtx, bg, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: 14, Bottom: 14, Left: 18, Right: 18}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									name := "No file selected"
									if state.current != "" {
										name = filepath.Base(state.current)
									}
									label := material.H6(th, name)
									label.Color = fg
									return layout.Inset{Bottom: 12}.Layout(gtx, label.Layout)
								}),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return state.contentList.List.Layout(gtx, len(state.lines), func(gtx layout.Context, index int) layout.Dimensions {
										return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Baseline}.Layout(gtx,
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												label := material.Body2(th, fmt.Sprintf("%5d", index+1))
												label.Color = muted
												label.Font.Typeface = "Go Mono"
												return layout.Inset{Right: 14}.Layout(gtx, label.Layout)
											}),
											layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
												label := material.Body2(th, state.lines[index])
												label.Color = fg
												label.Font.Typeface = "Go Mono"
												return layout.Inset{Bottom: 3}.Layout(gtx, label.Layout)
											}),
										)
									})
								}),
							)
						})
					})
				}),
			)
		}),
	)
}

func colored(gtx layout.Context, background color.NRGBA, content layout.Widget) layout.Dimensions {
	return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		size := gtx.Constraints.Max
		paint.FillShape(gtx.Ops, background, clip.Rect{Max: size}.Op())
		return layout.Dimensions{Size: size}
	}, content)
}

func contrastText(c color.NRGBA) color.NRGBA {
	brightness := uint32(c.R)*299 + uint32(c.G)*587 + uint32(c.B)*114
	if brightness > 128000 {
		return color.NRGBA{R: 24, G: 27, B: 31, A: 255}
	}
	return color.NRGBA{R: 250, G: 248, B: 242, A: 255}
}

func paintBackground(gtx layout.Context, c color.NRGBA) {
	paint.FillShape(gtx.Ops, c, clip.Rect{Max: gtx.Constraints.Max}.Op())
}
