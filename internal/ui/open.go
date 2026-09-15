package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/itishrishikesh/lumeleaf/internal/document"
	"github.com/itishrishikesh/lumeleaf/internal/workspace"
)

type OpenedFile struct {
	Path string
	Text string
}

type OpenedProject struct {
	Root  string
	Files []string
}

func ResolveOpenPath(input, base string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", errors.New("enter a file or project path")
	}
	if input == "~" || strings.HasPrefix(input, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		input = filepath.Join(home, strings.TrimPrefix(input, "~/"))
	}
	if !filepath.IsAbs(input) {
		if base == "" {
			var err error
			base, err = os.Getwd()
			if err != nil {
				return "", err
			}
		}
		input = filepath.Join(base, input)
	}
	abs, err := filepath.Abs(input)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func OpenTextFile(input, base string) (OpenedFile, error) {
	path, err := ResolveOpenPath(input, base)
	if err != nil {
		return OpenedFile{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return OpenedFile{}, err
	}
	if info.IsDir() {
		return OpenedFile{}, fmt.Errorf("%s is a directory; use Open Project", path)
	}
	doc, err := document.Open(path)
	if err != nil {
		return OpenedFile{}, fmt.Errorf("open %s: %w", path, err)
	}
	return OpenedFile{Path: path, Text: string(doc.Text())}, nil
}

func OpenProject(ctx context.Context, input, base string) (OpenedProject, error) {
	root, err := ResolveOpenPath(input, base)
	if err != nil {
		return OpenedProject{}, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return OpenedProject{}, err
	}
	if !info.IsDir() {
		return OpenedProject{}, fmt.Errorf("%s is a file; use Open File", root)
	}
	batches := make(chan []workspace.Entry)
	errCh := make(chan error, 1)
	go func() {
		errCh <- workspace.Walk(ctx, workspace.WalkOptions{
			Root:      root,
			Workers:   4,
			BatchSize: 256,
			Exclusions: []string{
				".git", ".idea", ".gradle", "build", "target", "out", "node_modules", "vendor",
			},
		}, batches)
	}()
	var files []string
	for batch := range batches {
		for _, entry := range batch {
			files = append(files, entry.Path)
		}
	}
	if err := <-errCh; err != nil {
		return OpenedProject{}, err
	}
	sort.Strings(files)
	return OpenedProject{Root: root, Files: files}, nil
}

func ProjectFile(root, relative string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, filepath.FromSlash(relative))
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("project file escapes workspace root")
	}
	return path, nil
}
