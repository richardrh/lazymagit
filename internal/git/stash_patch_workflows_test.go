package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStashWorkflowOptionsAndTypedInspection(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("tracked.txt", "base\n")
	r.write("other.txt", "base\n")
	r.commitAll("base")
	r.write("tracked.txt", "stashed\n")
	r.write("other.txt", "left alone\n")
	r.write("-untracked.txt", "untracked\n")
	repo, _ := Discover(r.dir)

	var records []ProcessRecord
	recorded := WithProcessRecorder(ctx, func(record ProcessRecord) { records = append(records, record) })
	if err := repo.StashPush(recorded, StashPushOptions{
		Message: "selected stash", IncludeUntracked: true, Paths: []string{"tracked.txt", "-untracked.txt"},
	}); err != nil {
		t.Fatalf("StashPush: %v", err)
	}
	if len(records) != 1 || strings.Join(records[0].Args, "\x00") != "stash\x00push\x00--include-untracked\x00--message\x00selected stash\x00--\x00tracked.txt\x00-untracked.txt" {
		t.Fatalf("stash process record = %#v", records)
	}
	if got := r.read("other.txt"); got != "left alone\n" {
		t.Fatalf("unselected worktree content = %q", got)
	}
	if _, err := os.Stat(filepath.Join(r.dir, "-untracked.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("selected untracked path was not stashed: %v", err)
	}

	stashes, err := repo.Stashes(ctx)
	if err != nil || len(stashes) != 1 {
		t.Fatalf("Stashes = %#v, %v", stashes, err)
	}
	if stashes[0].Ref != "stash@{0}" || stashes[0].ID == "" || stashes[0].Date.IsZero() || !strings.Contains(stashes[0].Subject, "selected stash") {
		t.Fatalf("typed stash = %#v", stashes[0])
	}
	details, err := repo.ShowStash(ctx, stashes[0].Ref)
	if err != nil {
		t.Fatalf("ShowStash: %v", err)
	}
	if details.Stash.ID != stashes[0].ID || details.PatchTruncated || !strings.Contains(details.Patch, "stashed") {
		t.Fatalf("stash details = %#v", details)
	}
	patchDir := filepath.Join(r.dir, "stash patches")
	if err := os.Mkdir(patchDir, 0o755); err != nil {
		t.Fatal(err)
	}
	formatted, err := repo.FormatPatchFromStash(recorded, stashes[0].Ref, FormatPatchOptions{OutputDirectory: patchDir})
	if err != nil || len(formatted) != 1 {
		t.Fatalf("FormatPatchFromStash = %#v, %v", formatted, err)
	}
	formattedData, err := os.ReadFile(formatted[0])
	if err != nil || !strings.Contains(string(formattedData), "+stashed") {
		t.Fatalf("stash format patch omitted tracked change: %v, %q", err, formattedData)
	}

	if err := repo.StashApply(recorded, stashes[0].Ref, StashApplyOptions{}); err != nil {
		t.Fatalf("StashApply: %v", err)
	}
	assertRecordedArgs(t, records, []string{"stash", "apply", stashes[0].ID})
	if got := r.read("tracked.txt"); got != "stashed\n" {
		t.Fatalf("applied contents = %q", got)
	}
	if err := repo.StashDrop(ctx, stashes[0].Ref, ConfirmationOptions{}); !errors.Is(err, ErrConfirmationRequired) {
		t.Fatalf("unconfirmed drop error = %v", err)
	}
	if got, _ := repo.Stashes(ctx); len(got) != 1 {
		t.Fatal("unconfirmed drop changed stash list")
	}
	if err := repo.StashDrop(recorded, stashes[0].Ref, ConfirmationOptions{Token: NewConfirmationToken(stashes[0].ID)}); err != nil {
		t.Fatalf("confirmed drop: %v", err)
	}
}

func TestStashKeepIndexAndBranch(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("file.txt", "base\n")
	r.commitAll("base")
	r.write("file.txt", "staged\n")
	r.git("add", "--", "file.txt")
	r.write("file.txt", "unstaged\n")
	repo, _ := Discover(r.dir)
	if err := repo.StashPush(ctx, StashPushOptions{Message: "keep", KeepIndex: true}); err != nil {
		t.Fatalf("keep-index stash: %v", err)
	}
	if got := r.read("file.txt"); got != "staged\n" {
		t.Fatalf("keep-index worktree = %q", got)
	}
	// Clean the retained index/worktree before asking stash branch to apply both
	// layers at the stash's original base.
	r.git("reset", "--hard", "HEAD")
	if err := repo.StashBranch(ctx, "from-stash", "stash@{0}"); err != nil {
		t.Fatalf("StashBranch: %v", err)
	}
	if got := r.git("branch", "--show-current"); got != "from-stash" {
		t.Fatalf("branch = %q", got)
	}
	if got, _ := repo.Stashes(ctx); len(got) != 0 {
		t.Fatalf("stash branch did not drop stash: %#v", got)
	}
}

func TestStashSnapshotStoresBothLayersWithoutChangingThem(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("staged.txt", "base staged\n")
	r.write("worktree.txt", "base worktree\n")
	r.commitAll("base")
	r.write("staged.txt", "index snapshot\n")
	r.git("add", "--", "staged.txt")
	r.write("worktree.txt", "worktree snapshot\n")
	repo, _ := Discover(r.dir)

	beforeIndex := r.git("diff", "--cached", "--binary")
	beforeWorktree := r.git("diff", "--binary")
	var records []ProcessRecord
	recorded := WithProcessRecorder(ctx, func(record ProcessRecord) { records = append(records, record) })
	if err := repo.StashSnapshot(recorded, "checkpoint"); err != nil {
		t.Fatalf("StashSnapshot: %v", err)
	}
	if got := r.git("diff", "--cached", "--binary"); got != beforeIndex {
		t.Fatalf("snapshot changed index:\n%s", got)
	}
	if got := r.git("diff", "--binary"); got != beforeWorktree {
		t.Fatalf("snapshot changed worktree:\n%s", got)
	}
	stashes, err := repo.Stashes(ctx)
	if err != nil || len(stashes) != 1 || !strings.Contains(stashes[0].Subject, "checkpoint") {
		t.Fatalf("snapshot stash = %#v, %v", stashes, err)
	}
	details, err := repo.ShowStash(ctx, stashes[0].ID)
	if err != nil || !strings.Contains(details.Patch, "+index snapshot") || !strings.Contains(details.Patch, "+worktree snapshot") {
		t.Fatalf("snapshot details = %#v, %v", details, err)
	}
	if len(records) != 1 || len(records[0].Args) != 5 || strings.Join(records[0].Args[:4], "\x00") != "stash\x00store\x00--message\x00checkpoint" || records[0].Args[4] != stashes[0].ID {
		t.Fatalf("snapshot process records = %#v", records)
	}
}

func TestStashSnapshotRejectsCleanRepositoryWithoutCreatingEntry(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("file", "base\n")
	r.commitAll("base")
	repo, _ := Discover(r.dir)
	if err := repo.StashSnapshot(ctx, "empty"); err == nil || !strings.Contains(err.Error(), "no tracked changes") {
		t.Fatalf("clean snapshot error = %v", err)
	}
	if stashes, err := repo.Stashes(ctx); err != nil || len(stashes) != 0 {
		t.Fatalf("clean snapshot created entry: %#v, %v", stashes, err)
	}
}

func TestStashPopRestoresIndexAndClearRequiresConfirmation(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("file.txt", "base\n")
	r.commitAll("base")
	r.write("file.txt", "staged\n")
	r.git("add", "--", "file.txt")
	repo, _ := Discover(r.dir)
	if err := repo.StashPush(ctx, StashPushOptions{Message: "indexed"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.StashPop(ctx, "stash@{0}", StashApplyOptions{Index: true}); err != nil {
		t.Fatalf("StashPop --index: %v", err)
	}
	if got := r.git("diff", "--cached", "--name-only"); got != "file.txt" {
		t.Fatalf("restored index paths = %q", got)
	}
	if got, _ := repo.Stashes(ctx); len(got) != 0 {
		t.Fatalf("pop retained stash: %#v", got)
	}
	r.git("reset", "--hard", "HEAD")
	r.write("ignored.tmp", "ignored\n")
	r.write(".gitignore", "*.tmp\n")
	r.git("add", "--", ".gitignore")
	r.git("commit", "-m", "ignore")
	if err := repo.StashPush(ctx, StashPushOptions{Message: "all", All: true}); err != nil {
		t.Fatalf("stash --all: %v", err)
	}
	if _, err := os.Stat(filepath.Join(r.dir, "ignored.tmp")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stash --all retained ignored path: %v", err)
	}
	if err := repo.StashClear(ctx, ConfirmationOptions{}); !errors.Is(err, ErrConfirmationRequired) {
		t.Fatalf("unconfirmed clear = %v", err)
	}
	if err := repo.StashClear(ctx, ConfirmationOptions{Token: NewConfirmationToken("all-stashes")}); err != nil {
		t.Fatalf("confirmed clear: %v", err)
	}
	if got, _ := repo.Stashes(ctx); len(got) != 0 {
		t.Fatalf("clear retained stashes: %#v", got)
	}
}

func TestSaveAndApplyDiffPatchUsesCompleteLiteralFile(t *testing.T) {
	ctx := context.Background()
	source := newTestRepo(t)
	source.write("a*.txt", "base\n")
	source.write("other.txt", "base\n")
	source.commitAll("base")
	source.write("a*.txt", "changed\n")
	source.write("other.txt", "not in patch\n")
	repo, _ := Discover(source.dir)
	var records []ProcessRecord
	recorded := WithProcessRecorder(ctx, func(record ProcessRecord) { records = append(records, record) })
	if err := repo.SaveDiffPatch(recorded, "-saved.patch", DiffPatchOptions{Paths: []string{"a*.txt"}}); err != nil {
		t.Fatalf("SaveDiffPatch: %v", err)
	}
	patch := source.read("-saved.patch")
	if !strings.Contains(patch, "+changed") || strings.Contains(patch, "other.txt") {
		t.Fatalf("saved patch contents = %q", patch)
	}
	if len(records) != 1 || records[0].Args[0] != "diff" {
		t.Fatalf("save process records = %#v", records)
	}

	source.git("reset", "--hard", "HEAD")
	if err := repo.ApplyPatch(recorded, "-saved.patch", ApplyPatchOptions{Index: true}); err != nil {
		t.Fatalf("ApplyPatch: %v", err)
	}
	if got := source.read("a*.txt"); got != "changed\n" {
		t.Fatalf("applied contents = %q", got)
	}
	if got := source.git("diff", "--cached", "--name-only"); got != "a*.txt" {
		t.Fatalf("cached paths = %q", got)
	}
}

func TestFormatPatchAndAMStateControls(t *testing.T) {
	ctx := context.Background()
	source := newTestRepo(t)
	source.write("base.txt", "base\n")
	base := source.commitAll("base")
	source.write("topic.txt", "topic\n")
	source.commitAll("topic change")
	repo, _ := Discover(source.dir)
	outDir := filepath.Join(source.dir, "patch output")
	if err := os.Mkdir(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	patches, err := repo.FormatPatch(ctx, base+"..HEAD", FormatPatchOptions{OutputDirectory: outDir, Numbered: true, Signoff: true})
	if err != nil || len(patches) != 1 {
		t.Fatalf("FormatPatch = %#v, %v", patches, err)
	}
	if !strings.Contains(source.read(filepath.ToSlash(strings.TrimPrefix(patches[0], source.dir+string(filepath.Separator)))), "Signed-off-by: Backend Test") {
		t.Fatal("formatted patch omitted signoff")
	}

	target := newTestRepo(t)
	target.write("base.txt", "base\n")
	target.commitAll("base")
	targetRepo, _ := Discover(target.dir)
	if state, err := targetRepo.AMState(); err != nil || state.InProgress {
		t.Fatalf("initial AMState = %#v, %v", state, err)
	}
	if err := targetRepo.AMStart(ctx, patches, AMOptions{ThreeWay: true}); err != nil {
		t.Fatalf("AMStart file: %v", err)
	}
	if got := target.git("log", "-1", "--format=%s"); got != "topic change" {
		t.Fatalf("am subject = %q", got)
	}
	if err := targetRepo.AMContinue(ctx); !errors.Is(err, ErrNoAMInProgress) {
		t.Fatalf("AMContinue without state = %v", err)
	}
}

func TestAMConflictStateAndAbort(t *testing.T) {
	ctx := context.Background()
	source := newTestRepo(t)
	source.write("file.txt", "base\n")
	base := source.commitAll("base")
	source.write("file.txt", "source\n")
	source.commitAll("source")
	sourceRepo, _ := Discover(source.dir)
	out := filepath.Join(source.dir, "patches")
	if err := os.Mkdir(out, 0o755); err != nil {
		t.Fatal(err)
	}
	patches, err := sourceRepo.FormatPatch(ctx, base+"..HEAD", FormatPatchOptions{OutputDirectory: out})
	if err != nil {
		t.Fatal(err)
	}

	target := newTestRepo(t)
	target.write("file.txt", "base\n")
	target.commitAll("base")
	target.write("file.txt", "target\n")
	target.commitAll("target")
	targetRepo, _ := Discover(target.dir)
	if err := targetRepo.AMStart(ctx, patches, AMOptions{}); err == nil {
		t.Fatal("conflicting AMStart succeeded")
	}
	state, err := targetRepo.AMState()
	if err != nil || !state.InProgress || state.Current != 1 || state.Last != 1 {
		t.Fatalf("conflict AMState = %#v, %v", state, err)
	}
	if err := targetRepo.AMAbort(ctx); err != nil {
		t.Fatalf("AMAbort: %v", err)
	}
	if state, _ := targetRepo.AMState(); state.InProgress {
		t.Fatal("AM state remained after abort")
	}
}
