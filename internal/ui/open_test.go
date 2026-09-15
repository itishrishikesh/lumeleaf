package ui

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestOpenTextFileAndProject(t *testing.T) {
	root := t.TempDir()
	write := func(name, text string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("src/Reader.java", "record Reader() {}\n")
	write("README.md", "# Project\n")
	write("target/generated.txt", "ignored\n")
	write(".git/config", "ignored\n")

	opened, err := OpenTextFile("src/Reader.java", root)
	if err != nil {
		t.Fatal(err)
	}
	if opened.Text != "record Reader() {}\n" || opened.Path != filepath.Join(root, "src", "Reader.java") {
		t.Fatalf("opened = %#v", opened)
	}

	project, err := OpenProject(context.Background(), root, "")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"README.md", "src/Reader.java"}
	if !reflect.DeepEqual(project.Files, want) {
		t.Fatalf("files = %v, want %v", project.Files, want)
	}
}

func TestOpenPathValidation(t *testing.T) {
	root := t.TempDir()
	if _, err := OpenTextFile(root, ""); err == nil {
		t.Fatal("OpenTextFile accepted a directory")
	}
	file := filepath.Join(root, "a.java")
	if err := os.WriteFile(file, []byte("class A {}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenProject(context.Background(), file, ""); err == nil {
		t.Fatal("OpenProject accepted a file")
	}
	if _, err := ProjectFile(root, "../outside"); err == nil {
		t.Fatal("ProjectFile accepted an escaping path")
	}
}

func TestResolveOpenPathExpandsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	got, err := ResolveOpenPath("~/Reader.java", "")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "Reader.java"); got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}
