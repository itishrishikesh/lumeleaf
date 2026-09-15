package buildinfo

import "testing"

func TestString(t *testing.T) {
	oldVersion, oldCommit, oldDate := Version, Commit, Date
	t.Cleanup(func() { Version, Commit, Date = oldVersion, oldCommit, oldDate })

	Version, Commit, Date = "1.0.0", "abc1234", "2026-09-16T00:00:00Z"
	if got, want := String(), "Lumeleaf 1.0.0 (abc1234, 2026-09-16T00:00:00Z)"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	Commit = "unknown"
	if got, want := String(), "Lumeleaf 1.0.0"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}
