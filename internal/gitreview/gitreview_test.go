package gitreview

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParsePorcelainV1Z(t *testing.T) {
	data := []byte(" M ordinary.txt\x00R  new name.txt\x00old name.txt\x00?? --leading-and-😀\x00UU conflict\x00")
	got, err := ParsePorcelainV1Z(data)
	if err != nil {
		t.Fatal(err)
	}
	want := []StatusEntry{
		{IndexStatus: ' ', WorktreeStatus: 'M', Path: "ordinary.txt"},
		{IndexStatus: 'R', WorktreeStatus: ' ', Path: "new name.txt", OriginalPath: "old name.txt"},
		{IndexStatus: '?', WorktreeStatus: '?', Path: "--leading-and-😀"},
		{IndexStatus: 'U', WorktreeStatus: 'U', Path: "conflict"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParsePorcelainV1Z() = %#v, want %#v", got, want)
	}
	if !got[3].IsConflicted() || !got[2].IsUntracked() || !got[1].IsRenamed() {
		t.Fatal("status classification failed")
	}
}

func TestParsePorcelainV1ZRejectsTruncatedRename(t *testing.T) {
	if _, err := ParsePorcelainV1Z([]byte("R  new\x00")); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseUnifiedDiffAndNavigation(t *testing.T) {
	diff := "diff --git a/a.txt b/a.txt\nindex 000..111 100644\n--- a/a.txt\n+++ b/a.txt\n@@ -4,2 +9,3 @@ section\n same\n-old\n+new\n+extra\n@@ -20 +25 @@\n-old2\n+new2\n\\ No newline at end of file\n"
	files, err := ParseUnifiedDiff([]byte(diff))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || len(files[0].Hunks) != 2 {
		t.Fatalf("unexpected diff: %#v", files)
	}
	h := files[0].Hunks[0]
	if h.OldStart != 4 || h.NewStart != 9 || h.Lines[0].OldLine != 4 || h.Lines[1].OldLine != 5 || h.Lines[2].NewLine != 10 {
		t.Fatalf("bad line coordinates: %#v", h.Lines)
	}
	first, ok := NextHunk(files, HunkRef{File: 0, Hunk: 1}, false)
	if !ok || first != (HunkRef{}) {
		t.Fatalf("wrapped next = %#v", first)
	}
	previous, ok := NextHunk(files, HunkRef{}, true)
	if !ok || previous != (HunkRef{File: 0, Hunk: 1}) {
		t.Fatalf("wrapped previous = %#v", previous)
	}
}

func TestHunkIdentityRelocatesButChangesWhenContentChanges(t *testing.T) {
	a := Hunk{OldStart: 1, NewStart: 10, Section: "f", Lines: []DiffLine{{Kind: ContextLine, Text: "nearby"}, {Kind: DeletionLine, Text: "old"}, {Kind: AdditionLine, Text: "new"}}}
	b := a
	b.Lines = append([]DiffLine(nil), a.Lines...)
	b.OldStart, b.NewStart = 100, 200
	if HunkIdentity("x.go", a) != HunkIdentity("x.go", b) {
		t.Fatal("line movement changed identity")
	}
	b.Lines[2].Text = "other"
	if HunkIdentity("x.go", a) == HunkIdentity("x.go", b) {
		t.Fatal("content change did not invalidate identity")
	}
}

func TestReviewStorePersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "review.json")
	store, err := OpenReviewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set("head", "hunk", Reviewed); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenReviewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Get("head", "hunk"); got != Reviewed {
		t.Fatalf("state = %q", got)
	}
	if err := reopened.Set("head", "hunk", Unreviewed); err != nil {
		t.Fatal(err)
	}
	if got := reopened.Get("head", "hunk"); got != Unreviewed {
		t.Fatalf("state = %q", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
}

type captureRunner struct {
	name   string
	args   []string
	output []byte
	err    error
}

func (r *captureRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.name, r.args = name, append([]string(nil), args...)
	return r.output, r.err
}

func TestServiceUsesArgumentArray(t *testing.T) {
	runner := &captureRunner{output: []byte("?? path with space\x00")}
	s := &Service{Repository: "--repo with spaces", Git: "git", Runner: runner}
	if _, err := s.Status(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{"-C", "--repo with spaces", "status", "--porcelain=v1", "-z", "--untracked-files=all"}
	if runner.name != "git" || !reflect.DeepEqual(runner.args, want) {
		t.Fatalf("run %q %#v", runner.name, runner.args)
	}
	runner.err = errors.New("boom")
	if _, err := s.Status(context.Background()); err == nil {
		t.Fatal("expected runner error")
	}
}

func BenchmarkParsePorcelainV1Z(b *testing.B) {
	data := []byte(" M src/one.go\x00R  renamed.go\x00old.go\x00?? unicode-😀.txt\x00")
	b.ReportAllocs()
	for b.Loop() {
		_, _ = ParsePorcelainV1Z(data)
	}
}

func BenchmarkParseUnifiedDiff(b *testing.B) {
	data := []byte("diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1,2 +1,3 @@\n a\n-b\n+c\n+d\n")
	b.ReportAllocs()
	for b.Loop() {
		_, _ = ParseUnifiedDiff(data)
	}
}
