package workspace

import (
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type QuickOpenEntry struct {
	Path       string
	Score      float64
	LastOpened time.Time
	Changed    bool
}
type QuickOpenQuery struct {
	Text       string
	Limit      int
	Now        time.Time
	CurrentDir string
}

// RankQuickOpen scores every entry without mutating the index. Matching is
// case-insensitive and rewards basename prefixes, path prefixes, recency,
// changed files, and files near CurrentDir.
func RankQuickOpen(paths []string, q QuickOpenQuery, metadata map[string]QuickOpenEntry) []QuickOpenEntry {
	needle := strings.ToLower(strings.TrimSpace(q.Text))
	if q.Limit <= 0 {
		q.Limit = 50
	}
	if q.Now.IsZero() {
		q.Now = time.Now()
	}
	out := make([]QuickOpenEntry, 0, len(paths))
	for _, p := range paths {
		low := strings.ToLower(filepath.ToSlash(p))
		base := strings.ToLower(filepath.Base(p))
		score := 0.0
		if needle != "" {
			if !strings.Contains(low, needle) {
				continue
			}
			if strings.HasPrefix(base, needle) {
				score += 100
			}
			if strings.HasPrefix(low, needle) {
				score += 45
			}
			score += 20 * float64(len(needle)) / float64(len(base)+1)
			score -= float64(strings.Count(low, "/")) * 0.05
		}
		m := metadata[p]
		m.Path = p
		if m.Changed {
			score += 18
		}
		if !m.LastOpened.IsZero() {
			age := q.Now.Sub(m.LastOpened).Hours()
			if age < 0 {
				age = 0
			}
			score += 12 / (1 + age/24)
		}
		if q.CurrentDir != "" {
			rel, e := filepath.Rel(q.CurrentDir, p)
			if e == nil && !strings.HasPrefix(rel, "..") {
				score += 8 / (1 + float64(strings.Count(rel, string(filepath.Separator))))
			}
		}
		m.Score = score
		out = append(out, m)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Path < out[j].Path
	})
	if len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out
}
