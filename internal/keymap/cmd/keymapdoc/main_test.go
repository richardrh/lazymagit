package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateAndCheck(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "docs"), 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "docs", "keybindings.md")
	if err := os.WriteFile(path, []byte("stale"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := generate(root, true); err == nil {
		t.Fatal("-check accepted stale documentation")
	}
	if err := generate(root, false); err != nil {
		t.Fatal(err)
	}
	if err := generate(root, true); err != nil {
		t.Fatal(err)
	}
}
