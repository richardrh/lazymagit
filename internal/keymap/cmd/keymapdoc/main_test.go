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

func TestRepositoryRootFromNestedDirectoryAndMissingModule(t *testing.T) {
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(original)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(nested); err != nil {
		t.Fatal(err)
	}
	if got, err := repositoryRoot(); err != nil || got != root {
		t.Fatalf("repositoryRoot() = %q, %v", got, err)
	}
	missing := t.TempDir()
	if err := os.Chdir(missing); err != nil {
		t.Fatal(err)
	}
	if _, err := repositoryRoot(); err == nil {
		t.Fatal("repositoryRoot found a module where none exists")
	}
}
