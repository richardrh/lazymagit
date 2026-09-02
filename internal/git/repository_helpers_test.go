package git

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSummary(t *testing.T) {
	out := []byte(strings.Join([]string{
		"# branch.oid abc123",
		"# branch.head main",
		"# branch.upstream origin/main",
		"# branch.ab +3 -2",
	}, "\x00") + "\x00")
	got := parseSummary(out)
	want := Summary{Head: "abc123", Branch: "main", Upstream: "origin/main", Ahead: 3, Behind: 2}
	if got != want {
		t.Fatalf("parseSummary() = %#v, want %#v", got, want)
	}

	got = parseSummary([]byte("# branch.oid (initial)\x00# branch.head (detached)\x00"))
	if !got.Unborn || !got.Detached {
		t.Fatalf("special summary flags = %#v", got)
	}
}

func TestSafeNewDirectoryHelpers(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := validateNewDirectoryDestination(empty); err != nil {
		t.Fatalf("empty destination: %v", err)
	}
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{file, filepath.Join(dir, "missing", "destination")} {
		if err := validateNewDirectoryDestination(path); (err == nil) != (path != file) {
			t.Errorf("validateNewDirectoryDestination(%q) = %v", path, err)
		}
	}
	if err := os.WriteFile(filepath.Join(empty, "entry"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateNewDirectoryDestination(empty); !errors.Is(err, ErrUnsafeDestination) {
		t.Fatalf("non-empty destination error = %v", err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(empty, link); err != nil {
		t.Fatal(err)
	}
	if err := validateNewDirectoryDestination(link); !errors.Is(err, ErrUnsafeDestination) {
		t.Fatalf("symlink destination error = %v", err)
	}
	if err := validateNewDirectoryParent(filepath.Join(dir, "missing", "nested")); err != nil {
		t.Fatalf("existing ancestor: %v", err)
	}
	if err := validateNewDirectoryParent(filepath.Join(file, "nested")); err == nil {
		t.Fatal("file parent was accepted")
	}
}

func TestSubtreeHelpers(t *testing.T) {
	valid := map[string]SubtreeOptions{
		"add":   {Prefix: "vendor/lib", Repository: "origin", Ref: "main", Squash: true, Message: "add"},
		"merge": {Prefix: "vendor/lib", Ref: "topic", Squash: true, Message: "merge"},
		"pull":  {Prefix: "vendor/lib", Repository: "origin", Ref: "main", Squash: true},
		"push":  {Prefix: "vendor/lib", Repository: "origin", Ref: "main"},
		"split": {Prefix: "vendor/lib", Branch: "split"},
	}
	for action, options := range valid {
		if err := validateSubtreeOptions(action, options); err != nil {
			t.Errorf("validateSubtreeOptions(%s): %v", action, err)
		}
		args := subtreeArgs(action, options)
		if len(args) < 4 || args[0] != "subtree" || args[1] != action || args[3] != options.Prefix {
			t.Errorf("subtreeArgs(%s) = %#v", action, args)
		}
	}
	if err := validateSubtreeTokens("add", SubtreeOptions{Repository: "bad\nvalue"}); err == nil {
		t.Fatal("control character accepted")
	}
	if got := validateSubtreeAdd(SubtreeOptions{}); got != "ref is required" {
		t.Fatalf("validateSubtreeAdd = %q", got)
	}
	if got := validateSubtreeMerge(SubtreeOptions{Ref: "main", Repository: "origin"}); got != "repository and branch are not supported" {
		t.Fatalf("validateSubtreeMerge = %q", got)
	}
	if got := validateSubtreeTransfer("push", SubtreeOptions{Repository: "origin", Ref: "main", Squash: true}); got != "unsupported options" {
		t.Fatalf("validateSubtreeTransfer = %q", got)
	}
	if got := validateSubtreeSplit(SubtreeOptions{Message: "message"}); got != "repository, ref, squash, and message are not supported" {
		t.Fatalf("validateSubtreeSplit = %q", got)
	}
}

func TestPathExpansionAndExclusiveFileCopy(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	absolute := filepath.Join(t.TempDir(), "..", "absolute")
	for input, want := range map[string]string{
		"~":        home,
		"~/nested": filepath.Join(home, "nested"),
		`~\nested`: filepath.Join(home, "nested"),
		absolute:   filepath.Clean(absolute),
	} {
		got, err := expandUserPath(input)
		if err != nil || got != want {
			t.Errorf("expandUserPath(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	if _, err := expandUserPath("relative/path"); err == nil {
		t.Fatal("relative global excludes path was accepted")
	}

	dir := t.TempDir()
	source, destination := filepath.Join(dir, "source"), filepath.Join(dir, "destination")
	if err := os.WriteFile(source, []byte("copied\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := copyFileExclusive(source, destination); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(destination); err != nil || string(got) != "copied\n" {
		t.Fatalf("copied file = %q, %v", got, err)
	}
	if err := copyFileExclusive(source, destination); !errors.Is(err, os.ErrExist) {
		t.Fatalf("exclusive copy error = %v", err)
	}
	if err := copyFileExclusive(filepath.Join(dir, "missing"), filepath.Join(dir, "unused")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing source error = %v", err)
	}
}
