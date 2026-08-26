package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func requireFileWorkflow(t *testing.T, m *Model, title string) {
	t.Helper()
	if m.mode != modeWorkflow || m.workflow == nil {
		t.Fatalf("%s did not open a workflow: mode=%d message=%q", title, m.mode, m.message)
	}
}

func replaceWorkflowText(t *testing.T, m *Model, value string) {
	t.Helper()
	for m.workflow.dialog.Fields[m.workflow.field].Value != "" {
		sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	}
	for _, r := range value {
		sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
	}
}

func submitReviewedFileWorkflow(t *testing.T, m *Model) {
	t.Helper()
	for m.workflow != nil && m.workflow.field < len(m.workflow.dialog.Fields) {
		sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	}
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if m.workflow == nil || m.workflow.review == nil {
		t.Fatalf("file workflow did not produce a review: message=%q", m.message)
	}
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
}

func TestE2EFileRenameAndUntrackByTopLevelKeys(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("tracked ; old.txt", "base\n")
	r.write("other.txt", "other\n")
	r.git("add", "--", "tracked ; old.txt", "other.txt")
	r.git("commit", "-m", "base")
	r.write("tracked ; old.txt", "changed\n")
	r.write("loose [x] !.txt", "loose\n")

	m := newE2EModel(t, r)
	selectE2EPath(t, m, "loose [x] !.txt", rowUntracked)
	sendE2EKey(t, m, keyMsg("R"))
	requireFileWorkflow(t, m, "R")
	replaceWorkflowText(t, m, "renamed ; [x] !.txt")
	submitReviewedFileWorkflow(t, m)
	if content, err := os.ReadFile(filepath.Join(r.dir, "renamed ; [x] !.txt")); err != nil || string(content) != "loose\n" {
		t.Fatalf("untracked rename content=%q err=%v", content, err)
	}
	if got := r.git("ls-files", "--", "renamed ; [x] !.txt"); got != "" {
		t.Fatalf("untracked rename became tracked: %q", got)
	}

	selectE2EPath(t, m, "tracked ; old.txt", rowUnstaged)
	sendE2EKey(t, m, keyMsg("R"))
	requireFileWorkflow(t, m, "R")
	replaceWorkflowText(t, m, "tracked ; new.txt")
	submitReviewedFileWorkflow(t, m)
	if got := r.git("diff", "--cached", "--name-status"); !strings.Contains(got, "tracked ; old.txt") || !strings.Contains(got, "tracked ; new.txt") {
		t.Fatalf("tracked rename was not staged: %q", got)
	}

	// Stage unrelated content and ensure K removes only the selected index path
	// while explicitly preserving its worktree file.
	r.write("other.txt", "staged unrelated\n")
	r.git("add", "--", "other.txt")
	sendE2EKey(t, m, keyMsg("g"))
	selectE2EPath(t, m, "tracked ; new.txt", rowStaged)
	sendE2EKey(t, m, keyMsg("K"))
	requireFileWorkflow(t, m, "K")
	submitReviewedFileWorkflow(t, m)
	if _, err := os.Stat(filepath.Join(r.dir, "tracked ; new.txt")); err != nil {
		t.Fatalf("K deleted the worktree file: %v", err)
	}
	if got := r.git("ls-files", "--", "tracked ; new.txt"); got != "" {
		t.Fatalf("K left selected file tracked: %q", got)
	}
	if got := r.git("diff", "--cached", "--name-only", "--", "other.txt"); got != "other.txt" {
		t.Fatalf("K disturbed unrelated staged content: %q", got)
	}
}

func TestE2EGitignoreLiteralRulePreservesUnrelatedStaging(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write(".gitignore", "base.ignore\n")
	r.write("tracked.txt", "base\n")
	r.git("add", "--", ".gitignore", "tracked.txt")
	r.git("commit", "-m", "base")
	r.write(".gitignore", "base.ignore\nunrelated worktree edit\n")
	weird := "dir/odd [#] ! name?.txt"
	r.write(weird, "ignore me\n")

	m := newE2EModel(t, r)
	selectE2EPath(t, m, weird, rowUntracked)
	sendE2EKey(t, m, keyMsg("i"))
	sendE2EKey(t, m, keyMsg("t"))
	requireFileWorkflow(t, m, "i t")
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	worktree, err := os.ReadFile(filepath.Join(r.dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	rule := "/" + escapeIgnoreLiteral(weird)
	if got := string(worktree); got != "base.ignore\nunrelated worktree edit\n"+rule+"\n" {
		t.Fatalf("worktree .gitignore = %q", got)
	}
	if got := r.git("show", ":.gitignore"); got != "base.ignore\n"+rule {
		t.Fatalf("index .gitignore staged unrelated edit: %q", got)
	}
	if got := r.git("check-ignore", "--no-index", "--", weird); got != weird {
		t.Fatalf("literal rule did not ignore weird path: %q", got)
	}
}

func TestE2EFileWorkflowCancelStaleAndIndexFlags(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("staged weird.txt", "base\n")
	r.git("add", "--", "staged weird.txt")
	r.git("commit", "-m", "base")
	r.write("staged weird.txt", "staged\n")
	r.git("add", "--", "staged weird.txt")

	m := newE2EModel(t, r)
	selectE2EPath(t, m, "staged weird.txt", rowStaged)
	sendE2EKey(t, m, keyMsg("R"))
	requireFileWorkflow(t, m, "R")
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	if _, err := os.Stat(filepath.Join(r.dir, "staged weird.txt")); err != nil || m.mode != modeStatus {
		t.Fatalf("cancel changed file or mode: err=%v mode=%d", err, m.mode)
	}

	for _, tc := range []struct {
		set, clear string
		flag       byte
	}{{"w", "W", 'S'}, {"u", "U", 'h'}} {
		selectE2EPath(t, m, "staged weird.txt", rowStaged)
		sendE2EKey(t, m, keyMsg("i"))
		sendE2EKey(t, m, keyMsg(tc.set))
		requireFileWorkflow(t, m, "index flag set")
		submitReviewedFileWorkflow(t, m)
		if got := r.git("ls-files", "-v", "--", "staged weird.txt"); got == "" || got[0] != tc.flag {
			t.Fatalf("set %s flag output = %q", tc.set, got)
		}
		selectE2EPath(t, m, "staged weird.txt", rowStaged)
		sendE2EKey(t, m, keyMsg("i"))
		sendE2EKey(t, m, keyMsg(tc.clear))
		requireFileWorkflow(t, m, "index flag clear")
		submitReviewedFileWorkflow(t, m)
	}

	// A review uses the exact selected status row. If it disappears after the
	// dialog opens, the mutation is rejected instead of acting on stale context.
	r.write("stale.txt", "stale\n")
	sendE2EKey(t, m, keyMsg("g"))
	selectE2EPath(t, m, "stale.txt", rowUntracked)
	sendE2EKey(t, m, keyMsg("R"))
	requireFileWorkflow(t, m, "stale rename")
	replaceWorkflowText(t, m, "must-not-exist.txt")
	if err := os.Remove(filepath.Join(r.dir, "stale.txt")); err != nil {
		t.Fatal(err)
	}
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if m.workflow == nil || !strings.Contains(m.workflow.error, "stale") {
		t.Fatalf("stale selection was not rejected: workflow=%v message=%q", m.workflow != nil, m.message)
	}
	if _, err := os.Stat(filepath.Join(r.dir, "must-not-exist.txt")); !os.IsNotExist(err) {
		t.Fatalf("stale rename created destination: %v", err)
	}
}
