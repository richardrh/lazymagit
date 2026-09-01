package quality

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyzeDirectInputFailures(t *testing.T) {
	t.Run("missing module", func(t *testing.T) {
		if _, err := Analyze(t.TempDir(), filepath.Join(t.TempDir(), "cover.out")); err == nil {
			t.Fatal("Analyze accepted a root without go.mod")
		}
	})

	t.Run("invalid production source", func(t *testing.T) {
		root := t.TempDir()
		writeTestFile(t, filepath.Join(root, "go.mod"), "module example.com/broken\n")
		writeTestFile(t, filepath.Join(root, "broken.go"), "package broken\nfunc (")
		profile := filepath.Join(root, "cover.out")
		writeTestFile(t, profile, "mode: set\n")
		if _, err := Analyze(root, profile); err == nil {
			t.Fatal("Analyze accepted invalid Go source")
		}
	})

	t.Run("missing coverprofile", func(t *testing.T) {
		root := t.TempDir()
		writeTestFile(t, filepath.Join(root, "go.mod"), "module example.com/project\n")
		writeTestFile(t, filepath.Join(root, "work.go"), "package project\nfunc Work() {}\n")
		if _, err := Analyze(root, filepath.Join(root, "missing.out")); err == nil {
			t.Fatal("Analyze accepted a missing coverprofile")
		}
	})
}

func TestAnalyzeDirectIgnoresUnmatchedCoverageBlocks(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "go.mod"), "module example.com/project\n")
	writeTestFile(t, filepath.Join(root, "work.go"), "package project\nfunc Work() {}\n")
	profile := filepath.Join(root, "cover.out")
	if err := os.WriteFile(profile, []byte("mode: set\nexample.com/project/other.go:1.1,1.2 1 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	functions, err := Analyze(root, profile)
	if err != nil {
		t.Fatal(err)
	}
	if len(functions) != 1 || functions[0].Coverage != 0 || functions[0].CRAP != Score(functions[0].Complexity, 0) {
		t.Fatalf("Analyze unmatched block = %#v", functions)
	}
}
