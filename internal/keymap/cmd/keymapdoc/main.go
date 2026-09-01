package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/richardrh/lazymagit/internal/keymap"
)

func main() {
	check := flag.Bool("check", false, "fail if docs/keybindings.md differs from generated output")
	flag.Parse()
	root, err := repositoryRoot()
	if err != nil {
		fatal(err)
	}
	if err := generate(root, *check); err != nil {
		fatal(err)
	}
}

func generate(root string, check bool) error {
	want, err := keymap.RenderLedger()
	if err != nil {
		return err
	}
	path := filepath.Join(root, "docs", "keybindings.md")
	if check {
		got, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(got, []byte(want)) {
			return fmt.Errorf("%s is stale; run go run ./internal/keymap/cmd/keymapdoc", path)
		}
		return nil
	}
	return os.WriteFile(path, []byte(want), 0644)
}

func repositoryRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found")
		}
		dir = parent
	}
}

func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
