package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRunRebaseTodoEditorOnlyReplacesExpectedTodoFile(t *testing.T) {
	if handled, err := RunRebaseTodoEditor(nil); handled || err != nil {
		t.Fatalf("unrelated invocation = handled %t, err %v", handled, err)
	}
	if handled, err := RunRebaseTodoEditor([]string{"--lazymagit-rebase-todo-editor"}); !handled || err == nil {
		t.Fatalf("short helper invocation = handled %t, err %v", handled, err)
	}

	admin := t.TempDir()
	destination := filepath.Join(admin, "rebase-merge", "git-rebase-todo")
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("pick old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	todo := "pick " + strings.Repeat("a", 40) + " replacement\n"
	source := filepath.Join(t.TempDir(), "todo")
	if err := os.WriteFile(source, []byte(todo), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"--lazymagit-rebase-todo-editor", source, admin, destination}
	if handled, err := RunRebaseTodoEditor(args); !handled || err != nil {
		t.Fatalf("valid helper invocation = handled %t, err %v", handled, err)
	}
	if got, err := os.ReadFile(destination); err != nil || string(got) != todo {
		t.Fatalf("replaced todo = %q, %v", got, err)
	}

	if handled, err := RunRebaseTodoEditor([]string{"--lazymagit-rebase-todo-editor", source, admin, filepath.Join(admin, "unexpected")}); !handled || err == nil {
		t.Fatalf("unexpected destination = handled %t, err %v", handled, err)
	}
	if handled, err := RunRebaseTodoEditor([]string{"--lazymagit-rebase-todo-editor", admin, admin, destination}); !handled || err == nil {
		t.Fatalf("unsafe source = handled %t, err %v", handled, err)
	}
	if err := os.WriteFile(source, []byte("exec echo unsafe\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if handled, err := RunRebaseTodoEditor(args); !handled || err == nil {
		t.Fatalf("invalid todo source = handled %t, err %v", handled, err)
	}
	link := filepath.Join(t.TempDir(), "todo-link")
	if err := os.Symlink(source, link); err != nil {
		t.Fatal(err)
	}
	if handled, err := RunRebaseTodoEditor([]string{"--lazymagit-rebase-todo-editor", link, admin, destination}); !handled || err == nil {
		t.Fatalf("symlink source = handled %t, err %v", handled, err)
	}
}

func TestCanonicalRebaseTodoDirectlyNormalizesAndRejectsInvalidPlans(t *testing.T) {
	r := newTestRepo(t)
	r.write("file", "one\n")
	first := r.commitAll("first")
	r.write("file", "two\n")
	second := r.commitAll("second")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	expected := []string{first, second}
	todo := "# comment\n  pick " + first[:8] + "   first subject  \n\tsquash " + second[:8] + " second subject\n"
	canonical, err := repo.canonicalRebaseTodo(ctx, todo, expected)
	if err != nil {
		t.Fatal(err)
	}
	want := "pick " + first + " first subject\nsquash " + second + " second subject\n"
	if canonical != want {
		t.Fatalf("canonical todo = %q, want %q", canonical, want)
	}

	invalid := []string{
		"fixup " + first + " first\npick " + second + " second\n",
		"drop " + first + " first\nfixup " + second + " second\n",
		"pick " + first + " first\npick " + first + " duplicate\n",
		"pick " + first + " missing second\n",
		"pick " + strings.Repeat("f", 40) + " unselected\npick " + second + " second\n",
	}
	for _, input := range invalid {
		if _, err := repo.canonicalRebaseTodo(ctx, input, expected); err == nil {
			t.Errorf("canonicalRebaseTodo(%q) succeeded", input)
		}
	}
}

func TestCherryPickOrderedNoCommitAndProcessRecord(t *testing.T) {
	r := newTestRepo(t)
	r.write("base", "base\n")
	base := r.commitAll("base")
	r.git("switch", "-c", "source")
	r.write("one", "one\n")
	one := r.commitAll("one")
	r.write("two", "two\n")
	two := r.commitAll("two")
	r.git("switch", "main")
	repo, _ := Discover(r.dir)
	var records []ProcessRecord
	ctx := WithProcessRecorder(context.Background(), func(record ProcessRecord) { records = append(records, record) })

	if err := repo.CherryPickStart(ctx, []string{one, two}, PickOptions{NoCommit: true, Signoff: true, NoEdit: true, RecordOrigin: true}); err != nil {
		t.Fatalf("CherryPickStart: %v", err)
	}
	if got := r.git("rev-parse", "HEAD"); got != base {
		t.Fatalf("--no-commit moved HEAD to %s", got)
	}
	if got := r.git("diff", "--cached", "--name-only"); got != "one\ntwo" {
		t.Fatalf("staged paths = %q", got)
	}
	want := []string{"cherry-pick", "--no-commit", "--signoff", "--no-edit", "-x", "--", one, two}
	if len(records) != 1 || !reflect.DeepEqual(records[0].Args, want) {
		t.Fatalf("process records = %#v, want args %#v", records, want)
	}
}

func TestCherryPickFastForwardOptionUsesTypedArgv(t *testing.T) {
	r := newTestRepo(t)
	r.write("base", "base\n")
	base := r.commitAll("base")
	r.git("switch", "-c", "source")
	r.write("topic", "topic\n")
	tip := r.commitAll("topic")
	r.git("switch", "main")
	repo, _ := Discover(r.dir)
	var records []ProcessRecord
	ctx := WithProcessRecorder(context.Background(), func(record ProcessRecord) { records = append(records, record) })

	if err := repo.CherryPickStart(ctx, []string{tip}, PickOptions{FastForward: true, NoEdit: true}); err != nil {
		t.Fatalf("fast-forward cherry-pick: %v", err)
	}
	if got := r.git("rev-parse", "HEAD"); got != tip || got == base {
		t.Fatalf("fast-forward cherry-pick HEAD = %s, want %s", got, tip)
	}
	want := []string{"cherry-pick", "--no-edit", "--ff", "--", tip}
	if len(records) != 1 || !reflect.DeepEqual(records[0].Args, want) {
		t.Fatalf("process records = %#v, want args %#v", records, want)
	}
}

func TestCherryPickConflictUsesAdminStateAndCanAbort(t *testing.T) {
	r := newTestRepo(t)
	r.write("file", "base\n")
	r.commitAll("base")
	r.git("switch", "-c", "source")
	r.write("file", "source\n")
	commit := r.commitAll("source")
	r.git("switch", "main")
	r.write("file", "main\n")
	r.commitAll("main")
	repo, _ := Discover(r.dir)

	if err := repo.CherryPickStart(context.Background(), []string{commit}, PickOptions{}); err == nil {
		t.Fatal("conflicting cherry-pick unexpectedly succeeded")
	}
	if err := repo.CherryPickAbort(context.Background()); err != nil {
		t.Fatalf("CherryPickAbort: %v", err)
	}
	if got := r.read("file"); got != "main\n" {
		t.Fatalf("abort restored %q", got)
	}
	if err := repo.CherryPickContinue(context.Background()); !errors.Is(err, ErrWorkflowNotActive) {
		t.Fatalf("continue outside workflow = %v", err)
	}
}

func TestAppendRebaseStartOptionsDirect(t *testing.T) {
	opts := RebaseOptions{
		KeepEmpty: true, RebaseMerges: true, UpdateRefs: true,
		Autostash: true, ForceRebase: true, Strategy: "ort", Signoff: true,
	}
	want := []string{"prefix", "--keep-empty", "--rebase-merges", "--update-refs", "--autostash", "--force-rebase", "--strategy=ort", "--signoff"}
	if got := appendRebaseStartOptions([]string{"prefix"}, opts); !reflect.DeepEqual(got, want) {
		t.Fatalf("appendRebaseStartOptions = %#v, want %#v", got, want)
	}
	if got := appendRebaseStartOptions(nil, RebaseOptions{}); len(got) != 0 {
		t.Fatalf("empty appendRebaseStartOptions = %#v", got)
	}
}

func TestRebaseStartRejectsInvalidOntoAndBranch(t *testing.T) {
	r := newTestRepo(t)
	r.write("base", "base\n")
	r.commitAll("base")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := repo.RebaseStart(ctx, RebaseOptions{Upstream: "HEAD", Onto: "missing"}); err == nil || !strings.Contains(err.Error(), "rebase onto") {
		t.Fatalf("invalid onto error = %v", err)
	}
	if err := repo.RebaseStart(ctx, RebaseOptions{Upstream: "HEAD", Branch: "missing"}); err == nil || !strings.Contains(err.Error(), "rebase branch") {
		t.Fatalf("invalid branch error = %v", err)
	}
}

func TestRebaseNonInteractiveOntoAndLifecycleGuard(t *testing.T) {
	r := newTestRepo(t)
	r.write("base", "base\n")
	base := r.commitAll("base")
	r.git("switch", "-c", "topic")
	r.write("topic", "topic\n")
	r.commitAll("topic")
	r.git("switch", "main")
	r.write("main", "main\n")
	main := r.commitAll("main")
	r.git("switch", "topic")
	repo, _ := Discover(r.dir)

	if err := repo.RebaseStart(context.Background(), RebaseOptions{Upstream: base, Onto: main}); err != nil {
		t.Fatalf("RebaseStart: %v", err)
	}
	if parent := r.git("rev-parse", "HEAD^"); parent != main {
		t.Fatalf("rebased parent = %s, want %s", parent, main)
	}
	if err := repo.RebaseAbort(context.Background()); !errors.Is(err, ErrWorkflowNotActive) {
		t.Fatalf("abort outside rebase = %v", err)
	}
	todo, err := repo.DefaultRebaseTodo(context.Background(), base)
	if err != nil {
		t.Fatalf("DefaultRebaseTodo: %v", err)
	}
	if err := repo.RebaseInteractive(context.Background(), RebaseOptions{Upstream: base, Todo: todo}); err != nil {
		t.Fatalf("RebaseInteractive: %v", err)
	}
}

func TestRebaseTypedOptionsBuildNonInteractiveArgv(t *testing.T) {
	r := newTestRepo(t)
	r.write("base", "base\n")
	base := r.commitAll("base")
	r.git("switch", "-c", "topic")
	r.write("topic", "topic\n")
	r.commitAll("topic")
	r.git("switch", "main")
	r.write("main", "main\n")
	onto := r.commitAll("main")
	r.git("switch", "topic")
	repo, _ := Discover(r.dir)
	var records []ProcessRecord
	ctx := WithProcessRecorder(context.Background(), func(record ProcessRecord) { records = append(records, record) })

	err := repo.RebaseStart(ctx, RebaseOptions{Upstream: base, Onto: onto, KeepEmpty: true, RebaseMerges: true, UpdateRefs: true, Autostash: true, ForceRebase: true, Strategy: "ort", Signoff: true})
	if err != nil {
		t.Fatalf("RebaseStart: %v", err)
	}
	want := []string{"rebase", "--keep-empty", "--rebase-merges", "--update-refs", "--autostash", "--force-rebase", "--strategy=ort", "--signoff", "--onto", onto, "--", base}
	if len(records) != 1 || !reflect.DeepEqual(records[0].Args, want) {
		t.Fatalf("process records = %#v, want args %#v", records, want)
	}
	if parent := r.git("rev-parse", "HEAD^"); parent != onto {
		t.Fatalf("rebased parent = %s, want %s", parent, onto)
	}
}

func TestRebaseTodoValidationAndAtomicAdminWrite(t *testing.T) {
	r := newTestRepo(t)
	r.write("file", "base\n")
	oid := r.commitAll("base")
	repo, _ := Discover(r.dir)
	admin := filepath.Join(repo.GitDir(), "rebase-merge")
	if err := os.Mkdir(admin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(admin, "git-rebase-todo"), []byte("pick "+oid+" base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	want := "drop " + oid + " base\n"
	if err := repo.WriteRebaseTodo(context.Background(), want); err != nil {
		t.Fatalf("WriteRebaseTodo: %v", err)
	}
	got, err := repo.ReadRebaseTodo(context.Background())
	if err != nil || got != want {
		t.Fatalf("ReadRebaseTodo = %q, %v", got, err)
	}
	if err := ValidateRebaseTodo("exec rm -rf .\n"); err == nil {
		t.Fatal("shell-bearing exec todo was accepted")
	}
	if err := ValidateRebaseTodo("p " + oid + " alias\n"); err == nil {
		t.Fatal("abbreviated todo command was accepted")
	}
	if err := ValidateRebaseTodo("pick " + oid + "\nreword " + oid + "\nedit " + oid + "\nsquash " + oid + "\nfixup " + oid + "\ndrop " + oid + "\n"); err != nil {
		t.Fatalf("closed supported todo commands rejected: %v", err)
	}
}

func TestResetRequiresConfirmationAndReportsDestruction(t *testing.T) {
	r := newTestRepo(t)
	r.write("file", "base\n")
	base := r.commitAll("base")
	r.write("file", "changed\n")
	r.commitAll("changed")
	repo, _ := Discover(r.dir)
	var records []ProcessRecord
	ctx := WithProcessRecorder(context.Background(), func(record ProcessRecord) { records = append(records, record) })
	opts := ResetOptions{Mode: ResetHard, Target: base}

	err := repo.Reset(ctx, opts)
	var required *HistoryConfirmationRequiredError
	if !errors.As(err, &required) || !required.Preflight.LosesHEAD || !required.Preflight.LosesIndex || !required.Preflight.LosesWorktree {
		t.Fatalf("reset confirmation = %#v, %v", required, err)
	}
	if len(records) != 0 {
		t.Fatalf("unconfirmed reset ran commands: %#v", records)
	}
	opts.Confirmed = true
	if err := repo.Reset(ctx, opts); err != nil {
		t.Fatalf("confirmed Reset: %v", err)
	}
	if got := r.git("rev-parse", "HEAD"); got != base {
		t.Fatalf("HEAD = %s, want %s", got, base)
	}
}

func TestValidateResetPreflightOptionsAndLossesDirect(t *testing.T) {
	valid := []ResetOptions{{Mode: ResetMixed}, {Mode: ResetIndex, Paths: []string{"one"}}, {Mode: ResetWorktree, Paths: []string{"one"}}, {Mode: ResetFile, Paths: []string{"one"}}}
	for _, opts := range valid {
		if err := validateResetPreflightOptions(opts); err != nil {
			t.Errorf("valid options %#v: %v", opts, err)
		}
	}
	invalid := []ResetOptions{{Mode: ResetMode(255)}, {Mode: ResetFile}, {Mode: ResetFile, Paths: []string{"one", "two"}}, {Mode: ResetHard, Paths: []string{"one"}}}
	for _, opts := range invalid {
		if err := validateResetPreflightOptions(opts); err == nil {
			t.Errorf("invalid options %#v succeeded", opts)
		}
	}
	tests := []struct {
		mode                  ResetMode
		head, index, worktree bool
	}{
		{ResetSoft, true, false, false}, {ResetMixed, true, true, false},
		{ResetHard, true, true, true}, {ResetKeep, true, true, true},
		{ResetIndex, false, true, false}, {ResetWorktree, false, false, true},
		{ResetFile, false, false, true},
	}
	for _, test := range tests {
		var got DestructivePreflight
		setResetPreflightLosses(&got, test.mode)
		if got.LosesHEAD != test.head || got.LosesIndex != test.index || got.LosesWorktree != test.worktree {
			t.Errorf("losses(%d) = (%t, %t, %t), want (%t, %t, %t)", test.mode, got.LosesHEAD, got.LosesIndex, got.LosesWorktree, test.head, test.index, test.worktree)
		}
	}
}

func TestResetIndexAndWorktreeDoNotMoveHEAD(t *testing.T) {
	r := newTestRepo(t)
	r.write("file", "base\n")
	r.commitAll("base")
	r.write("file", "head\n")
	head := r.commitAll("head")
	r.write("file", "staged\n")
	r.git("add", "file")
	repo, _ := Discover(r.dir)

	if err := repo.Reset(context.Background(), ResetOptions{Mode: ResetIndex, Target: "HEAD", ConfirmOptions: ConfirmOptions{Confirmed: true}}); err != nil {
		t.Fatalf("ResetIndex: %v", err)
	}
	if got := r.git("rev-parse", "HEAD"); got != head {
		t.Fatalf("index reset moved HEAD")
	}
	if err := repo.Reset(context.Background(), ResetOptions{Mode: ResetWorktree, Target: "HEAD", ConfirmOptions: ConfirmOptions{Confirmed: true}}); err != nil {
		t.Fatalf("ResetWorktree: %v", err)
	}
	if got := r.read("file"); got != "head\n" {
		t.Fatalf("worktree reset contents = %q", got)
	}
}

func TestBisectStartTypedOptionsAndLifecycle(t *testing.T) {
	r := newTestRepo(t)
	r.write("n", "0\n")
	good := r.commitAll("good")
	r.write("n", "1\n")
	r.commitAll("middle")
	r.write("n", "2\n")
	bad := r.commitAll("bad")
	repo, _ := Discover(r.dir)
	var records []ProcessRecord
	ctx := WithProcessRecorder(context.Background(), func(record ProcessRecord) { records = append(records, record) })

	if err := repo.BisectStart(ctx, BisectStartOptions{Bad: bad, Good: good, NoCheckout: true, FirstParent: true}); err != nil {
		t.Fatalf("BisectStart: %v", err)
	}
	want := []string{"bisect", "start", "--no-checkout", "--first-parent", bad, good}
	if len(records) != 1 || !reflect.DeepEqual(records[0].Args, want) {
		t.Fatalf("process records = %#v, want args %#v", records, want)
	}
	if err := repo.BisectReset(context.Background()); err != nil {
		t.Fatalf("BisectReset: %v", err)
	}
}

func TestBisectLifecycleAndExplicitArgvValidation(t *testing.T) {
	r := newTestRepo(t)
	r.write("n", "0\n")
	good := r.commitAll("good")
	r.write("n", "1\n")
	r.commitAll("middle")
	r.write("n", "2\n")
	bad := r.commitAll("bad")
	repo, _ := Discover(r.dir)

	capability := NewAllowUnsafeExecution()
	if err := repo.UnsafeBisectRun(context.Background(), capability, nil); !errors.Is(err, ErrWorkflowNotActive) {
		t.Fatalf("run outside bisect = %v", err)
	}
	if err := repo.BisectStart(context.Background(), BisectStartOptions{Bad: bad, Good: good}); err != nil {
		t.Fatalf("BisectStart: %v", err)
	}
	if err := repo.UnsafeBisectRun(context.Background(), capability, nil); err == nil {
		t.Fatal("empty bisect argv accepted")
	}
	if err := repo.BisectReset(context.Background()); err != nil {
		t.Fatalf("BisectReset: %v", err)
	}
}

func TestNotesRemoveConfirmationAndEditorRequirement(t *testing.T) {
	r := newTestRepo(t)
	r.write("file", "base\n")
	oid := r.commitAll("base")
	r.git("notes", "add", "-m", "note", oid)
	repo, _ := Discover(r.dir)
	o := NotesRemoveOptions{Objects: []string{oid}}
	if err := repo.NotesRemove(context.Background(), o); !errors.Is(err, ErrConfirmationRequired) {
		t.Fatalf("NotesRemove confirmation = %v", err)
	}
	if got := r.git("notes", "show", oid); got != "note" {
		t.Fatalf("unconfirmed remove changed note to %q", got)
	}
	o.Confirmed = true
	if err := repo.NotesRemove(context.Background(), o); err != nil {
		t.Fatalf("NotesRemove: %v", err)
	}
	var editorRequired *EditorRequiredError
	if err := repo.NotesEdit(context.Background(), "", oid); !errors.As(err, &editorRequired) {
		t.Fatalf("NotesEdit = %v", err)
	}
}
