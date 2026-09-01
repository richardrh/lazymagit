package ui

import (
	"strings"
	"testing"

	sectionmodel "github.com/richardrh/lazymagit/internal/model"
)

func TestStashStatusSectionJumpAndDetailUsesStableOID(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("file", "base\n")
	r.git("add", "--", "file")
	r.git("commit", "-m", "base")
	r.write("file", "stashed\n")
	r.git("stash", "push", "-m", "status stash")
	m := newE2EModel(t, r)
	stashes, err := m.repo.Stashes(m.appCtx)
	if err != nil || len(stashes) != 1 {
		t.Fatalf("stashes = %#v, %v", stashes, err)
	}
	wantID := "status/stashes/stash/" + stashes[0].ID
	if m.tree.Section(sectionmodel.SectionID(wantID)) == nil {
		t.Fatalf("status tree omitted stable stash row %q", wantID)
	}

	sendE2EKey(t, m, keyMsg("f2"))
	sendE2EKey(t, m, keyMsg("j"))
	sendE2EKey(t, m, keyMsg("z"))
	if got := string(m.tree.Cursor()); got != "status/stashes" {
		t.Fatalf("j z cursor = %q", got)
	}
	if m.tree.IsFolded("status/stashes") {
		m.tree.ToggleFold("status/stashes")
	}
	if !m.tree.SetCursor(sectionmodel.SectionID(wantID)) {
		t.Fatalf("could not select visible stash row %q", wantID)
	}
	runE2ECmd(t, m, m.loadDetailCmd())
	for _, want := range []string{stashes[0].ID, "status stash", "+stashed"} {
		if !strings.Contains(m.detail, want) {
			t.Fatalf("stash detail omitted %q: %q", want, m.detail)
		}
	}
}

func TestStashE2EPushRoutesSelectionAndCancellation(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("tracked.txt", "base\n")
	r.write("other.txt", "base\n")
	r.git("add", "--", "tracked.txt", "other.txt")
	r.git("commit", "-m", "base")
	r.write("tracked.txt", "first\n")
	m := newE2EModel(t, r)

	openStashByKeys(t, m, "z", "z")
	setStashMessageAndSubmit(t, m, "first stash")
	if got := r.git("stash", "list", "--format=%s"); !strings.Contains(got, "first stash") {
		t.Fatalf("z z did not create stash: %q", got)
	}

	r.write("tracked.txt", "second\n")
	r.write("other.txt", "leave this change\n")
	sendE2EKey(t, m, keyMsg("z"))
	sendE2EKey(t, m, keyMsg("P"))
	if m.mode == modeWorkflow {
		t.Fatal("parent z P was an inert duplicate terminal instead of a child edge")
	}
	// The child -- infix is edited as bounded text and passed as literal argv,
	// not interpreted as a pathspec expression or shell input.
	sendE2EKey(t, m, keyMsg("-"))
	sendE2EKey(t, m, keyMsg("-"))
	sendE2EKey(t, m, keyMsg("tracked.txt"))
	sendE2EKey(t, m, keyMsg("enter"))
	sendE2EKey(t, m, keyMsg("P"))
	if m.mode != modeWorkflow {
		t.Fatalf("z P P did not reach terminal stash push: mode=%v message=%q", m.mode, m.message)
	}
	setStashMessageAndSubmit(t, m, "second stash")
	if r.git("status", "--porcelain", "--", "other.txt") != "M other.txt" {
		t.Fatal("child literal path selection stashed an unselected file")
	}

	openStashByKeys(t, m, "z", "a")
	first := m.workflow.dialog.Fields[0].Value
	sendE2EKey(t, m, keyMsg("space"))
	selected := m.workflow.dialog.Fields[0].Value
	if selected == first {
		t.Fatal("stash selection did not advance")
	}
	sendE2EKey(t, m, keyMsg("esc"))
	got := r.git("stash", "list", "--format=%H")
	if m.mode != modeStatus || len(strings.Fields(got)) != 2 {
		t.Fatalf("cancelled apply mutated stashes: mode=%v stashes=%q", m.mode, got)
	}
}

func TestStashE2EChildOptionsDoNotInheritParentInfixes(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("tracked", "base\n")
	r.git("add", "--", "tracked")
	r.git("commit", "-m", "base")
	r.write("tracked", "first\n")
	r.write("untracked", "keep after parent option\n")
	m := newE2EModel(t, r)

	for _, key := range []string{"z", "-", "u", "P", "P"} {
		sendE2EKey(t, m, keyMsg(key))
	}
	if m.mode != modeWorkflow {
		t.Fatalf("z -u P P did not open child push: mode=%v message=%q", m.mode, m.message)
	}
	setStashMessageAndSubmit(t, m, "parent option must not leak")
	if got := r.git("status", "--porcelain", "--", "untracked"); got != "?? untracked" {
		t.Fatalf("z -u P P inherited parent -u: %q", got)
	}

	r.write("tracked", "second\n")
	for _, key := range []string{"z", "P", "-", "u", "P"} {
		sendE2EKey(t, m, keyMsg(key))
	}
	if m.mode != modeWorkflow {
		t.Fatalf("z P -u P did not open child push: mode=%v message=%q", m.mode, m.message)
	}
	setStashMessageAndSubmit(t, m, "child option is consumed")
	if got := r.git("status", "--porcelain", "--", "untracked"); got != "" {
		t.Fatalf("z P -u P did not consume child -u: %q", got)
	}
}

func TestStashE2ESnapshotBothKeepsIndexAndWorktree(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("staged", "base staged\n")
	r.write("worktree", "base worktree\n")
	r.git("add", "--", "staged", "worktree")
	r.git("commit", "-m", "base")
	r.write("staged", "indexed checkpoint\n")
	r.git("add", "--", "staged")
	r.write("worktree", "worktree checkpoint\n")
	beforeIndex := r.git("diff", "--cached", "--binary")
	beforeWorktree := r.git("diff", "--binary")
	m := newE2EModel(t, r)

	openStashByKeys(t, m, "z", "Z")
	setStashMessageAndSubmit(t, m, "safe checkpoint")
	if m.isError {
		t.Fatalf("snapshot workflow failed: %s", m.message)
	}
	if got := r.git("diff", "--cached", "--binary"); got != beforeIndex {
		t.Fatalf("z Z changed index:\n%s", got)
	}
	if got := r.git("diff", "--binary"); got != beforeWorktree {
		t.Fatalf("z Z changed worktree:\n%s", got)
	}
	if got := r.git("stash", "list", "--format=%s"); !strings.Contains(got, "safe checkpoint") {
		t.Fatalf("z Z did not store snapshot: %q", got)
	}
	patch := r.git("stash", "show", "--patch", "--no-color", "stash@{0}")
	if !strings.Contains(patch, "+indexed checkpoint") || !strings.Contains(patch, "+worktree checkpoint") {
		t.Fatalf("z Z snapshot omitted a layer: %q", patch)
	}
}

func TestStashE2EBranchReviewBindsNormalizedName(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("file", "base\n")
	r.git("add", "--", "file")
	r.git("commit", "-m", "base")
	r.write("file", "stash\n")
	r.git("stash", "push", "-m", "branch target")
	m := newE2EModel(t, r)

	openStashByKeys(t, m, "z", "b")
	sendE2EKey(t, m, keyMsg("tab"))
	sendE2EKey(t, m, keyMsg("  reviewed-branch  "))
	sendE2EKey(t, m, keyMsg("tab"))
	sendE2EKey(t, m, keyMsg("enter"))
	if m.workflow == nil || m.workflow.review == nil || len(m.workflow.review.Plan) == 0 || m.workflow.review.Plan[0] != "branch: reviewed-branch" {
		t.Fatalf("normalized branch is absent from review: %#v", m.workflow)
	}
	// Simulate substitution outside normal locked editing. SubmitReview must use
	// only the opaque reviewed plan, never these newly collected values.
	m.workflow.dialog.Fields[1].Value = "substituted-branch"
	sendE2EKey(t, m, keyMsg("enter"))
	if m.isError || r.git("branch", "--show-current") != "reviewed-branch" {
		t.Fatalf("branch review accepted substituted values: error=%v message=%q branch=%q", m.isError, m.message, r.git("branch", "--show-current"))
	}
	if got := r.git("branch", "--list", "substituted-branch"); got != "" {
		t.Fatalf("substituted branch was created: %q", got)
	}
	if got := r.git("stash", "list", "--format=%H"); len(strings.Fields(got)) != 1 {
		t.Fatalf("exact reviewed branch removed its stash entry: %q", got)
	}
}

func TestStashE2EReviewedApplyBindsSelectedOIDAndOptions(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("file", "base\n")
	r.git("add", "--", "file")
	r.git("commit", "-m", "base")
	r.write("file", "old stash\n")
	r.git("stash", "push", "-m", "old")
	r.write("file", "new stash\n")
	r.git("stash", "push", "-m", "new")
	ids := strings.Fields(r.git("stash", "list", "--format=%H"))
	if len(ids) != 2 {
		t.Fatalf("stash setup = %#v", ids)
	}
	m := newE2EModel(t, r)

	openStashByKeys(t, m, "z", "a")
	sendE2EKey(t, m, keyMsg("space")) // select the older stash
	sendE2EKey(t, m, keyMsg("tab"))
	sendE2EKey(t, m, keyMsg("tab"))
	sendE2EKey(t, m, keyMsg("enter"))
	plan, ok := m.workflow.review.Data.(reviewedStashApply)
	if !ok || plan.stash.Stash.ID != ids[1] || plan.index {
		t.Fatalf("reviewed apply plan = %#v", m.workflow.review)
	}
	// Substitute both editable values after review. Execution must use only the
	// selected OID and restore-index choice captured in the opaque plan.
	m.workflow.dialog.Fields[0].Value = ids[0]
	m.workflow.dialog.Fields[1].Bool = true
	sendE2EKey(t, m, keyMsg("enter"))
	if m.isError || r.git("show", ":file") != "base" || r.git("status", "--porcelain", "--", "file") != "M file" {
		t.Fatalf("reviewed apply accepted substituted values: error=%v message=%q", m.isError, m.message)
	}
	if r.git("stash", "list", "--format=%H") != strings.Join(ids, "\n") {
		t.Fatal("reviewed apply removed or reordered stash entries")
	}
}

func TestStashE2EShowSelectsExactOIDFromBothTransients(t *testing.T) {
	for name, keys := range map[string][]string{"stash": {"z", "v"}, "diff": {"d", "t"}} {
		t.Run(name, func(t *testing.T) {
			r := newUIE2ERepo(t)
			r.write("file", "base\n")
			r.git("add", "--", "file")
			r.git("commit", "-m", "base")
			r.write("file", "old selected content\n")
			r.git("stash", "push", "-m", "old selected stash")
			r.write("file", "new top content\n")
			r.git("stash", "push", "-m", "new top stash")
			before := r.git("stash", "list", "--format=%H")
			ids := strings.Fields(before)
			m := newE2EModel(t, r)

			openStashByKeys(t, m, keys...)
			if name == "stash" {
				sendE2EKey(t, m, keyMsg("esc"))
				if m.mode != modeStatus || r.git("stash", "list", "--format=%H") != before {
					t.Fatal("cancelled stash show mutated state")
				}
				openStashByKeys(t, m, keys...)
			}
			sendE2EKey(t, m, keyMsg("space")) // choose non-top stash
			if m.workflow.dialog.Fields[0].Value != ids[1] {
				t.Fatalf("selected OID = %q, want %q", m.workflow.dialog.Fields[0].Value, ids[1])
			}
			sendE2EKey(t, m, keyMsg("tab"))
			sendE2EKey(t, m, keyMsg("enter"))
			if !strings.Contains(m.detail, ids[1]) || !strings.Contains(m.detail, "old selected content") || strings.Contains(m.detail, "new top content") {
				t.Fatalf("exact selected stash was not displayed through %v:\n%s", keys, m.detail)
			}
			if got := r.git("stash", "list", "--format=%H"); got != before {
				t.Fatalf("stash show mutated entries: before=%q after=%q", before, got)
			}
		})
	}
}

func openStashByKeys(t *testing.T, m *Model, keys ...string) {
	t.Helper()
	for _, key := range keys {
		sendE2EKey(t, m, keyMsg(key))
	}
	if m.mode != modeWorkflow {
		t.Fatalf("keys %q did not open stash workflow: mode=%v message=%q", keys, m.mode, m.message)
	}
}

func setStashMessageAndSubmit(t *testing.T, m *Model, message string) {
	t.Helper()
	sendE2EKey(t, m, keyMsg(message))
	sendE2EKey(t, m, keyMsg("tab"))
	sendE2EKey(t, m, keyMsg("enter"))
	if m.isError {
		t.Fatalf("stash push failed: %s", m.message)
	}
}
