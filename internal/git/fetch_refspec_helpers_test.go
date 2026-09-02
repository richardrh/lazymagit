package git

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

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

func TestFetchConfigurationTagAndNotesPruningAndBisectState(t *testing.T) {
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

func TestCompilePushUISelectionSupportsRefspecTagAndNotesModes(t *testing.T) {
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
