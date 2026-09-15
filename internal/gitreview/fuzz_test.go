package gitreview

import "testing"

func FuzzGitPorcelainParser(f *testing.F) {
	f.Add([]byte("?? ordinary.txt\x00"))
	f.Add([]byte("R  new\x00old\x00"))
	f.Fuzz(func(t *testing.T, data []byte) { _, _ = ParsePorcelainV1Z(data) })
}

func FuzzDiffParser(f *testing.F) {
	f.Add([]byte("diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1 +1 @@\n-a\n+b\n"))
	f.Fuzz(func(t *testing.T, data []byte) { _, _ = ParseUnifiedDiff(data) })
}
