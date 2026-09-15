// Package gitreview provides the process and parsing boundary for local Git
// review features. It deliberately uses Git's stable machine-readable output
// and never invokes a shell.
package gitreview

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Runner runs an executable with an argument array. Implementations must not
// concatenate arguments into a shell command.
type Runner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

// ExecRunner is the production Runner.
type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s %q: %w: %s", name, args, err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// Service is a small, typed gateway to a local repository.
type Service struct {
	Repository string
	Git        string
	Runner     Runner
}

func NewService(repository string) *Service {
	return &Service{Repository: repository, Git: "git", Runner: ExecRunner{}}
}

func (s *Service) runner() Runner {
	if s.Runner != nil {
		return s.Runner
	}
	return ExecRunner{}
}

func (s *Service) git() string {
	if s.Git != "" {
		return s.Git
	}
	return "git"
}

func (s *Service) run(ctx context.Context, args ...string) ([]byte, error) {
	if s.Repository == "" {
		return nil, errors.New("gitreview: empty repository")
	}
	all := append([]string{"-C", s.Repository}, args...)
	return s.runner().Run(ctx, s.git(), all...)
}

// Status obtains porcelain v1 output in its NUL-delimited form.
func (s *Service) Status(ctx context.Context) ([]StatusEntry, error) {
	out, err := s.run(ctx, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	return ParsePorcelainV1Z(out)
}

// Diff returns Git's color-free unified diff. Baseline is passed to Git as a
// single revision argument; an empty baseline means the working-tree diff.
func (s *Service) Diff(ctx context.Context, baseline string) ([]FileDiff, error) {
	args := []string{"diff", "--no-ext-diff", "--no-color", "--find-renames", "--binary", "--unified=3"}
	if baseline != "" {
		args = append(args, baseline)
	}
	out, err := s.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	return ParseUnifiedDiff(out)
}

// StatusEntry is one entry in git status --porcelain=v1 -z output.
type StatusEntry struct {
	IndexStatus    byte
	WorktreeStatus byte
	Path           string
	OriginalPath   string // populated for renames and copies
}

func (e StatusEntry) IsUntracked() bool { return e.IndexStatus == '?' && e.WorktreeStatus == '?' }
func (e StatusEntry) IsIgnored() bool   { return e.IndexStatus == '!' && e.WorktreeStatus == '!' }
func (e StatusEntry) IsConflicted() bool {
	return (e.IndexStatus == 'U' || e.WorktreeStatus == 'U') ||
		(e.IndexStatus == 'A' && e.WorktreeStatus == 'A') ||
		(e.IndexStatus == 'D' && e.WorktreeStatus == 'D')
}
func (e StatusEntry) IsRenamed() bool { return e.IndexStatus == 'R' || e.WorktreeStatus == 'R' }
func (e StatusEntry) IsCopied() bool  { return e.IndexStatus == 'C' || e.WorktreeStatus == 'C' }
func (e StatusEntry) IsDeleted() bool { return e.IndexStatus == 'D' || e.WorktreeStatus == 'D' }

// ParsePorcelainV1Z parses only NUL-delimited porcelain v1. It preserves
// unusual path bytes in Go strings; callers must not apply quote decoding.
func ParsePorcelainV1Z(data []byte) ([]StatusEntry, error) {
	parts := bytes.Split(data, []byte{0})
	if len(parts) > 0 && len(parts[len(parts)-1]) == 0 {
		parts = parts[:len(parts)-1]
	}
	entries := make([]StatusEntry, 0, len(parts))
	for i := 0; i < len(parts); i++ {
		record := parts[i]
		if len(record) < 3 || record[2] != ' ' {
			return nil, fmt.Errorf("gitreview: invalid porcelain record %q", record)
		}
		e := StatusEntry{IndexStatus: record[0], WorktreeStatus: record[1], Path: string(record[3:])}
		if e.Path == "" {
			return nil, errors.New("gitreview: porcelain entry has empty path")
		}
		// In -z format Git reverses the human-readable rename order: the new
		// path is in this record and the old path is the following NUL field.
		if e.IsRenamed() || e.IsCopied() {
			i++
			if i >= len(parts) || len(parts[i]) == 0 {
				return nil, errors.New("gitreview: truncated rename porcelain entry")
			}
			e.OriginalPath = string(parts[i])
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// LineKind identifies a line inside a diff hunk.
type LineKind uint8

const (
	ContextLine LineKind = iota
	AdditionLine
	DeletionLine
	NoNewlineMarker
)

type DiffLine struct {
	Kind    LineKind
	Text    string // text does not include the diff prefix or trailing newline
	OldLine int    // zero when this side does not have a line
	NewLine int    // zero when this side does not have a line
}

type Hunk struct {
	OldStart, OldCount int
	NewStart, NewCount int
	Section            string
	Lines              []DiffLine
}

type FileDiff struct {
	OldPath, NewPath string
	IsNew, IsDeleted bool
	IsRenamed        bool
	IsBinary         bool
	Hunks            []Hunk
}

// ParseUnifiedDiff parses the portable subset emitted by git diff. Unknown
// metadata is retained only insofar as it affects file state; this prevents UI
// code from depending on Git's presentation-oriented quoting.
func ParseUnifiedDiff(data []byte) ([]FileDiff, error) {
	lines := splitDiffLines(data)
	var files []FileDiff
	var current *FileDiff
	var hunk *Hunk
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			if current != nil {
				files = append(files, *current)
			}
			old, newPath := parseGitHeader(strings.TrimPrefix(line, "diff --git "))
			current = &FileDiff{OldPath: old, NewPath: newPath}
			hunk = nil
		case current == nil:
			continue
		case strings.HasPrefix(line, "new file mode "):
			current.IsNew = true
		case strings.HasPrefix(line, "deleted file mode "):
			current.IsDeleted = true
		case strings.HasPrefix(line, "rename from "):
			current.IsRenamed = true
			current.OldPath = strings.TrimPrefix(line, "rename from ")
		case strings.HasPrefix(line, "rename to "):
			current.IsRenamed = true
			current.NewPath = strings.TrimPrefix(line, "rename to ")
		case strings.HasPrefix(line, "Binary files ") || line == "GIT binary patch":
			current.IsBinary = true
		case strings.HasPrefix(line, "--- "):
			current.OldPath = diffPath(strings.TrimPrefix(line, "--- "))
		case strings.HasPrefix(line, "+++ "):
			current.NewPath = diffPath(strings.TrimPrefix(line, "+++ "))
		case strings.HasPrefix(line, "@@ "):
			parsed, err := parseHunkHeader(line)
			if err != nil {
				return nil, err
			}
			current.Hunks = append(current.Hunks, parsed)
			hunk = &current.Hunks[len(current.Hunks)-1]
		case hunk != nil && line == "\\ No newline at end of file":
			hunk.Lines = append(hunk.Lines, DiffLine{Kind: NoNewlineMarker})
		case hunk != nil && len(line) > 0:
			if err := appendDiffLine(hunk, line); err != nil {
				return nil, err
			}
		}
	}
	if current != nil {
		files = append(files, *current)
	}
	return files, nil
}

func splitDiffLines(data []byte) []string {
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	data = bytes.TrimSuffix(data, []byte("\n"))
	if len(data) == 0 {
		return nil
	}
	return strings.Split(string(data), "\n")
}

func parseGitHeader(v string) (string, string) {
	// Git quotes unusual names here. The ---/+++ header is authoritative for
	// content diffs; this fallback handles ordinary headers without guessing.
	p := strings.SplitN(v, " ", 2)
	if len(p) != 2 {
		return "", ""
	}
	return diffPath(p[0]), diffPath(p[1])
}

func diffPath(v string) string {
	if v == "/dev/null" {
		return ""
	}
	if i := strings.IndexByte(v, '\t'); i >= 0 {
		v = v[:i]
	}
	if strings.HasPrefix(v, "a/") || strings.HasPrefix(v, "b/") {
		return v[2:]
	}
	return v
}

func parseHunkHeader(line string) (Hunk, error) {
	if !strings.HasPrefix(line, "@@ -") {
		return Hunk{}, fmt.Errorf("gitreview: invalid hunk header %q", line)
	}
	end := strings.Index(line[3:], " @@")
	if end < 0 {
		return Hunk{}, fmt.Errorf("gitreview: unterminated hunk header %q", line)
	}
	body := line[3 : 3+end]
	parts := strings.Split(body, " +")
	if len(parts) != 2 {
		return Hunk{}, fmt.Errorf("gitreview: invalid hunk ranges %q", line)
	}
	oldStart, oldCount, err := parseRange(strings.TrimPrefix(parts[0], "-"))
	if err != nil {
		return Hunk{}, err
	}
	newStart, newCount, err := parseRange(parts[1])
	if err != nil {
		return Hunk{}, err
	}
	return Hunk{OldStart: oldStart, OldCount: oldCount, NewStart: newStart, NewCount: newCount, Section: strings.TrimSpace(line[3+end+3:])}, nil
}

func parseRange(v string) (int, int, error) {
	p := strings.SplitN(v, ",", 2)
	start, err := strconv.Atoi(p[0])
	if err != nil || start < 0 {
		return 0, 0, fmt.Errorf("gitreview: invalid range %q", v)
	}
	count := 1
	if len(p) == 2 {
		count, err = strconv.Atoi(p[1])
		if err != nil || count < 0 {
			return 0, 0, fmt.Errorf("gitreview: invalid range %q", v)
		}
	}
	return start, count, nil
}

func appendDiffLine(h *Hunk, line string) error {
	old, new := h.OldStart, h.NewStart
	for _, prior := range h.Lines {
		switch prior.Kind {
		case ContextLine:
			old++
			new++
		case AdditionLine:
			new++
		case DeletionLine:
			old++
		}
	}
	entry := DiffLine{Text: line[1:]}
	switch line[0] {
	case ' ':
		entry.Kind, entry.OldLine, entry.NewLine = ContextLine, old, new
	case '+':
		entry.Kind, entry.NewLine = AdditionLine, new
	case '-':
		entry.Kind, entry.OldLine = DeletionLine, old
	default:
		return fmt.Errorf("gitreview: invalid hunk line %q", line)
	}
	h.Lines = append(h.Lines, entry)
	return nil
}

// HunkRef is a stable navigation target within a parsed diff.
type HunkRef struct{ File, Hunk int }

func OrderedHunks(files []FileDiff) []HunkRef {
	refs := make([]HunkRef, 0)
	for fi := range files {
		for hi := range files[fi].Hunks {
			refs = append(refs, HunkRef{File: fi, Hunk: hi})
		}
	}
	return refs
}

// NextHunk returns the deterministic next/previous hunk, wrapping at either
// end. It returns false only when no hunk exists.
func NextHunk(files []FileDiff, current HunkRef, previous bool) (HunkRef, bool) {
	refs := OrderedHunks(files)
	if len(refs) == 0 {
		return HunkRef{}, false
	}
	idx := -1
	for i, ref := range refs {
		if ref == current {
			idx = i
			break
		}
	}
	if idx < 0 {
		return refs[0], true
	}
	if previous {
		idx = (idx - 1 + len(refs)) % len(refs)
	} else {
		idx = (idx + 1) % len(refs)
	}
	return refs[idx], true
}

type ReviewState string

const (
	Unreviewed ReviewState = "unreviewed"
	Reviewed   ReviewState = "reviewed"
	NeedsWork  ReviewState = "needs_work"
)

// HunkIdentity does not include source line numbers. It therefore survives
// unrelated insertions/deletions and changes only when the hunk's actual
// reviewable content changes.
func HunkIdentity(path string, h Hunk) string {
	var b strings.Builder
	b.WriteString(path)
	b.WriteByte(0)
	b.WriteString(h.Section)
	b.WriteByte(0)
	for _, line := range h.Lines {
		if line.Kind == NoNewlineMarker {
			continue
		}
		b.WriteByte(byte(line.Kind))
		b.WriteString(line.Text)
		b.WriteByte(0)
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

type reviewDisk struct {
	States map[string]ReviewState `json:"states"`
}

// ReviewStore persists strictly local state as JSON. Its key is baseline plus
// a relocatable hunk identity. The on-disk format is intentionally tiny and
// migratable to bbolt without leaking through the package API.
type ReviewStore struct {
	path   string
	mu     sync.RWMutex
	states map[string]ReviewState
}

func OpenReviewStore(path string) (*ReviewStore, error) {
	s := &ReviewStore{path: path, states: make(map[string]ReviewState)}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var disk reviewDisk
	if err := json.Unmarshal(b, &disk); err != nil {
		return nil, fmt.Errorf("gitreview: read review state: %w", err)
	}
	if disk.States != nil {
		s.states = disk.States
	}
	return s, nil
}

func reviewKey(baseline, identity string) string { return baseline + "\x00" + identity }

func (s *ReviewStore) Get(baseline, identity string) ReviewState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if state, ok := s.states[reviewKey(baseline, identity)]; ok {
		return state
	}
	return Unreviewed
}

func (s *ReviewStore) Set(baseline, identity string, state ReviewState) error {
	if state != Unreviewed && state != Reviewed && state != NeedsWork {
		return fmt.Errorf("gitreview: invalid review state %q", state)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := reviewKey(baseline, identity)
	if state == Unreviewed {
		delete(s.states, key)
	} else {
		s.states[key] = state
	}
	return s.saveLocked()
}

func (s *ReviewStore) saveLocked() error {
	if s.path == "" {
		return errors.New("gitreview: empty review state path")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(reviewDisk{States: s.states}, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".review-state-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err = tmp.Write(append(b, '\n')); err == nil {
		err = tmp.Chmod(0o600)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmpName, s.path)
}

func (s *ReviewStore) Close() error { return nil }

// States returns an ordered copy for diagnostics and tests.
func (s *ReviewStore) States() []ReviewRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]ReviewRecord, 0, len(s.states))
	for key, state := range s.states {
		base, identity, _ := strings.Cut(key, "\x00")
		result = append(result, ReviewRecord{Baseline: base, Identity: identity, State: state})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Baseline == result[j].Baseline {
			return result[i].Identity < result[j].Identity
		}
		return result[i].Baseline < result[j].Baseline
	})
	return result
}

type ReviewRecord struct {
	Baseline, Identity string
	State              ReviewState
}

// CopyTo writes a JSON snapshot to dst. It is provided for future migration
// tooling and makes the currently local nature of review state explicit.
func (s *ReviewStore) CopyTo(dst io.Writer) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return json.NewEncoder(dst).Encode(reviewDisk{States: s.states})
}
