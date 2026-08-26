package git

import (
	"bytes"
	"testing"
)

func FuzzParseUnifiedDiffReparseHunk(f *testing.F) {
	f.Add([]byte("diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1 +1 @@\n-old\n+new\n"))
	f.Add([]byte("diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -0,0 +1 @@\n+x\n\\ No newline at end of file\n"))
	f.Fuzz(func(t *testing.T, patch []byte) {
		if len(patch) > 64<<10 {
			t.Skip()
		}
		doc, err := ParseUnifiedDiff(patch)
		if err != nil {
			return
		}
		if len(doc.Files) > 64 {
			t.Skip()
		}
		for fileIndex := range doc.Files {
			if len(doc.Files[fileIndex].Hunks) > 64 {
				t.Skip()
			}
			for hunkIndex := range doc.Files[fileIndex].Hunks {
				hunkPatch, err := doc.HunkPatch(fileIndex, hunkIndex)
				if err != nil { // Binary and rename patches are intentionally immutable.
					continue
				}
				reparsed, err := ParseUnifiedDiff(hunkPatch)
				if err != nil {
					t.Fatalf("reparse rendered hunk: %v\n%s", err, hunkPatch)
				}
				if len(reparsed.Files) != 1 || len(reparsed.Files[0].Hunks) != 1 {
					t.Fatalf("rendered hunk parsed as %d files", len(reparsed.Files))
				}
				again, err := reparsed.HunkPatch(0, 0)
				if err != nil || !bytes.Equal(again, hunkPatch) {
					t.Fatalf("hunk rendering is not stable: err=%v", err)
				}
			}
		}
	})
}
