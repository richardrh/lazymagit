package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConflictEntriesReadIndexStagesAndReportRepositoryErrors(t *testing.T) {
	ctx := context.Background()
	_, repo := conflictedRepository(t)
	entries, err := repo.conflictEntries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stages := entries["conflict.txt"]
	if len(stages) != 3 || stages[ConflictOurs].oid == "" {
		t.Fatalf("conflict entries = %#v", entries)
	}

	if _, err := (&Repository{commandDir: t.TempDir()}).conflictEntries(ctx); err == nil {
		t.Fatal("conflictEntries outside a repository succeeded")
	}
}

func TestParseConflictEntries(t *testing.T) {
	oid := strings.Repeat("a", 40)
	out := []byte("100644 " + oid + " 1\tpath\twith-tab\x00100644 " + oid + " 2\tpath\twith-tab\x00")
	entries, err := parseConflictEntries(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || len(entries["path\twith-tab"]) != 2 {
		t.Fatalf("parsed entries = %#v", entries)
	}
	if empty, err := parseConflictEntries(nil); err != nil || len(empty) != 0 {
		t.Fatalf("empty entries = %#v, %v", empty, err)
	}
	if _, err := parseConflictEntries([]byte("100644 " + oid + " 1\tpath\x00100644 " + oid + " 1\tpath\x00")); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate error = %v", err)
	}
}

func TestParseConflictEntryRejectsMalformedRecords(t *testing.T) {
	oid := strings.Repeat("a", 40)
	path, stage, entry, err := parseConflictEntry([]byte("100755 " + oid + " 3\tdir/file"))
	if err != nil || path != "dir/file" || stage != ConflictTheirs || entry.mode != "100755" || entry.oid != oid {
		t.Fatalf("parseConflictEntry = %q, %d, %#v, %v", path, stage, entry, err)
	}

	for _, record := range []string{
		"missing-tab",
		"100644 " + oid + " 1\t",
		"100644 " + oid + "\tpath",
		"100644 not-hex 1\tpath",
		"100644 " + oid + " nope\tpath",
		"100644 " + oid + " 0\tpath",
		"100644 " + oid + " 4\tpath",
	} {
		if _, _, _, err := parseConflictEntry([]byte(record)); err == nil {
			t.Errorf("malformed record %q was accepted", record)
		}
	}
}

func TestConflictWorktreeIdentityClassifiesPathsAndReportsErrors(t *testing.T) {
	r := newTestRepo(t)
	r.write("tracked", "tracked\n")
	r.commitAll("base")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if got, err := repo.conflictWorktreeIdentity(ctx, "missing"); err != nil || got != "absent" {
		t.Fatalf("missing identity = %q, %v", got, err)
	}
	r.write("regular", "content\n")
	if got, err := repo.conflictWorktreeIdentity(ctx, "regular"); err != nil || !strings.HasPrefix(got, "file:644:") {
		t.Fatalf("regular identity = %q, %v", got, err)
	}
	if err := os.Symlink("regular", filepath.Join(r.dir, "link")); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.conflictWorktreeIdentity(ctx, "link"); err != nil || !strings.HasPrefix(got, "symlink:") {
		t.Fatalf("symlink identity = %q, %v", got, err)
	}
	if err := os.Mkdir(filepath.Join(r.dir, "directory"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.conflictWorktreeIdentity(ctx, "directory"); err != nil || !strings.HasPrefix(got, "other:") {
		t.Fatalf("directory identity = %q, %v", got, err)
	}
	if _, err := repo.conflictWorktreeIdentity(ctx, filepath.Join(r.dir, "regular")); err == nil {
		t.Fatal("absolute conflict path was accepted")
	}

	r.write("not-a-directory", "content\n")
	if _, err := repo.conflictWorktreeIdentity(ctx, "not-a-directory/child"); err == nil || !strings.Contains(err.Error(), "inspect conflict worktree path") {
		t.Fatalf("lstat error = %v", err)
	}
	broken := &Repository{workTree: r.dir, commandDir: filepath.Join(r.dir, "missing-command-directory")}
	if _, err := broken.conflictWorktreeIdentity(ctx, "regular"); err == nil || !strings.Contains(err.Error(), "hash conflict worktree path") {
		t.Fatalf("hash error = %v", err)
	}
}
