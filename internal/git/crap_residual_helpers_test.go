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

func TestNotesPruneHelpers(t *testing.T) {
	refs := map[string]string{
		"":                  "refs/notes/commits",
		"review":            "refs/notes/review",
		"refs/notes/review": "refs/notes/review",
	}
	for input, want := range refs {
		if got := canonicalNotesRef(input); got != want {
			t.Errorf("canonicalNotesRef(%q) = %q, want %q", input, got, want)
		}
	}
	if got, want := parseNotesPruneObjects([]byte("deadbeef\n\ncafebabe\n")), []string{"cafebabe", "deadbeef"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("parseNotesPruneObjects() = %#v, want %#v", got, want)
	}
}

func TestParseRebaseNumber(t *testing.T) {
	if got, err := parseRebaseNumber(" 12\n"); err != nil || got != 12 {
		t.Fatalf("parseRebaseNumber(valid) = %d, %v", got, err)
	}
	for _, input := range []string{"", "nope", "-1"} {
		if _, err := parseRebaseNumber(input); err == nil {
			t.Errorf("parseRebaseNumber(%q) succeeded", input)
		}
	}
}

func TestValidateBranchConfigUpdate(t *testing.T) {
	valid := BranchConfigUpdate{
		Description: ConfigUpdate{Action: ConfigKeep}, Upstream: ConfigUpdate{Action: ConfigKeep},
		Rebase: ConfigUpdate{Action: ConfigSet, Value: "merges"}, PushRemote: ConfigUpdate{Action: ConfigUnset},
		PullRebase: ConfigUpdate{Action: ConfigSet, Value: "interactive"}, RemotePushDefault: ConfigUpdate{Action: ConfigKeep},
		AutoSetupMerge: ConfigUpdate{Action: ConfigSet, Value: "simple"}, AutoSetupRebase: ConfigUpdate{Action: ConfigSet, Value: "remote"},
	}
	if err := validateBranchConfigUpdate(valid); err != nil {
		t.Fatalf("valid update: %v", err)
	}
	invalid := valid
	invalid.Description.Action = "invalid"
	if err := validateBranchConfigUpdate(invalid); err == nil || !strings.Contains(err.Error(), "description") {
		t.Fatalf("invalid action error = %v", err)
	}
	invalid = valid
	invalid.AutoSetupMerge.Value = "invalid"
	if err := validateBranchConfigUpdate(invalid); err == nil || !strings.Contains(err.Error(), "autoSetupMerge") {
		t.Fatalf("invalid mode error = %v", err)
	}
}

func TestApplyConfigUpdate(t *testing.T) {
	var calls []string
	set := func(value string) error { calls = append(calls, "set "+value); return nil }
	unset := func() error { calls = append(calls, "unset"); return nil }
	for _, update := range []ConfigUpdate{{Action: ConfigKeep}, {Action: ConfigSet, Value: "value"}, {Action: ConfigUnset}} {
		if err := applyConfigUpdate(configMutation{update: update, set: set, unset: unset}); err != nil {
			t.Fatalf("applyConfigUpdate(%q): %v", update.Action, err)
		}
	}
	if !reflect.DeepEqual(calls, []string{"set value", "unset"}) {
		t.Fatalf("config mutation calls = %#v", calls)
	}
	if err := applyConfigUpdate(configMutation{update: ConfigUpdate{Action: "invalid"}, set: set, unset: unset}); err == nil {
		t.Fatal("invalid config update succeeded")
	}
}

func TestBranchConfigExecutionHelpers(t *testing.T) {
	r := newTestRepo(t)
	r.write("base", "base\n")
	r.commitAll("base")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	keep := ConfigUpdate{Action: ConfigKeep}
	reviewed := BranchConfigUpdate{Branch: "main", Description: keep, Upstream: keep, Rebase: keep, PushRemote: keep, PullRebase: keep, RemotePushDefault: keep, AutoSetupMerge: ConfigUpdate{Action: ConfigSet, Value: "simple"}, AutoSetupRebase: keep}
	if err := repo.applyBranchConfigUpdates(context.Background(), reviewed); err != nil {
		t.Fatal(err)
	}
	if got := r.git("config", "--get", "branch.autoSetupMerge"); got != "simple" {
		t.Fatalf("autoSetupMerge = %q", got)
	}
	path := filepath.Join(t.TempDir(), "config")
	cause := errors.New("mutation failed")
	if err := os.WriteFile(path, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := rollbackBranchConfig(path, []byte("before"), true, cause); !errors.Is(got, cause) {
		t.Fatalf("rollbackBranchConfig = %v", got)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "before" {
		t.Fatalf("restored config = %q, %v", data, err)
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

func TestFetchArgsHelper(t *testing.T) {
	refspec := "+refs/heads/main:refs/remotes/origin/main"
	tests := []struct {
		name    string
		in      FetchArgs
		want    []string
		wantErr string
	}{
		{name: "defaults", want: []string{"fetch"}},
		{
			name: "all options and refspec",
			in: FetchArgs{
				Remote: "origin", Prune: true, Tags: FetchAllTags, Unshallow: true,
				Force: true, Refspec: refspec, RecurseSubmodules: SubmodulesOnDemand,
			},
			want: []string{"fetch", "--prune", "--tags", "--unshallow", "--force", "--recurse-submodules=on-demand", "--", "origin", refspec},
		},
		{
			name: "no tags recursive branch",
			in:   FetchArgs{Remote: "upstream", Tags: FetchNoTags, Branch: "main", RecurseSubmodules: SubmodulesYes},
			want: []string{"fetch", "--no-tags", "--recurse-submodules=yes", "--", "upstream", "main"},
		},
		{
			name: "remote and disabled recursion",
			in:   FetchArgs{Remote: "origin", RecurseSubmodules: SubmodulesNo},
			want: []string{"fetch", "--recurse-submodules=no", "--", "origin"},
		},
		{name: "invalid tags", in: FetchArgs{Tags: FetchTags(255)}, wantErr: "invalid fetch tags mode"},
		{name: "invalid recursion", in: FetchArgs{RecurseSubmodules: SubmoduleRecursion(255)}, wantErr: "invalid submodule recursion mode"},
		{name: "branch without remote", in: FetchArgs{Branch: "main"}, wantErr: "fetch suffix requires a remote"},
		{name: "refspec without remote", in: FetchArgs{Refspec: refspec}, wantErr: "fetch suffix requires a remote"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := fetchArgs(tt.in)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("fetchArgs error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("fetchArgs = %#v, want %#v", got, tt.want)
			}
		})
	}

	r := newTestRepo(t)
	r.write("base", "base\n")
	r.commitAll("base")
	repo, _ := Discover(r.dir)
	if err := repo.validateFetchArgs(context.Background(), FetchArgs{Branch: "main", Refspec: "refs/heads/main:refs/remotes/origin/main"}); err == nil {
		t.Fatal("mutually exclusive fetch suffix accepted")
	}
}

func TestFormatPatchExtractionHelpers(t *testing.T) {
	options := FormatPatchOptions{Numbered: true, CoverLetter: true, Signoff: true, ThreadStyle: "deep", RFC: true, SubjectPrefix: "PATCH v2", RerollCount: 2, StartNumber: 3, From: "A <a@example.com>", InReplyTo: "<id@example.com>", Base: "HEAD~2", To: []string{"B <b@example.com>"}, Cc: []string{"C <c@example.com>"}}
	args := formatPatchArgs("out", "HEAD~2..HEAD", options)
	joined := strings.Join(args, " ")
	for _, value := range []string{"--numbered", "--cover-letter", "--signoff", "--thread=deep", "--rfc", "--subject-prefix=PATCH v2", "--reroll-count=2", "--start-number=3", "--from=A <a@example.com>", "--in-reply-to=<id@example.com>", "--base=HEAD~2", "--to=B <b@example.com>", "--cc=C <c@example.com>", "HEAD~2..HEAD"} {
		if !strings.Contains(joined, value) {
			t.Errorf("formatPatchArgs %q omit %q", joined, value)
		}
	}
	dir := t.TempDir()
	before := map[string]bool{"existing.patch": true}
	for name, contents := range map[string]string{"existing.patch": "old", "0001-new.patch": "new", "notes.txt": "ignore"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	created, err := newFormatPatchFiles(dir, before)
	if err != nil || !reflect.DeepEqual(created, []string{filepath.Join(dir, "0001-new.patch")}) {
		t.Fatalf("newFormatPatchFiles = %#v, %v", created, err)
	}
	if err := os.Remove(filepath.Join(dir, "0001-new.patch")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("notes.txt", filepath.Join(dir, "0002-link.patch")); err != nil {
		t.Fatal(err)
	}
	if _, err := newFormatPatchFiles(dir, before); err == nil {
		t.Fatal("non-regular format patch output accepted")
	}
}

func TestDiscardPlanningHelpers(t *testing.T) {
	repo := &Repository{workTree: t.TempDir()}
	status := Status{Files: []FileStatus{
		{Path: "tracked", Unstaged: ChangeModified},
		{Path: "untracked", Unstaged: ChangeUntracked},
		{Path: "added", Staged: ChangeAdded},
		{Path: "new", OriginalPath: "old", Staged: ChangeRenamed},
		{Path: "ignored", Unstaged: ChangeModified},
	}}
	plan, err := repo.planDiscard(status, []string{"tracked", "untracked", "added", "old"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan.tracked, []string{"tracked"}) || !reflect.DeepEqual(plan.untracked, []string{"untracked"}) ||
		!reflect.DeepEqual(plan.stagedAdded, []string{"added"}) || !reflect.DeepEqual(plan.stagedTracked, []string{"old", "new"}) {
		t.Fatalf("discard plan = %#v", plan)
	}
	if !supportedDiscardState(ChangeModified) || supportedDiscardState(Change(255)) {
		t.Fatal("supportedDiscardState classified states incorrectly")
	}
	if got := conflictSubject("old", "rename source %s was recreated"); got != "rename source old" {
		t.Fatalf("conflictSubject = %q", got)
	}
}

func TestValidateDiscardFileRejectsUnsafeStates(t *testing.T) {
	dir := t.TempDir()
	repo := &Repository{workTree: dir}
	if err := os.WriteFile(filepath.Join(dir, "deleted"), []byte("replacement"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := repo.validateDiscardFile(FileStatus{Path: "deleted", Unstaged: ChangeDeleted}); !errors.Is(err, ErrMixedState) {
		t.Fatalf("replacement error = %v", err)
	}
	if err := repo.validateDiscardFile(FileStatus{Path: "mixed", Staged: ChangeModified, Unstaged: ChangeModified}); !errors.Is(err, ErrMixedState) {
		t.Fatalf("mixed error = %v", err)
	}
	if err := repo.validateDiscardFile(FileStatus{Path: "bad", Staged: Change(255)}); !errors.Is(err, ErrUnsupportedStagedState) {
		t.Fatalf("unsupported error = %v", err)
	}
}

func TestDiffPatchHelpers(t *testing.T) {
	dir := t.TempDir()
	repo := &Repository{commandDir: dir}
	path, err := repo.diffPatchOutputPath("out.patch")
	if err != nil || path != filepath.Join(dir, "out.patch") {
		t.Fatalf("diffPatchOutputPath = %q, %v", path, err)
	}
	if _, err := repo.diffPatchOutputPath(""); err == nil {
		t.Fatal("empty output path accepted")
	}
	args := diffPatchArgs(DiffPatchOptions{Cached: true, Range: "HEAD^", Paths: []string{"a b"}})
	wantSuffix := []string{"--cached", "HEAD^", "--", "a b"}
	if !reflect.DeepEqual(args[len(args)-len(wantSuffix):], wantSuffix) {
		t.Fatalf("diffPatchArgs = %#v", args)
	}
	for _, overwrite := range []bool{false, true} {
		tmp := filepath.Join(dir, "tmp")
		destination := filepath.Join(dir, "installed")
		_ = os.Remove(destination)
		if err := os.WriteFile(tmp, []byte("patch"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := installDiffPatch(tmp, destination, "installed", overwrite); err != nil {
			t.Fatalf("installDiffPatch(overwrite=%t): %v", overwrite, err)
		}
		if data, err := os.ReadFile(destination); err != nil || string(data) != "patch" {
			t.Fatalf("installed patch = %q, %v", data, err)
		}
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

func TestRefspecHelpers(t *testing.T) {
	source, destination, colon, err := splitRefspec("+refs/heads/*:refs/remotes/origin/*")
	if err != nil || source != "refs/heads/*" || destination != "refs/remotes/origin/*" || !colon {
		t.Fatalf("splitRefspec = %q, %q, %t, %v", source, destination, colon, err)
	}
	if _, _, _, err := splitRefspec("a:b:c"); err == nil {
		t.Fatal("multi-colon refspec accepted")
	}
	r := newTestRepo(t)
	r.write("base", "base\n")
	r.commitAll("base")
	repo, _ := Discover(r.dir)
	ctx := context.Background()
	for _, test := range []struct {
		spec  string
		fetch bool
		valid bool
	}{
		{"refs/heads/main:refs/remotes/origin/main", true, true},
		{"main:copy", false, true},
		{"main", true, false},
		{"main:refs/remotes/origin/main", true, false},
		{"refs/heads/[bad]:refs/heads/main", false, false},
	} {
		err := repo.validateRefspec(ctx, test.spec, test.fetch)
		if (err == nil) != test.valid {
			t.Errorf("validateRefspec(%q, %t) = %v", test.spec, test.fetch, err)
		}
	}
}

func TestValidateExtractedRefspecHelpers(t *testing.T) {
	r := newTestRepo(t)
	r.write("base", "base\n")
	r.commitAll("base")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, test := range []struct {
		name                string
		source, destination string
		hasColon, invalid   bool
	}{
		{"fetch accepts complete pair", "refs/heads/main", "refs/remotes/origin/main", true, false},
		{"fetch rejects missing colon", "refs/heads/main", "", false, true},
		{"fetch rejects missing source", "", "refs/remotes/origin/main", true, true},
		{"fetch rejects missing destination", "refs/heads/main", "", true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := repo.validateFetchRefspec(ctx, test.source, test.destination, test.hasColon)
			if (err != nil) != test.invalid {
				t.Fatalf("validateFetchRefspec() = %v", err)
			}
		})
	}
	for _, test := range []struct {
		name, source, destination string
		invalid                   bool
	}{
		{"valid source and destination", "main", "reviewed", false},
		{"invalid source", "missing", "reviewed", true},
		{"invalid destination", "main", "bad..branch", true},
		{"fully qualified refs", "refs/heads/main", "refs/heads/reviewed", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := repo.validatePushRefspec(ctx, test.source, test.destination)
			if (err != nil) != test.invalid {
				t.Fatalf("validatePushRefspec() = %v", err)
			}
		})
	}
}

func TestMergeHelpers(t *testing.T) {
	in := MergeArgs{Mode: MergeNoFF, NoCommit: true, Squash: true, Strategy: "ort", StrategyOptions: []string{"ours"}, Signoff: true}
	if err := validateMergeArgs(in); err != nil {
		t.Fatal(err)
	}
	args, err := mergeArgs(in, "abc123")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, expected := range []string{"--no-ff", "--no-commit", "--squash", "--strategy=ort", "--strategy-option=ours", "--signoff", "-- abc123"} {
		if !strings.Contains(joined, expected) {
			t.Errorf("merge args %q omit %q", joined, expected)
		}
	}
	if _, err := mergeArgs(MergeArgs{Mode: MergeMode(255)}, "abc"); err == nil {
		t.Fatal("invalid merge mode accepted")
	}
	for _, preflight := range []MergePreflight{
		{State: MergeState{InProgress: true}},
		{State: MergeState{Conflicts: []string{"file"}}},
		{RequiresDirtyConfirmation: true},
	} {
		if err := validateMergePreflight(preflight, false); err == nil {
			t.Errorf("unsafe preflight accepted: %#v", preflight)
		}
	}
}

func TestResidualPathAndFileHelpers(t *testing.T) {
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

func TestResidualHistoryResolversAndPlans(t *testing.T) {
	r := newTestRepo(t)
	r.write("file", "base\n")
	head := r.commitAll("base")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if got, err := repo.resolveHistoryBranch(ctx, "main"); err != nil || got != "main" {
		t.Fatalf("resolveHistoryBranch(main) = %q, %v", got, err)
	}
	for _, branch := range []string{"", " main", "-main", "bad\x00name", "missing"} {
		if _, err := repo.resolveHistoryBranch(ctx, branch); err == nil {
			t.Errorf("resolveHistoryBranch(%q) succeeded", branch)
		}
	}
	for _, symmetric := range []bool{false, true} {
		got, err := repo.resolveLogRange(ctx, LogQuery{From: "HEAD", To: "main", Symmetric: symmetric})
		separator := ".."
		if symmetric {
			separator = "..."
		}
		if err != nil || !reflect.DeepEqual(got, []string{head + separator + head}) {
			t.Fatalf("resolveLogRange(%v) = %#v, %v", symmetric, got, err)
		}
	}
	if _, err := repo.resolveLogRange(ctx, LogQuery{From: "missing", To: "HEAD"}); err == nil {
		t.Fatal("missing range start resolved")
	}
	if _, err := repo.resolveLogRange(ctx, LogQuery{From: "HEAD", To: "missing"}); err == nil {
		t.Fatal("missing range end resolved")
	}
	if got, want := rebaseTodoPlan("\n# comment\n pick abc \n\n squash def\n"), []string{"pick abc", "squash def"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("rebaseTodoPlan() = %#v, want %#v", got, want)
	}
}

func TestResidualRebaseStateReadersAndTodoReview(t *testing.T) {
	r := newTestRepo(t)
	r.write("file", "base\n")
	head := r.commitAll("base")
	repo, _ := Discover(r.dir)
	directory := filepath.Join(r.dir, ".git", "rebase-merge")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"head-name": "refs/heads/main\n", "onto": head + "\n", "msgnum": "2\n", "end": "4\n", "git-rebase-todo": "pick " + head + "\n"}
	for name, value := range files {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	op, err := repo.rebaseState(context.Background(), "rebase-merge", false)
	if err != nil || op.Kind != OperationRebase || op.Branch != "refs/heads/main" || op.Onto != head || op.Current != 2 || op.Total != 4 {
		t.Fatalf("rebaseState() = %#v, %v", op, err)
	}
	op, err = repo.rebaseState(context.Background(), "rebase-merge", true)
	if err != nil || op.Kind != OperationApplyMailbox {
		t.Fatalf("mailbox rebaseState() = %#v, %v", op, err)
	}
	q := HistoryUIRequest{Rebase: RebaseOptions{Todo: "pick " + head + "\n"}}
	canonical, plan, err := repo.canonicalRebaseTodoHistoryUIRequest(context.Background(), q)
	if err != nil || canonical.Rebase.Todo == "" || len(plan) != 3 {
		t.Fatalf("canonical todo = %#v, %#v, %v", canonical, plan, err)
	}
	if err := os.WriteFile(filepath.Join(directory, "msgnum"), []byte("bad\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.rebaseState(context.Background(), "rebase-merge", false); err == nil {
		t.Fatal("invalid rebase progress was accepted")
	}
}

func TestResidualFetchTagsNotesAndBisect(t *testing.T) {
	ctx := context.Background()
	remote := newBareTestRepo(t)
	r := newTestRepo(t)
	r.write("file", "one\n")
	first := r.commitAll("one")
	r.git("remote", "add", "origin", remote.dir)
	r.git("push", "origin", "main")
	r.git("tag", "keep", first)
	r.git("tag", "stale", first)
	r.git("push", "origin", "refs/tags/keep")
	repo, _ := Discover(r.dir)

	if err := repo.ConfigureCurrentFetchBranch(ctx, "origin", "main"); err != nil {
		t.Fatal(err)
	}
	if got := r.git("config", "--get", "branch.main.merge"); got != "refs/heads/main" {
		t.Fatalf("configured merge ref = %q", got)
	}
	if err := repo.ConfigureCurrentFetchBranch(ctx, "missing", "main"); err == nil {
		t.Fatal("missing fetch remote accepted")
	}
	if err := repo.ConfigureCurrentFetchBranch(ctx, "origin", "missing"); err == nil {
		t.Fatal("missing fetch branch accepted")
	}

	comparison, err := repo.PruneRemoteTags(ctx, "origin", false)
	if err == nil || !reflect.DeepEqual(comparison.LocalOnly, []string{"stale"}) {
		t.Fatalf("unconfirmed prune = %#v, %v", comparison, err)
	}
	if _, err := repo.PruneRemoteTags(ctx, "origin", true); err != nil {
		t.Fatal(err)
	}
	if testRefExists(r.dir, "refs/tags/stale") {
		t.Fatal("stale local tag survived prune")
	}
	if _, err := repo.PruneRemoteTags(ctx, "missing", true); err == nil {
		t.Fatal("missing prune remote accepted")
	}

	r.git("notes", "add", "-m", "note", first)
	review, err := repo.ReviewNotesPrune(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if review.NotesOID == "" {
		t.Fatal("notes prune review did not bind the notes ref")
	}
	if err := repo.PruneNotesReviewed(ctx, review); err != nil {
		t.Fatal(err)
	}
	stale := review
	stale.NotesOID = "different"
	if err := repo.PruneNotesReviewed(ctx, stale); err == nil {
		t.Fatal("stale notes prune review accepted")
	}

	r.write("file", "two\n")
	second := r.commitAll("two")
	if err := repo.BisectStart(ctx, BisectStartOptions{Bad: second, Good: first}); err != nil {
		t.Fatal(err)
	}
	if err := repo.bisectMark(ctx, "good", first); err != nil {
		t.Fatal(err)
	}
	_ = repo.BisectReset(ctx)
	if err := repo.bisectMark(ctx, "bad", ""); !errors.Is(err, ErrWorkflowNotActive) {
		t.Fatalf("inactive bisect mark = %v", err)
	}
}

func TestResidualCommitInvocationsAndSquash(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("base", "base\n")
	base := r.commitAll("base")
	repo, _ := Discover(r.dir)

	for _, variant := range []CommitUIVariant{CommitUIFixup, CommitUISquash, CommitUIAugment, CommitUIAlter, CommitUIRevise} {
		message := ""
		if variant != CommitUIFixup && variant != CommitUISquash {
			message = "replacement"
		}
		invocation, err := repo.autosquashCommitUIInvocation(ctx, variant, "HEAD", message, CommitOptions{})
		if err != nil || len(invocation.args) == 0 {
			t.Errorf("autosquash invocation %s = %#v, %v", variant, invocation, err)
		}
	}
	if _, err := repo.autosquashCommitUIInvocation(ctx, "unknown", "HEAD", "", CommitOptions{}); err == nil {
		t.Fatal("unknown autosquash workflow accepted")
	}
	if _, err := repo.squashCommitUIInvocation(ctx, CommitUISquash, base, "", CommitOptions{ReuseMessage: "HEAD"}); err == nil {
		t.Fatal("squash reuse-message accepted")
	}
	if _, err := repo.squashCommitUIInvocation(ctx, CommitUIAugment, base, "", CommitOptions{}); err == nil {
		t.Fatal("empty augment message accepted")
	}
	if _, err := repo.structuredFixupCommitUIInvocation(ctx, CommitUIRevise, base, "replacement", CommitOptions{All: true}); err == nil {
		t.Fatal("revised all invocation accepted")
	}

	r.write("squash", "content\n")
	r.git("add", "squash")
	commit, err := repo.SquashCommit(ctx, "HEAD", "extra", CommitOptions{})
	if err != nil || commit.ID == "" {
		t.Fatalf("SquashCommit() = %#v, %v", commit, err)
	}
	if _, err := repo.SquashCommit(ctx, "HEAD", "", CommitOptions{ReuseMessage: "HEAD"}); err == nil {
		t.Fatal("SquashCommit accepted reuse-message")
	}
}

func TestResidualCompilePushUISelection(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("base", "base\n")
	r.commitAll("base")
	r.git("tag", "v1")
	r.git("notes", "--ref=review", "add", "-m", "note", "HEAD")
	repo, _ := Discover(r.dir)
	tests := []PushUIArgs{
		{Refspecs: []string{"main:copy"}},
		{PushArgs: PushArgs{Refspec: "main:copy"}},
		{PushArgs: PushArgs{Source: "main", Destination: "copy"}},
		{PushArgs: PushArgs{Matching: true}},
		{PushArgs: PushArgs{Tag: "v1"}},
		{PushArgs: PushArgs{AllTags: true}},
		{NotesRef: "refs/notes/review"},
		{PushArgs: PushArgs{Notes: true}},
		{},
	}
	for i, input := range tests {
		selection, err := repo.compilePushUISelection(ctx, input)
		if err != nil || (!selection.AllTags && len(selection.Refspecs) == 0) {
			t.Errorf("selection %d = %#v, %v", i, selection, err)
		}
	}
	if selection, err := repo.compilePushUISelection(ctx, PushUIArgs{PushArgs: PushArgs{Tag: "missing"}}); err != nil || !reflect.DeepEqual(selection.Refspecs, []string{"refs/tags/missing"}) {
		t.Fatalf("syntactic tag selection = %#v, %v", selection, err)
	}
	if _, err := repo.compilePushUISelection(ctx, PushUIArgs{NotesRef: "review"}); err == nil {
		t.Fatal("short notes ref accepted")
	}
}
