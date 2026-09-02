package git

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseWorktreesPreservesPorcelainState(t *testing.T) {
	out := []byte("worktree /primary\x00HEAD abc\x00branch refs/heads/main\x00bare\x00" +
		"worktree /linked\x00HEAD def\x00detached\x00locked maintenance\x00prunable stale metadata\x00")
	got, err := parseWorktrees(out)
	if err != nil {
		t.Fatal(err)
	}
	want := []Worktree{
		{Path: "/primary", HEAD: "abc", Branch: "main", Bare: true, Primary: true},
		{Path: "/linked", HEAD: "def", Detached: true, Locked: true, LockReason: "maintenance", Prunable: true, PruneReason: "stale metadata"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("worktrees = %#v, want %#v", got, want)
	}
	if _, err := parseWorktrees([]byte("HEAD abc\x00")); err == nil {
		t.Fatal("field before worktree record succeeded")
	}
}

func TestReadSparseCheckoutPatterns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sparse-checkout")
	patterns, err := readSparseCheckoutPatterns(path)
	if err != nil || patterns != nil {
		t.Fatalf("missing patterns = %#v, %v", patterns, err)
	}
	if err := os.WriteFile(path, []byte("dir\nother/file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	patterns, err = readSparseCheckoutPatterns(path)
	if err != nil || !reflect.DeepEqual(patterns, []string{"dir", "other/file"}) {
		t.Fatalf("patterns = %#v, %v", patterns, err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	patterns, err = readSparseCheckoutPatterns(path)
	if err != nil || patterns != nil {
		t.Fatalf("empty patterns = %#v, %v", patterns, err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("x\n", 100001)), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = readSparseCheckoutPatterns(path)
	var tooLarge *TooLargeError
	if !errors.As(err, &tooLarge) {
		t.Fatalf("oversized pattern error = %T %v", err, err)
	}
}

func TestWorktreePruneOptionsProduceExplicitCommand(t *testing.T) {
	opts := WorktreePruneOptions{DryRun: true, Verbose: true, Expire: "2.weeks.ago"}
	if got, want := worktreePruneArgs(worktreePruneExpiration(opts), opts), []string{"worktree", "prune", "--dry-run", "--verbose", "--expire", "2.weeks.ago"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
	forced := WorktreePruneOptions{Force: Confirmed}
	if got := worktreePruneExpiration(forced); got != "now" {
		t.Fatalf("forced expiration = %q, want now", got)
	}
}
