package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverClassifiesOnlyNotRepository(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()

	_, err := Discover(dir)
	if !errors.Is(err, ErrNotRepository) {
		t.Fatalf("Discover(empty directory) error = %v, want ErrNotRepository", err)
	}

	_, err = Discover(filepath.Join(dir, "missing"))
	if err == nil || errors.Is(err, ErrNotRepository) {
		t.Fatalf("Discover(missing directory) error = %v, want preserved non-repository-unrelated error", err)
	}
}

func TestInitCreatesRepositoryInExactExistingDirectory(t *testing.T) {
	requireGit(t)
	parent := newTestRepo(t)
	nested := filepath.Join(parent.dir, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	repo, err := Init(context.Background(), nested)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if got, want := repo.WorkTree(), nested; got != want {
		t.Fatalf("WorkTree() = %q, want exact initialized directory %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(nested, ".git")); err != nil {
		t.Fatalf("nested .git was not created: %v", err)
	}
}

func TestInitRequiresExistingDirectory(t *testing.T) {
	requireGit(t)
	parent := t.TempDir()
	file := filepath.Join(parent, "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	for name, path := range map[string]string{
		"missing": filepath.Join(parent, "missing"),
		"file":    file,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Init(context.Background(), path); err == nil {
				t.Fatalf("Init(%q) succeeded", path)
			}
			if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
				t.Fatalf("Init(%q) created .git", path)
			}
		})
	}
}

func TestDiscoverFromNestedDirectory(t *testing.T) {
	r := newTestRepo(t)
	nested := filepath.Join(r.dir, "one", "two")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	repo, err := Discover(nested)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if got, want := repo.WorkTree(), r.dir; got != want {
		t.Fatalf("WorkTree() = %q, want %q", got, want)
	}
	if repo.IsBare() {
		t.Fatal("ordinary repository reported as bare")
	}
	if got, want := repo.GitDir(), filepath.Join(r.dir, ".git"); got != want {
		t.Fatalf("GitDir() = %q, want %q", got, want)
	}
}

func TestDiscoverBareRepository(t *testing.T) {
	r := newBareTestRepo(t)
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if !repo.IsBare() {
		t.Fatal("bare repository reported as non-bare")
	}
	if got := repo.WorkTree(); got != "" {
		t.Fatalf("WorkTree() = %q, want empty for bare repository", got)
	}
	if got := repo.GitDir(); got != r.dir {
		t.Fatalf("GitDir() = %q, want %q", got, r.dir)
	}
}

func TestStatusRepresentsPorcelainV2FileStates(t *testing.T) {
	r := newTestRepo(t)
	r.write("both.txt", "base\n")
	r.write("unstaged.txt", "base\n")
	r.commitAll("base")

	// both.txt has distinct staged and unstaged changes.
	r.write("both.txt", "in index\n")
	r.git("add", "--", "both.txt")
	r.write("both.txt", "in worktree\n")
	r.write("unstaged.txt", "changed\n")
	r.write("staged new.txt", "new\n")
	r.git("add", "--", "staged new.txt")
	for _, name := range []string{"space name.txt", "雪.txt", "-leading.txt"} {
		r.write(name, name+"\n")
	}

	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	status, err := repo.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	byPath := make(map[string]FileStatus, len(status.Files))
	for _, file := range status.Files {
		byPath[file.Path] = file
	}
	want := map[string][2]Change{
		"both.txt":       {ChangeModified, ChangeModified},
		"unstaged.txt":   {ChangeNone, ChangeModified},
		"staged new.txt": {ChangeAdded, ChangeNone},
		"space name.txt": {ChangeNone, ChangeUntracked},
		"雪.txt":          {ChangeNone, ChangeUntracked},
		"-leading.txt":   {ChangeNone, ChangeUntracked},
	}
	if len(byPath) != len(want) {
		t.Fatalf("Status returned %d files, want %d: %#v", len(byPath), len(want), status.Files)
	}
	for path, changes := range want {
		got, ok := byPath[path]
		if !ok {
			t.Errorf("Status omitted %q", path)
			continue
		}
		if got.Staged != changes[0] || got.Unstaged != changes[1] {
			t.Errorf("%q changes = (%v, %v), want (%v, %v)", path, got.Staged, got.Unstaged, changes[0], changes[1])
		}
	}
}
