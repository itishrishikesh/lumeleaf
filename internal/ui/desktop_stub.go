//go:build !desktop

package ui

import "fmt"

func RunDesktop(path, theme string) error {
	return fmt.Errorf("native desktop support is not present in this build; use make run after installing Gio system dependencies, or use --render-fixture")
}
